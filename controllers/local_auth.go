package controllers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

const (
	maxSigninFailures       = 5
	signinFailureWindow     = 5 * time.Minute
	signinBlockTime         = 5 * time.Minute
	signinAttemptMaxEntries = 1000
)

type localSigninForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type localSetupForm struct {
	SetupToken string `json:"setupToken"`
	Password   string `json:"password"`
}

type localAccountForm struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type signinAttempt struct {
	failures     int
	firstFailure time.Time
	blockedUntil time.Time
}

type signinAttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]signinAttempt
}

var localSigninLimiter = signinAttemptLimiter{attempts: map[string]signinAttempt{}}

var (
	localSetupTokenMu sync.RWMutex
	localSetupToken   string
)

// InitLocalSetup prepares the one-time token required to create the first
// administrator. The token is intentionally never returned by an API.
func InitLocalSetup() error {
	if !conf.IsLocalAuthMode() {
		return nil
	}
	initialized, err := object.IsLocalAdminInitialized()
	if err != nil || initialized {
		return err
	}

	token := strings.TrimSpace(conf.GetConfigString("localSetupToken"))
	if token != "" && len(token) < 16 {
		return errors.New("localSetupToken must contain at least 16 characters")
	}
	if token == "" {
		token, err = generateAuthToken(24)
		if err != nil {
			return err
		}
		logs.Warning("local administrator setup token: %s", token)
		logs.Warning("this token is only needed for first-run setup from another machine")
	} else {
		logs.Info("local administrator setup will use the configured localSetupToken")
	}

	localSetupTokenMu.Lock()
	localSetupToken = token
	localSetupTokenMu.Unlock()
	return nil
}

func (l *signinAttemptLimiter) begin(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attempts) > signinAttemptMaxEntries {
		for k, attempt := range l.attempts {
			if !attempt.blockedUntil.IsZero() && !now.Before(attempt.blockedUntil) {
				delete(l.attempts, k)
				continue
			}
			if !attempt.firstFailure.IsZero() && now.Sub(attempt.firstFailure) >= signinFailureWindow {
				delete(l.attempts, k)
			}
		}
	}
	attempt := l.attempts[key]
	if !attempt.blockedUntil.IsZero() && now.Before(attempt.blockedUntil) {
		return false
	}
	if !attempt.blockedUntil.IsZero() {
		attempt = signinAttempt{}
	}
	if !attempt.firstFailure.IsZero() && now.Sub(attempt.firstFailure) >= signinFailureWindow {
		attempt = signinAttempt{}
	}
	if attempt.firstFailure.IsZero() {
		attempt = signinAttempt{firstFailure: now}
	}
	attempt.failures++
	if attempt.failures >= maxSigninFailures {
		attempt.blockedUntil = now.Add(signinBlockTime)
	}
	l.attempts[key] = attempt
	return true
}

func (l *signinAttemptLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.attempts[key]
	if !ok {
		return
	}
	attempt.failures--
	if attempt.failures < maxSigninFailures {
		attempt.blockedUntil = time.Time{}
	}
	if attempt.failures <= 0 {
		delete(l.attempts, key)
		return
	}
	l.attempts[key] = attempt
}

func (l *signinAttemptLimiter) success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (c *ApiController) GetSigninOptions() {
	mode, err := conf.GetAuthModeSafe()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	options := map[string]interface{}{
		"authMode":           mode,
		"casdoorAvailable":   mode == conf.AuthModeCasdoor,
		"localAvailable":     false,
		"setupRequired":      false,
		"setupTokenRequired": false,
	}

	if mode == conf.AuthModeCasdoor {
		oauthState, err := c.getOrCreateOAuthState()
		if err != nil {
			c.ResponseError("failed to create OAuth state")
			return
		}
		options["authConfig"] = map[string]string{
			"serverUrl":        conf.GetConfigString("casdoorEndpoint"),
			"clientId":         conf.GetConfigString("clientId"),
			"appName":          conf.GetConfigString("casdoorApplication"),
			"organizationName": conf.GetConfigString("casdoorOrganization"),
			"redirectPath":     "/callback",
		}
		options["oauthState"] = oauthState
		c.ResponseOk(options)
		return
	}

	initialized, err := object.IsLocalAdminInitialized()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	options["localAvailable"] = initialized
	options["setupRequired"] = !initialized
	// First-run setup from the machine running CasOS needs no token; requests
	// arriving through the network or a reverse proxy still must prove they own
	// this instance.
	options["setupTokenRequired"] = !initialized && firstRunRequiresToken(
		c.Ctx.Request.RemoteAddr,
		c.Ctx.Request.Header.Get("X-Forwarded-For"),
		c.Ctx.Request.Header.Get("X-Real-IP"),
	)
	c.ResponseOk(options)
}

