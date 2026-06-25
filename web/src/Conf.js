export const AuthConfig = {
  serverUrl: "",
  clientId: "",
  appName: "",
  organizationName: "",
  redirectPath: "/callback",
};

export const ThemeDefault = {
  colorPrimary: "#404040",
};

export function setConfig(config) {
  if (config === null || config === undefined) {
    return;
  }

  if (config.authConfig) {
    Object.assign(AuthConfig, config.authConfig);
  }

  if (config.themeDefault) {
    Object.assign(ThemeDefault, config.themeDefault);
  }
}
