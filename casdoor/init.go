package casdoor

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
)

//go:embed token_jwt_key.pem
var JwtPublicKey string

func InitCasdoorConfig() error {
	if conf.IsLocalAuthMode() {
		return nil
	}

	casdoorEndpoint := conf.GetConfigString("casdoorEndpoint")
	clientId := conf.GetConfigString("clientId")
	clientSecret := conf.GetConfigString("clientSecret")
	casdoorOrganization := conf.GetConfigString("casdoorOrganization")
	casdoorApplication := conf.GetConfigString("casdoorApplication")
	values := map[string]string{
		"casdoorEndpoint":     casdoorEndpoint,
		"clientId":            clientId,
		"clientSecret":        clientSecret,
		"casdoorOrganization": casdoorOrganization,
		"casdoorApplication":  casdoorApplication,
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required when authMode=casdoor", key)
		}
	}
	casdoorsdk.InitConfig(casdoorEndpoint, clientId, clientSecret, JwtPublicKey, casdoorOrganization, casdoorApplication)
	return nil
}
