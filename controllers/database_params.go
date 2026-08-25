package controllers

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

const (
	// The tuned values, as the reader stated them. The container's own command
	// line is how the engine is told; this is the record of what was asked for,
	// which is what a form has to read back.
	databaseParamsAnnotation = "casos.io/db-params"
	// What was changed and when. Kubernetes keeps no history of its own for a
	// StatefulSet's command line, so a change nobody wrote down is a change
	// nobody can explain later.
	databaseParamHistoryAnnotation = "casos.io/db-param-history"
)

// How much history is worth carrying on an object that is read on every page load.
const databaseParamHistoryLimit = 20

type databaseParamChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

type databaseParamHistoryEntry struct {
	At      string                `json:"at"`
	Changes []databaseParamChange `json:"changes"`
}

func readDatabaseParams(meta metav1.ObjectMeta) map[string]string {
	values := map[string]string{}
	if raw := meta.Annotations[databaseParamsAnnotation]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &values)
	}
	return values
}

func readDatabaseParamHistory(meta metav1.ObjectMeta) []databaseParamHistoryEntry {
	history := []databaseParamHistoryEntry{}
	if raw := meta.Annotations[databaseParamHistoryAnnotation]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &history)
	}
	return history
}

func writeDatabaseParams(meta *metav1.ObjectMeta, values map[string]string, history []databaseParamHistoryEntry) {
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	if encoded, err := json.Marshal(values); err == nil {
		meta.Annotations[databaseParamsAnnotation] = string(encoded)
	}
	if len(history) > databaseParamHistoryLimit {
		history = history[:databaseParamHistoryLimit]
	}
	if encoded, err := json.Marshal(history); err == nil {
		meta.Annotations[databaseParamHistoryAnnotation] = string(encoded)
	}
}

// effectiveValue is what the engine is running with for one setting: what was
// asked for, or the engine's own default when nothing was.
func effectiveValue(param databaseParam, values map[string]string) string {
	if value, ok := values[param.Key]; ok && value != "" {
		return value
	}
	return param.Default
}

type databaseParamsResponse struct {
	Engine  string                      `json:"engine"`
	Params  []databaseParam             `json:"params"`
	Values  map[string]string           `json:"values"`
	History []databaseParamHistoryEntry `json:"history"`
}

// GetDatabaseParams returns the settings this engine exposes, what they are set
// to, and what has been changed before.
// @router /api/get-database-params [get]
func (c *ApiController) GetDatabaseParams() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	if namespace == "" {
		namespace = "default"
	}
	name := c.GetString("name")

	sts, err := object.GetStatefulSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", namespace, name))
		return
	}
	engine, ok := engineByKey(sts.Labels[databaseEngineLabel])
	if !ok {
		c.ResponseError("unknown database engine " + sts.Labels[databaseEngineLabel])
		return
	}

	stored := readDatabaseParams(sts.ObjectMeta)
	values := map[string]string{}
	for _, param := range engine.Params {
		values[param.Key] = effectiveValue(param, stored)
	}

	c.ResponseOk(databaseParamsResponse{
		Engine:  engine.Key,
		Params:  engine.Params,
		Values:  values,
		History: readDatabaseParamHistory(sts.ObjectMeta),
	})
}

// ConfigureDatabase applies tuned engine settings. The engine reads them at
// startup, so the change rolls the pod — which is why it is a deliberate action
// of its own rather than part of editing the database.
// @router /api/configure-database [post]
func (c *ApiController) ConfigureDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	var req struct {
		Namespace string            `json:"namespace"`
		Name      string            `json:"name"`
		Params    map[string]string `json:"params"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		c.ResponseError("name is required")
		return
	}

	sts, err := object.GetStatefulSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", req.Namespace, req.Name))
		return
	}
	engine, ok := engineByKey(sts.Labels[databaseEngineLabel])
	if !ok {
		c.ResponseError("unknown database engine " + sts.Labels[databaseEngineLabel])
		return
	}
	if len(sts.Spec.Template.Spec.Containers) == 0 {
		c.ResponseError("the database has no container to configure")
		return
	}

	previous := readDatabaseParams(sts.ObjectMeta)
	next := map[string]string{}
	changes := []databaseParamChange{}
	for _, param := range engine.Params {
		value, given := req.Params[param.Key]
		if !given || value == "" {
			value = param.Default
		}
		if err := param.validate(value); err != nil {
			c.ResponseError(err.Error())
			return
		}
		// Only what differs from the engine's own default is worth storing; the
		// rest is the engine being left alone.
		if value != param.Default {
			next[param.Key] = value
		}
		if before := effectiveValue(param, previous); before != value {
			changes = append(changes, databaseParamChange{Key: param.Key, From: before, To: value})
		}
	}

	if len(changes) == 0 {
		c.ResponseOk(map[string]any{"changed": 0})
		return
	}

	if err := applyDatabaseParams(&sts.Spec.Template.Spec.Containers[0], engine, next); err != nil {
		c.ResponseError(err.Error())
		return
	}

	history := append([]databaseParamHistoryEntry{{
		At:      time.Now().UTC().Format("2006-01-02 15:04:05"),
		Changes: changes,
	}}, readDatabaseParamHistory(sts.ObjectMeta)...)
	writeDatabaseParams(&sts.ObjectMeta, next, history)

	if _, err := object.UpdateStatefulSet(cfg, sts); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(map[string]any{"changed": len(changes)})
}
