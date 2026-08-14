package authn

import (
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/golang-jwt/jwt/v4"
)

func TestSessionClaims(t *testing.T) {
	claims := casdoorsdk.Claims{User: casdoorsdk.User{Name: "admin"}}
	if got, ok := SessionClaims(claims); !ok || got.Name != "admin" {
		t.Fatalf("SessionClaims(value) = (%#v, %t)", got, ok)
	}
	if got, ok := SessionClaims(&claims); !ok || got.Name != "admin" {
		t.Fatalf("SessionClaims(pointer) = (%#v, %t)", got, ok)
	}
	if _, ok := SessionClaims("invalid"); ok {
		t.Fatal("SessionClaims accepted an unrelated session value")
	}
}

func TestValidateSessionClaimsHonorsAuthMode(t *testing.T) {
	casdoorClaims := &casdoorsdk.Claims{
		User:             casdoorsdk.User{Owner: "built-in", Name: "admin"},
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	localClaims := &casdoorsdk.Claims{User: casdoorsdk.User{Owner: "casos-local", Name: "admin"}}

	t.Setenv("authMode", "casdoor")
	if !ValidateSessionClaims(casdoorClaims) {
		t.Fatal("Casdoor session was rejected in Casdoor mode")
	}
	if ValidateSessionClaims(localClaims) {
		t.Fatal("local session was accepted in Casdoor mode")
	}
}

func TestValidateSessionClaimsRejectsExpiredCasdoorSession(t *testing.T) {
	t.Setenv("authMode", "casdoor")
	claims := &casdoorsdk.Claims{
		User:             casdoorsdk.User{Owner: "built-in", Name: "admin"},
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))},
	}
	if ValidateSessionClaims(claims) {
		t.Fatal("expired Casdoor session was accepted")
	}
}

func TestValidateSessionClaimsAllowsE2EIdentityOnlyInTestMode(t *testing.T) {
	t.Setenv("authMode", "local")
	t.Setenv("e2eTestMode", "true")
	claims := &casdoorsdk.Claims{User: casdoorsdk.User{Owner: "e2e", Name: "ci-user"}}
	if !ValidateSessionClaims(claims) {
		t.Fatal("E2E identity was rejected while E2E test mode was enabled")
	}
}

func TestValidateSessionClaimsRejectsE2EIdentityOutsideTestMode(t *testing.T) {
	t.Setenv("authMode", "casdoor")
	t.Setenv("e2eTestMode", "false")
	claims := &casdoorsdk.Claims{User: casdoorsdk.User{Owner: "e2e", Name: "ci-user"}}
	if ValidateSessionClaims(claims) {
		t.Fatal("E2E identity was accepted while E2E test mode was disabled")
	}
}
