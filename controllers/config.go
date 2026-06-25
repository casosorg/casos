package controllers

import "github.com/casosorg/casos/conf"

type applicationConfig struct {
	AuthConfig authConfig `json:"authConfig"`
}

type authConfig struct {
	ServerURL        string `json:"serverUrl"`
	ClientID         string `json:"clientId"`
	AppName          string `json:"appName"`
	OrganizationName string `json:"organizationName"`
	RedirectPath     string `json:"redirectPath"`
}

func (c *ApiController) GetApplicationConfig() {
	c.ResponseOk(applicationConfig{
		AuthConfig: authConfig{
			ServerURL:        conf.GetConfigString("casdoorEndpoint"),
			ClientID:         conf.GetConfigString("clientId"),
			AppName:          conf.GetConfigString("casdoorApplication"),
			OrganizationName: conf.GetConfigString("casdoorOrganization"),
			RedirectPath:     "/callback",
		},
	})
}
