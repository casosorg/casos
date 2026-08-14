package authn

import (
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

func SessionClaims(value interface{}) (*casdoorsdk.Claims, bool) {
	switch claims := value.(type) {
	case casdoorsdk.Claims:
		return &claims, true
	case *casdoorsdk.Claims:
		return claims, claims != nil
	default:
		return nil, false
	}
}

// ValidateSessionClaims rejects sessions created by a different auth mode and
// invalidates old local sessions after the administrator changes the password.
func ValidateSessionClaims(claims *casdoorsdk.Claims) bool {
	if claims == nil || claims.User.Name == "" {
		return false
	}
	if claims.User.Owner == "e2e" {
		return conf.GetConfigBool("e2eTestMode") && claims.User.Name == "ci-user"
	}

	if conf.IsLocalAuthMode() {
		if claims.User.Owner != object.LocalUserOwner {
			return false
		}
		user, err := object.GetLocalUser(claims.User.Name)
		if err != nil || user == nil || user.IsDeleted || user.IsForbidden || !user.IsAdmin {
			return false
		}
		return claims.User.UpdatedTime == user.UpdatedTime
	}

	if claims.User.Owner == object.LocalUserOwner || claims.User.IsDeleted || claims.User.IsForbidden {
		return false
	}
	return claims.ExpiresAt != nil && claims.ExpiresAt.Time.After(time.Now())
}
