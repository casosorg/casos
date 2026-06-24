package object

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"golang.org/x/crypto/ssh"
	"xorm.io/xorm"
)

const (
	MachineNodeDeployStatusPending   = "pending"
	MachineNodeDeployStatusRunning   = "running"
	MachineNodeDeployStatusSucceeded = "succeeded"
	MachineNodeDeployStatusFailed    = "failed"

	MachineNodeDeployPhaseQueued      = "queued"
	MachineNodeDeployPhasePreflight   = "preflight"
	MachineNodeDeployPhaseInstalling  = "installing"
	MachineNodeDeployPhaseConfiguring = "configuring"
	MachineNodeDeployPhaseStarting    = "starting"
	MachineNodeDeployPhaseWaiting     = "waiting"
	MachineNodeDeployPhaseReady       = "ready"
	MachineNodeDeployPhaseFailed      = "failed"

	MachineNodeDeployLogLevelInfo  = "info"
	MachineNodeDeployLogLevelError = "error"

	machineNodeCredentialPrefix  = "enc:v1:"
	machineNodeCredentialKeyFile = "node-deploy-secret.key"
)

var (
	machineNodeCredentialSecretMutex sync.Mutex
	machineNodeDeployNameRegexp      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

type MachineNodeDeployTask struct {
	Id           int64     `xorm:"pk autoincr" json:"id"`
	Owner        string    `xorm:"varchar(100) notnull index" json:"owner"`
	MachineName  string    `xorm:"varchar(100) notnull index" json:"machineName"`
	NodeName     string    `xorm:"varchar(100) notnull" json:"nodeName"`
	ApiserverURL string    `xorm:"varchar(500)" json:"apiserverUrl"`
	Status       string    `xorm:"varchar(50) notnull index" json:"status"`
	Phase        string    `xorm:"varchar(100)" json:"phase"`
	ErrorMsg     string    `xorm:"text" json:"errorMsg"`
	CreatedAt    time.Time `json:"createdAt"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type MachineNodeDeployLog struct {
	Id        int64     `xorm:"pk autoincr" json:"id"`
	TaskId    int64     `xorm:"notnull index" json:"taskId"`
	Level     string    `xorm:"varchar(20) notnull" json:"level"`
	Message   string    `xorm:"text" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

type MachineNodeDeployCredential struct {
	Owner       string    `xorm:"varchar(100) notnull pk" json:"owner"`
	MachineName string    `xorm:"varchar(100) notnull pk" json:"machineName"`
	PrivateKey  string    `xorm:"mediumtext" json:"-"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func CreateMachineNodeDeployTask(owner, machineName, nodeName, apiserverURL string) (*MachineNodeDeployTask, error) {
	owner = strings.TrimSpace(owner)
	machineName = strings.TrimSpace(machineName)
	nodeName = strings.TrimSpace(nodeName)
	apiserverURL = strings.TrimSpace(apiserverURL)
	if owner == "" || machineName == "" {
		return nil, fmt.Errorf("owner and machineName are required")
	}
	if strings.Contains(machineName, "/") {
		return nil, fmt.Errorf("machineName must not contain /")
	}
	if nodeName == "" {
		return nil, fmt.Errorf("nodeName is required")
	}
	if err := validateMachineNodeDeployName(nodeName); err != nil {
		return nil, err
	}
	if apiserverURL != "" {
		parsed, err := url.Parse(apiserverURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("apiserverUrl must be a valid https URL")
		}
	}

	result, err := withMachineNodeDeployTransaction(func(session *xorm.Session) (interface{}, error) {
		machine := &Machine{}
		foundMachine, err := session.
			Where("owner = ? AND name = ?", owner, machineName).
			ForUpdate().
			Get(machine)
		if err != nil {
			return nil, err
		}
		if !foundMachine {
			return nil, fmt.Errorf("machine not found")
		}

		existing := &MachineNodeDeployTask{}
		found, err := session.
			Where("owner = ? AND machine_name = ? AND status IN (?, ?)", owner, machineName, MachineNodeDeployStatusPending, MachineNodeDeployStatusRunning).
			Desc("id").
			ForUpdate().
			Get(existing)
		if err != nil {
			return nil, err
		}
		if found {
			return nil, fmt.Errorf("node deployment task %d is already active for machine %s", existing.Id, machineName)
		}

		now := time.Now().UTC()
		task := &MachineNodeDeployTask{
			Owner:        owner,
			MachineName:  machineName,
			NodeName:     nodeName,
			ApiserverURL: apiserverURL,
			Status:       MachineNodeDeployStatusPending,
			Phase:        MachineNodeDeployPhaseQueued,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := session.Insert(task); err != nil {
			return nil, err
		}
		return task, nil
	})
	if err != nil {
		return nil, err
	}
	task, ok := result.(*MachineNodeDeployTask)
	if !ok || task == nil {
		return nil, fmt.Errorf("create node deployment task returned invalid result")
	}
	return task, nil
}

func GetMachineNodeDeployTask(id int64) (*MachineNodeDeployTask, error) {
	task := &MachineNodeDeployTask{Id: id}
	found, err := ormer.Engine.Get(task)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return task, nil
}

func validateMachineNodeDeployName(nodeName string) error {
	nodeName = strings.TrimSpace(nodeName)
	if len(nodeName) > 253 || !machineNodeDeployNameRegexp.MatchString(nodeName) {
		return fmt.Errorf("nodeName must be a valid RFC 1123 subdomain")
	}
	for _, label := range strings.Split(nodeName, ".") {
		if len(label) > 63 {
			return fmt.Errorf("nodeName labels must not exceed 63 characters")
		}
	}
	return nil
}

func GetMachineNodeDeployTasks(owner, machineName string, limit int) ([]*MachineNodeDeployTask, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must not be negative")
	}
	if limit == 0 {
		limit = 50
	} else if limit > 100 {
		return nil, fmt.Errorf("limit must not exceed 100")
	}
	owner = strings.TrimSpace(owner)
	machineName = strings.TrimSpace(machineName)
	if owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	tasks := []*MachineNodeDeployTask{}
	session := ormer.Engine.Desc("id").Limit(limit)
	session = session.Where("owner = ?", owner)
	if machineName != "" {
		session = session.And("machine_name = ?", machineName)
	}
	if err := session.Find(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func FailActiveMachineNodeDeployTasks(reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "node deployment task was interrupted"
	}
	now := time.Now().UTC()
	_, err := ormer.Engine.
		Where("status IN (?, ?)", MachineNodeDeployStatusPending, MachineNodeDeployStatusRunning).
		Cols("status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&MachineNodeDeployTask{
			Status:     MachineNodeDeployStatusFailed,
			Phase:      MachineNodeDeployPhaseFailed,
			ErrorMsg:   reason,
			FinishedAt: now,
			UpdatedAt:  now,
		})
	return err
}

func isValidMachineNodeDeployPhase(phase string) bool {
	switch phase {
	case MachineNodeDeployPhaseQueued, MachineNodeDeployPhasePreflight, MachineNodeDeployPhaseInstalling,
		MachineNodeDeployPhaseConfiguring, MachineNodeDeployPhaseStarting, MachineNodeDeployPhaseWaiting,
		MachineNodeDeployPhaseReady, MachineNodeDeployPhaseFailed:
		return true
	default:
		return false
	}
}

func StartMachineNodeDeployTask(id int64, phase string) error {
	if !isValidMachineNodeDeployPhase(phase) {
		return fmt.Errorf("invalid node deployment phase: %s", phase)
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.ID(id).
		Where("status = ?", MachineNodeDeployStatusPending).
		Cols("status", "phase", "started_at", "updated_at").
		Update(&MachineNodeDeployTask{
			Status:    MachineNodeDeployStatusRunning,
			Phase:     phase,
			StartedAt: now,
			UpdatedAt: now,
		})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("node deployment task %d is not pending", id)
	}
	return nil
}

func UpdateMachineNodeDeployTaskPhase(id int64, phase string) error {
	if !isValidMachineNodeDeployPhase(phase) {
		return fmt.Errorf("invalid node deployment phase: %s", phase)
	}
	_, err := ormer.Engine.ID(id).
		Where("status = ?", MachineNodeDeployStatusRunning).
		Cols("phase", "updated_at").
		Update(&MachineNodeDeployTask{
			Phase:     phase,
			UpdatedAt: time.Now().UTC(),
		})
	return err
}

func FinishMachineNodeDeployTask(id int64, success bool, phase, errorMsg string) error {
	if !isValidMachineNodeDeployPhase(phase) {
		return fmt.Errorf("invalid node deployment phase: %s", phase)
	}
	status := MachineNodeDeployStatusSucceeded
	if !success {
		status = MachineNodeDeployStatusFailed
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.ID(id).
		Where("status IN (?, ?)", MachineNodeDeployStatusPending, MachineNodeDeployStatusRunning).
		Cols("status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&MachineNodeDeployTask{
			Status:     status,
			Phase:      phase,
			ErrorMsg:   errorMsg,
			FinishedAt: now,
			UpdatedAt:  now,
		})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("node deployment task %d is already finished", id)
	}
	return nil
}

func AddMachineNodeDeployLog(taskId int64, level, message string) error {
	if level != MachineNodeDeployLogLevelInfo && level != MachineNodeDeployLogLevelError {
		return fmt.Errorf("invalid node deployment log level: %s", level)
	}
	_, err := ormer.Engine.Insert(&MachineNodeDeployLog{
		TaskId:    taskId,
		Level:     level,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	})
	return err
}

func GetMachineNodeDeployLogs(taskId int64, limit int) ([]*MachineNodeDeployLog, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must not be negative")
	}
	if limit == 0 {
		limit = 500
	} else if limit > 1000 {
		return nil, fmt.Errorf("limit must not exceed 1000")
	}
	logs := []*MachineNodeDeployLog{}
	err := ormer.Engine.Where("task_id = ?", taskId).Asc("id").Limit(limit).Find(&logs)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func GetMachineNodeDeployCredential(owner, machineName string) (*MachineNodeDeployCredential, error) {
	credential := &MachineNodeDeployCredential{Owner: owner, MachineName: machineName}
	found, err := ormer.Engine.Get(credential)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	privateKey, err := decryptMachineNodePrivateKey(owner, machineName, credential.PrivateKey)
	if err != nil {
		return nil, err
	}
	credential.PrivateKey = privateKey
	return credential, nil
}

func UpdateMachineNodeDeployCredential(machine *Machine, privateKey string) error {
	if strings.TrimSpace(privateKey) == "" {
		return fmt.Errorf("private key must not be empty")
	}
	if _, err := ssh.ParsePrivateKey([]byte(privateKey)); err != nil {
		return fmt.Errorf("invalid SSH private key: %w", err)
	}
	encryptedKey, err := encryptMachineNodePrivateKey(machine.Owner, machine.Name, privateKey)
	if err != nil {
		return err
	}
	credential := &MachineNodeDeployCredential{
		Owner:       machine.Owner,
		MachineName: machine.Name,
		PrivateKey:  encryptedKey,
		UpdatedAt:   time.Now().UTC(),
	}
	if _, err = withMachineNodeDeployTransaction(func(session *xorm.Session) (interface{}, error) {
		lockedMachine := &Machine{}
		foundMachine, err := session.
			Where("owner = ? AND name = ?", machine.Owner, machine.Name).
			ForUpdate().
			Get(lockedMachine)
		if err != nil {
			return nil, err
		}
		if !foundMachine {
			return nil, fmt.Errorf("machine not found")
		}

		affected, err := session.Where("owner = ? AND machine_name = ?", machine.Owner, machine.Name).
			Cols("private_key", "updated_at").
			Update(credential)
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			if _, err = session.Insert(credential); err != nil {
				return nil, err
			}
		}

		stored := &MachineNodeDeployCredential{Owner: machine.Owner, MachineName: machine.Name}
		foundCredential, err := session.Get(stored)
		if err != nil {
			return nil, err
		}
		if !foundCredential {
			return nil, fmt.Errorf("managed SSH credential was not stored")
		}
		if _, err := decryptMachineNodePrivateKey(machine.Owner, machine.Name, stored.PrivateKey); err != nil {
			return nil, fmt.Errorf("verify managed SSH credential: %w", err)
		}

		role := "worker"
		if lockedMachine.Role != "" && lockedMachine.Role != "worker" {
			role = lockedMachine.Role
		}
		_, err = session.Where("owner = ? AND name = ?", machine.Owner, machine.Name).
			Cols("password", "private_key", "status", "role").
			Update(&Machine{
				Password:   "",
				PrivateKey: "",
				Status:     MachineStatusDeployed,
				Role:       role,
			})
		return nil, err
	}); err != nil {
		return err
	}
	return nil
}

func encryptMachineNodePrivateKey(owner, machineName, privateKey string) (string, error) {
	aead, err := machineNodeCredentialAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	data := aead.Seal(nonce, nonce, []byte(privateKey), []byte(owner+"/"+machineName))
	return machineNodeCredentialPrefix + base64.StdEncoding.EncodeToString(data), nil
}

func decryptMachineNodePrivateKey(owner, machineName, encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, machineNodeCredentialPrefix) {
		logs.Warning("machine node deploy credential for %s/%s has unrecognized format", owner, machineName)
		return "", fmt.Errorf("node deployment credential has unrecognized format")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, machineNodeCredentialPrefix))
	if err != nil {
		return "", err
	}
	aead, err := machineNodeCredentialAEAD()
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", fmt.Errorf("invalid managed ssh key payload")
	}
	nonce := raw[:aead.NonceSize()]
	ciphertext := raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, []byte(owner+"/"+machineName))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func machineNodeCredentialAEAD() (cipher.AEAD, error) {
	secret, err := machineNodeCredentialSecret()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func machineNodeCredentialSecret() (string, error) {
	machineNodeCredentialSecretMutex.Lock()
	defer machineNodeCredentialSecretMutex.Unlock()

	if secret := strings.TrimSpace(conf.GetConfigString("nodeDeploySecretKey")); secret != "" {
		return secret, nil
	}

	dataDir := strings.TrimSpace(conf.GetConfigString("dataDir"))
	if dataDir == "" {
		dataDir = "/var/lib/casos"
	}
	secretPath := filepath.Join(dataDir, machineNodeCredentialKeyFile)
	if data, err := os.ReadFile(secretPath); err == nil {
		secret := strings.TrimSpace(string(data))
		if secret == "" {
			logs.Error("node deployment credential secret is empty: %s", secretPath)
			return "", fmt.Errorf("node deployment credential secret is empty")
		}
		return secret, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	secret := base64.StdEncoding.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	tmpFile, err := os.CreateTemp(dataDir, machineNodeCredentialKeyFile+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return "", err
	}
	if _, err := tmpFile.WriteString(secret + "\n"); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, secretPath); err != nil {
		return "", err
	}
	return secret, nil
}

func UpdateMachineNodeDeployStatus(owner, name, status string) error {
	if !isValidMachineStatus(status) {
		return fmt.Errorf("invalid machine status: %s", status)
	}
	_, err := ormer.Engine.Where("owner = ? AND name = ?", owner, name).
		Cols("status").
		Update(&Machine{Status: status})
	return err
}

func isValidMachineStatus(status string) bool {
	switch status {
	case MachineStatusDeploying, MachineStatusDeployed, MachineStatusFailed:
		return true
	default:
		return false
	}
}

func withMachineNodeDeployTransaction(fn func(*xorm.Session) (interface{}, error)) (interface{}, error) {
	session := ormer.Engine.NewSession()
	defer func() {
		if v := recover(); v != nil {
			_ = session.Rollback()
			session.Close()
			panic(v)
		}
		session.Close()
	}()

	if err := session.Begin(); err != nil {
		return nil, err
	}
	result, err := fn(session)
	if err != nil {
		_ = session.Rollback()
		return nil, err
	}
	if err = session.Commit(); err != nil {
		_ = session.Rollback()
		return nil, err
	}
	return result, nil
}
