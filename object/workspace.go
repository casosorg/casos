package object

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	ErrWorkspaceOwnerRequired = errors.New("workspace owner is required")
	ErrWorkspaceNotFound      = errors.New("workspace not found")
	ErrWorkspaceInvalidInput  = errors.New("invalid workspace input")
	ErrWorkspaceConflict      = errors.New("workspace changed; reload and retry")
)

type Workspace struct {
	Id             string    `xorm:"'id' varchar(40) pk" json:"id"`
	Owner          string    `xorm:"'owner' varchar(100) notnull unique(owner_name)" json:"owner"`
	Name           string    `xorm:"'name' varchar(100) notnull unique(owner_name)" json:"name"`
	Repo           string    `xorm:"'repo' varchar(2048)" json:"repo"`
	Template       string    `xorm:"'template' varchar(100)" json:"template"`
	DefaultAgent   string    `xorm:"'default_agent' varchar(20)" json:"defaultAgent"`
	Namespace      string    `xorm:"'namespace' varchar(253)" json:"namespace"`
	DeploymentName string    `xorm:"'deployment_name' varchar(253)" json:"deploymentName"`
	DeploymentUID  string    `xorm:"'deployment_uid' varchar(100)" json:"deploymentUid"`
	HomeClaimName  string    `xorm:"'home_claim_name' varchar(253)" json:"homeClaimName"`
	HomeClaimUID   string    `xorm:"'home_claim_uid' varchar(100)" json:"homeClaimUid"`
	DesiredState   string    `xorm:"'desired_state' varchar(20)" json:"desiredState"`
	DataPolicy     string    `xorm:"'data_policy' varchar(20)" json:"dataPolicy"`
	CreatedAt      time.Time `xorm:"'created_at'" json:"createdAt"`
	UpdatedAt      time.Time `xorm:"'updated_at'" json:"updatedAt"`
	LastActivityAt time.Time `xorm:"'last_activity_at'" json:"lastActivityAt"`
	Revision       int64     `xorm:"'revision'" json:"revision"`
}

type WorkspaceSession struct {
	Id          string    `xorm:"'id' varchar(40) pk" json:"id"`
	WorkspaceId string    `xorm:"'workspace_id' varchar(40) notnull index" json:"workspaceId"`
	AgentType   string    `xorm:"'agent_type' varchar(20)" json:"agentType"`
	TmuxName    string    `xorm:"'tmux_name' varchar(80)" json:"tmuxName"`
	PodUID      string    `xorm:"'pod_uid' varchar(100)" json:"podUid"`
	State       string    `xorm:"'last_observed_state' varchar(20)" json:"lastObservedState"`
	ExitCode    *int      `xorm:"'exit_code'" json:"exitCode"`
	ActiveKey   *string   `xorm:"'active_key' varchar(40) unique" json:"-"`
	CreatedAt   time.Time `xorm:"'created_at'" json:"createdAt"`
	UpdatedAt   time.Time `xorm:"'updated_at'" json:"updatedAt"`
	FinishedAt  time.Time `xorm:"'finished_at'" json:"finishedAt"`
	Revision    int64     `xorm:"'revision'" json:"revision"`
}

type WorkspaceSpec struct {
	Name         string
	Repo         string
	Template     string
	DefaultAgent string
}

func workspaceOwnerValid(owner string) error {
	if owner == "" || owner != strings.TrimSpace(owner) || len(owner) > 100 {
		return ErrWorkspaceOwnerRequired
	}
	return nil
}

func workspaceAgentValid(agent string) bool {
	return agent == "codex" || agent == "claude" || agent == "terminal"
}

func newWorkspaceId(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func CreateWorkspace(owner string, spec WorkspaceSpec) (*Workspace, error) {
	if err := workspaceOwnerValid(owner); err != nil {
		return nil, err
	}
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Repo = strings.TrimSpace(spec.Repo)
	if spec.Name == "" || len(spec.Name) > 100 || len(spec.Repo) > 2048 {
		return nil, fmt.Errorf("%w: name or repository length", ErrWorkspaceInvalidInput)
	}
	if spec.Repo != "" {
		u, err := url.Parse(spec.Repo)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("%w: repository must be an HTTPS URL without credentials, query or fragment", ErrWorkspaceInvalidInput)
		}
	}
	if spec.Template == "" {
		spec.Template = "devbox"
	}
	if spec.DefaultAgent == "" {
		spec.DefaultAgent = "terminal"
	}
	if spec.Template != "devbox" || !workspaceAgentValid(spec.DefaultAgent) {
		return nil, fmt.Errorf("%w: unsupported template or agent", ErrWorkspaceInvalidInput)
	}
	id, err := newWorkspaceId("ws_")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	workspace := &Workspace{
		Id: id, Owner: owner, Name: spec.Name, Repo: spec.Repo,
		Template: spec.Template, DefaultAgent: spec.DefaultAgent,
		DesiredState: "stopped", DataPolicy: "retain",
		CreatedAt: now, UpdatedAt: now, LastActivityAt: now, Revision: 1,
	}
	_, err = ormer.Engine.Insert(workspace)
	if err != nil {
		return nil, err
	}
	return workspace, nil
}

func GetWorkspace(owner, id string) (*Workspace, error) {
	if err := workspaceOwnerValid(owner); err != nil {
		return nil, err
	}
	workspace := &Workspace{}
	found, err := ormer.Engine.Where("owner = ? AND id = ?", owner, id).Get(workspace)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrWorkspaceNotFound
	}
	return workspace, nil
}

func workspacePageValid(offset, limit int) error {
	if offset < 0 || limit < 1 || limit > 100 {
		return fmt.Errorf("%w: offset must be nonnegative and limit between 1 and 100", ErrWorkspaceInvalidInput)
	}
	return nil
}

