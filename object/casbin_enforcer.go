package object

import (
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// Allow-and-deny model: allowed when at least one allow rule matches AND no deny rule matches.
const casbinModelText = `
[request_definition]
r = sub, ns, resource, action

[policy_definition]
p = sub, ns, resource, action, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (g(r.sub, p.sub) || r.sub == p.sub || p.sub == "*") && (p.ns == "*" || r.ns == p.ns) && (p.resource == "*" || r.resource == p.resource) && (p.action == "*" || r.action == p.action)
`

type scopedEnforcer struct {
	mu sync.RWMutex
	e  *casbin.Enforcer
}

func (se *scopedEnforcer) reload(scope string) error {
	rules, err := GetCasbinRules(scope)
	if err != nil {
		return err
	}
	m, err := model.NewModelFromString(casbinModelText)
	if err != nil {
		return err
	}
	e, err := casbin.NewEnforcer(m, &dbAdapter{rules: rules})
	if err != nil {
		return err
	}
	se.mu.Lock()
	se.e = e
	se.mu.Unlock()
	return nil
}

func (se *scopedEnforcer) enforce(user, namespace, resource, action string) (bool, error) {
	se.mu.RLock()
	e := se.e
	se.mu.RUnlock()
	if e == nil {
		return true, nil
	}
	return e.Enforce(user, namespace, resource, action)
}

var (
	admissionEnforcer     = &scopedEnforcer{}
	authorizationEnforcer = &scopedEnforcer{}
)

// ReloadEnforcer rebuilds the enforcer for the given scope.
func ReloadEnforcer(scope string) error {
	switch scope {
	case ScopeAdmission:
		return admissionEnforcer.reload(scope)
	case ScopeAuthorization:
		return authorizationEnforcer.reload(scope)
	default:
		return nil
	}
}

// ReloadAllEnforcers rebuilds both enforcers; called at startup.
func ReloadAllEnforcers() error {
	if err := admissionEnforcer.reload(ScopeAdmission); err != nil {
		return err
	}
	return authorizationEnforcer.reload(ScopeAuthorization)
}

func EnforceAdmissionPolicy(user, namespace, resource, action string) (bool, error) {
	return admissionEnforcer.enforce(user, namespace, resource, action)
}

func EnforceAuthorizationPolicy(user, namespace, resource, action string) (bool, error) {
	return authorizationEnforcer.enforce(user, namespace, resource, action)
}

// dbAdapter loads Casbin policy from the in-memory rule slice.
type dbAdapter struct{ rules []*CasbinRule }

func (a *dbAdapter) LoadPolicy(m model.Model) error {
	for _, r := range a.rules {
		var line string
		if r.PType == "p" {
			eft := r.V4
			if eft == "" {
				eft = "allow"
			}
			line = "p, " + r.V0 + ", " + r.V1 + ", " + r.V2 + ", " + r.V3 + ", " + eft
		} else {
			line = "g, " + r.V0 + ", " + r.V1
		}
		persist.LoadPolicyLine(line, m)
	}
	return nil
}

func (a *dbAdapter) SavePolicy(model.Model) error                              { return nil }
func (a *dbAdapter) AddPolicy(string, string, []string) error                  { return nil }
func (a *dbAdapter) RemovePolicy(string, string, []string) error               { return nil }
func (a *dbAdapter) RemoveFilteredPolicy(string, string, int, ...string) error { return nil }
