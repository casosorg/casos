package conf

import (
	"fmt"
	"strings"
)

const (
	AuthModeLocal   = "local"
	AuthModeCasdoor = "casdoor"
)

// GetAuthModeSafe returns the configured authentication mode or an error.
// Invalid values are rejected instead of silently downgrading authentication.
func GetAuthModeSafe() (string, error) {
	mode := strings.ToLower(strings.TrimSpace(GetConfigString("authMode")))
	if mode == "" {
		// Preserve existing Casdoor installations created before authMode was
		// introduced. New installations set authMode=local explicitly.
		if strings.TrimSpace(GetConfigString("casdoorEndpoint")) != "" {
			return AuthModeCasdoor, nil
		}
		return AuthModeLocal, nil
	}

	switch mode {
	case AuthModeLocal, AuthModeCasdoor:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid authMode %q: expected local or casdoor", mode)
	}
}

// GetAuthMode returns the configured authentication mode, panicking on an
// invalid value so misconfiguration fails fast at startup.
func GetAuthMode() string {
	mode, err := GetAuthModeSafe()
	if err != nil {
		panic(err)
	}
	return mode
}

func IsLocalAuthMode() bool {
	return GetAuthMode() == AuthModeLocal
}