func GetWorkspaces(owner string, offset, limit int) ([]*Workspace, error) {
	if err := workspaceOwnerValid(owner); err != nil {
		return nil, err
	}
	if err := workspacePageValid(offset, limit); err != nil {
		return nil, err
	}
	result := []*Workspace{}
	err := ormer.Engine.Where("owner = ?", owner).Desc("created_at", "id").Limit(limit, offset).Find(&result)
	return result, err
}

func SetWorkspaceDesiredState(owner, id, state string, revision int64) (*Workspace, error) {
	workspace, err := GetWorkspace(owner, id)
	if err != nil {
		return nil, err
	}
	if state != "running" && state != "stopped" && state != "deleted" {
		return nil, fmt.Errorf("%w: unsupported desired state", ErrWorkspaceInvalidInput)
	}
	if workspace.Revision != revision || workspace.DesiredState == "deleted" && state != "deleted" {
		return nil, ErrWorkspaceConflict
	}
	if workspace.DesiredState == state {
		return workspace, nil
	}
	workspace.DesiredState = state
	workspace.UpdatedAt = time.Now().UTC()
	workspace.Revision++
	affected, err := ormer.Engine.Where("owner = ? AND id = ? AND revision = ?", owner, id, revision).
		Cols("desired_state", "updated_at", "revision").Update(workspace)
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrWorkspaceConflict
	}
	return workspace, nil
}

func CreateWorkspaceSession(owner, workspaceId, agent, podUID string) (*WorkspaceSession, error) {
	if err := workspaceOwnerValid(owner); err != nil {
		return nil, err
	}
	if !workspaceAgentValid(agent) || podUID == "" || podUID != strings.TrimSpace(podUID) || len(podUID) > 100 {
		return nil, fmt.Errorf("%w: agent and Pod UID are required", ErrWorkspaceInvalidInput)
	}
	id, err := newWorkspaceId("ses_")
	if err != nil {
		return nil, err
	}
	session := ormer.Engine.NewSession()
	defer session.Close()
	if err = session.Begin(); err != nil {
		return nil, err
	}
	defer session.Rollback()
	workspace := &Workspace{}
	found, err := session.Where("owner = ? AND id = ?", owner, workspaceId).Get(workspace)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrWorkspaceNotFound
	}
	if workspace.DesiredState != "running" {
		return nil, ErrWorkspaceConflict
	}
	now := time.Now().UTC()
	affected, err := session.Where("owner = ? AND id = ? AND revision = ?", owner, workspaceId, workspace.Revision).
		Cols("last_activity_at", "updated_at", "revision").Update(&Workspace{LastActivityAt: now, UpdatedAt: now, Revision: workspace.Revision + 1})
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrWorkspaceConflict
	}
	result := &WorkspaceSession{
		Id: id, WorkspaceId: workspaceId, AgentType: agent,
		TmuxName: "casos-" + id, PodUID: podUID, State: "starting", ActiveKey: &workspaceId,
		CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	if _, err = session.Insert(result); err != nil {
		return nil, err
	}
	if err = session.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func GetWorkspaceSession(owner, workspaceId, id string) (*WorkspaceSession, error) {
	if _, err := GetWorkspace(owner, workspaceId); err != nil {
		return nil, err
	}
	result := &WorkspaceSession{}
	found, err := ormer.Engine.Where("workspace_id = ? AND id = ?", workspaceId, id).Get(result)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrWorkspaceNotFound
	}
	return result, nil
}

func GetWorkspaceSessions(owner, workspaceId string, offset, limit int) ([]*WorkspaceSession, error) {
	if _, err := GetWorkspace(owner, workspaceId); err != nil {
		return nil, err
	}
	if err := workspacePageValid(offset, limit); err != nil {
		return nil, err
	}
	result := []*WorkspaceSession{}
	err := ormer.Engine.Where("workspace_id = ?", workspaceId).Desc("created_at", "id").Limit(limit, offset).Find(&result)
	return result, err
}

func RecordWorkspaceSessionState(owner, workspaceId, id, podUID, state string, exitCode *int, revision int64) (*WorkspaceSession, error) {
	session, err := GetWorkspaceSession(owner, workspaceId, id)
	if err != nil {
		return nil, err
	}
	if state != "running" && state != "exited" && state != "interrupted" && state != "unknown" {
		return nil, fmt.Errorf("%w: unsupported session state", ErrWorkspaceInvalidInput)
	}
	if (state == "exited") != (exitCode != nil) {
		return nil, fmt.Errorf("%w: exit code is only required for an exited session", ErrWorkspaceInvalidInput)
	}
	if session.Revision != revision || session.PodUID != podUID {
		return nil, ErrWorkspaceConflict
	}
	if session.State == state && (exitCode == nil || session.ExitCode != nil && *session.ExitCode == *exitCode) {
		return session, nil
	}
	if session.State == "exited" || session.State == "interrupted" {
		return nil, ErrWorkspaceConflict
	}
	session.State, session.ExitCode = state, exitCode
	session.UpdatedAt = time.Now().UTC()
	session.Revision++
	if state == "exited" || state == "interrupted" {
		session.FinishedAt = session.UpdatedAt
		session.ActiveKey = nil
	}
	affected, err := ormer.Engine.Where("workspace_id = ? AND id = ? AND pod_uid = ? AND revision = ?", workspaceId, id, podUID, revision).
		Cols("last_observed_state", "exit_code", "updated_at", "finished_at", "active_key", "revision").Update(session)
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrWorkspaceConflict
	}
	return session, nil
}
