package casdoor

import (
	_ "embed"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/conf"
)

//go:embed token_jwt_key.pem
var JwtPublicKey string

func InitCasdoorConfig() {
	casdoorEndpoint := conf.GetConfigString("casdoorEndpoint")
	clientId := conf.GetConfigString("clientId")
	clientSecret := conf.GetConfigString("clientSecret")
	casdoorOrganization := conf.GetConfigString("casdoorOrganization")
	casdoorApplication := conf.GetConfigString("casdoorApplication")
	casdoorsdk.InitConfig(
		casdoorEndpoint,
		clientId,
		clientSecret,
		JwtPublicKey,
		casdoorOrganization,
		casdoorApplication,
	)
}