func (c *ApiController) Setup() {
	if !conf.IsLocalAuthMode() {
		c.ResponseError("local setup is unavailable in Casdoor mode")
		return
	}

	form := localSetupForm{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError("invalid setup request")
		return
	}
	c.Ctx.Input.RequestBody = []byte(`{"setupToken":"***","password":"***"}`)
	if firstRunRequiresToken(
		c.Ctx.Request.RemoteAddr,
		c.Ctx.Request.Header.Get("X-Forwarded-For"),
		c.Ctx.Request.Header.Get("X-Real-IP"),
	) && !validLocalSetupToken(form.SetupToken) {
		c.ResponseError("invalid setup token")
		return
	}

	user, err := object.CreateLocalAdmin(form.Password)
	if err != nil {
		if errors.Is(err, object.ErrLocalAdminAlreadyInitialized) {
			c.ResponseError(err.Error())
			return
		}
		c.ResponseError(err.Error())
		return
	}
	clearLocalSetupToken()
	claims := localUserClaims(user)
	if !c.establishSession(claims) {
		return
	}
	c.ResponseOk(claims)
}

func (c *ApiController) signinWithPassword() {
	form := localSigninForm{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError("invalid sign-in request")
		return
	}
	if sanitizedBody, err := json.Marshal(localSigninForm{Username: form.Username, Password: "***"}); err == nil {
		c.Ctx.Input.RequestBody = sanitizedBody
	}

	key := signinAttemptKey(c.Ctx.Request.RemoteAddr)
	now := time.Now()
	if !localSigninLimiter.begin(key, now) {
		c.ResponseError("too many sign-in attempts; try again later")
		return
	}

	user, ok, err := object.VerifyLocalUser(form.Username, form.Password)
	if err != nil {
		localSigninLimiter.release(key)
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("invalid username or password")
		return
	}

	localSigninLimiter.success(key)
	claims := localUserClaims(user)
	if !c.establishSession(claims) {
		return
	}
	c.ResponseOk(claims)
}

func (c *ApiController) UpdateAccount() {
	if !conf.IsLocalAuthMode() {
		c.ResponseError("local account management is unavailable in Casdoor mode")
		return
	}
	sessionUser := c.GetSessionUser()
	if sessionUser == nil || sessionUser.Owner != object.LocalUserOwner {
		c.ResponseError("unauthorized operation")
		return
	}

	form := localAccountForm{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError("invalid account request")
		return
	}
	c.Ctx.Input.RequestBody = []byte(`{"currentPassword":"***","newPassword":"***"}`)

	user, err := object.GetLocalUser(sessionUser.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if user == nil || !object.CheckLocalUserPassword(user, form.CurrentPassword) {
		c.ResponseError("current password is incorrect")
		return
	}
	if err = object.UpdateLocalUserPassword(user, form.NewPassword); err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims := localUserClaims(user)
	if !c.establishSession(claims) {
		return
	}
	c.ResponseOk(claims)
}

func localUserClaims(user *object.LocalUser) *casdoorsdk.Claims {
	return &casdoorsdk.Claims{
		User: casdoorsdk.User{
			Owner:       user.Owner,
			Name:        user.Name,
			Id:          user.Owner + "/" + user.Name,
			CreatedTime: user.CreatedTime,
			UpdatedTime: user.UpdatedTime,
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
		},
	}
}

func (c *ApiController) establishSession(claims *casdoorsdk.Claims) bool {
	if err := c.SessionRegenerateID(); err != nil {
		c.ResponseError("failed to create session")
		return false
	}
	c.SetSessionClaims(claims)
	return true
}

func signinAttemptKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return strings.TrimSpace(host)
}

func isLoopbackRequest(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// firstRunRequiresToken decides whether a first-run setup request must present
// the one-time setup token. Forwarded-IP headers fail safe: when present, the
// request is treated as remote even if the immediate peer is the loopback,
// which closes the same-host reverse-proxy bypass.
func firstRunRequiresToken(remoteAddr, xForwardedFor, xRealIP string) bool {
	if strings.TrimSpace(xForwardedFor) != "" || strings.TrimSpace(xRealIP) != "" {
		return true
	}
	return !isLoopbackRequest(remoteAddr)
}

func validLocalSetupToken(candidate string) bool {
	localSetupTokenMu.RLock()
	expected := localSetupToken
	localSetupTokenMu.RUnlock()
	if expected == "" || len(candidate) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}

func clearLocalSetupToken() {
	localSetupTokenMu.Lock()
	localSetupToken = ""
	localSetupTokenMu.Unlock()
}

func generateAuthToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (c *ApiController) getOrCreateOAuthState() (string, error) {
	if state, ok := c.GetSession("oauthState").(string); ok && state != "" {
		return state, nil
	}
	state, err := generateAuthToken(24)
	if err != nil {
		return "", err
	}
	c.SetSession("oauthState", state)
	return state, nil
}

func (c *ApiController) validateOAuthState(state string) bool {
	expected, ok := c.GetSession("oauthState").(string)
	if !ok || expected == "" || len(state) != len(expected) {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(expected)) != 1 {
		return false
	}
	c.DelSession("oauthState")
	return true
}
