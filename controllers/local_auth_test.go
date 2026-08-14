package controllers

import (
	"fmt"
	"testing"
	"time"
)

func TestSigninAttemptLimiter(t *testing.T) {
	limiter := signinAttemptLimiter{attempts: map[string]signinAttempt{}}
	key := "127.0.0.1\x00admin"
	now := time.Unix(1000, 0)

	for i := 0; i < maxSigninFailures; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("attempt %d was blocked too early", i+1)
		}
	}
	if limiter.begin(key, now) {
		t.Fatal("limiter allowed a blocked key")
	}
	if !limiter.begin(key, now.Add(signinBlockTime)) {
		t.Fatal("limiter did not unblock the key after the block period")
	}

	limiter.success(key)
	if !limiter.begin(key, now.Add(signinBlockTime)) {
		t.Fatal("successful sign-in did not clear failures")
	}
}

func TestSigninAttemptLimiterReleasesBackendErrors(t *testing.T) {
	limiter := signinAttemptLimiter{attempts: map[string]signinAttempt{}}
	now := time.Unix(1000, 0)
	if !limiter.begin("key", now) {
		t.Fatal("first attempt was blocked")
	}
	limiter.release("key")
	if got := len(limiter.attempts); got != 0 {
		t.Fatalf("attempt map length = %d, want 0", got)
	}
}

func TestSigninAttemptKey(t *testing.T) {
	if got := signinAttemptKey("127.0.0.1:12345"); got != "127.0.0.1" {
		t.Fatalf("signinAttemptKey() = %q", got)
	}
}

func TestIsLoopbackRequest(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       bool
	}{
		{remoteAddr: "127.0.0.1:9000", want: true},
		{remoteAddr: "[::1]:9000", want: true},
		{remoteAddr: "[::ffff:127.0.0.1]:9000", want: true},
		{remoteAddr: "192.168.1.10:9000", want: false},
		{remoteAddr: "10.0.0.2:22", want: false},
		{remoteAddr: "", want: false},
	}
	for _, tt := range tests {
		if got := isLoopbackRequest(tt.remoteAddr); got != tt.want {
			t.Fatalf("isLoopbackRequest(%q) = %t, want %t", tt.remoteAddr, got, tt.want)
		}
	}
}

func TestFirstRunRequiresToken(t *testing.T) {
	tests := []struct {
		name          string
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		want          bool
	}{
		{name: "local direct access", remoteAddr: "127.0.0.1:9000", want: false},
		{name: "local IPv6 direct access", remoteAddr: "[::1]:9000", want: false},
		{name: "remote direct access", remoteAddr: "192.168.1.10:9000", want: true},
		{name: "reverse proxy with X-Forwarded-For", remoteAddr: "127.0.0.1:9000", xForwardedFor: "203.0.113.7", want: true},
		{name: "reverse proxy with X-Real-IP", remoteAddr: "127.0.0.1:9000", xRealIP: "203.0.113.7", want: true},
		{name: "missing remote address", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstRunRequiresToken(tt.remoteAddr, tt.xForwardedFor, tt.xRealIP); got != tt.want {
				t.Fatalf("firstRunRequiresToken() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSigninAttemptLimiterSweepsExpiredEntries(t *testing.T) {
	limiter := signinAttemptLimiter{attempts: map[string]signinAttempt{}}
	now := time.Unix(1000, 0)
	for i := 0; i < signinAttemptMaxEntries+10; i++ {
		limiter.attempts[fmt.Sprintf("stale-%d", i)] = signinAttempt{firstFailure: now.Add(-signinFailureWindow - time.Minute)}
	}
	limiter.begin("fresh-key", now)
	if len(limiter.attempts) >= signinAttemptMaxEntries {
		t.Fatalf("attempt map length = %d, want sweep below %d", len(limiter.attempts), signinAttemptMaxEntries)
	}
	if _, ok := limiter.attempts["fresh-key"]; !ok {
		t.Fatal("fresh attempt was not recorded")
	}
}

func TestLocalSetupTokenValidation(t *testing.T) {
	localSetupTokenMu.Lock()
	localSetupToken = "test-setup-token-1234"
	localSetupTokenMu.Unlock()
	t.Cleanup(clearLocalSetupToken)

	if !validLocalSetupToken("test-setup-token-1234") {
		t.Fatal("valid setup token was rejected")
	}
	if validLocalSetupToken("wrong-setup-token-12") {
		t.Fatal("invalid setup token was accepted")
	}
}

func TestGenerateAuthToken(t *testing.T) {
	token, err := generateAuthToken(24)
	if err != nil {
		t.Fatalf("generateAuthToken(): %v", err)
	}
	if len(token) < 32 {
		t.Fatalf("generated token length = %d, want at least 32", len(token))
	}
}
