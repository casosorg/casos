package object

type ContainerTemplate struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Discription string `json:"description"`
	Icon        string `json:"icon"`
	Image       string `json:"image"`
	DefaultPort int32  `json:"defaultPort"`
}

func GetContainerTemplates() []ContainerTemplate {
	return []ContainerTemplate{
		{
			Name:        "firefox",
			DisplayName: "Firefox",
			Discription: "Firefox browser with noVNC web UI (jlesage/firefox).",
			Icon:        "🦊",
			Image:       "docker.io/jlesage/firefox:latest",
			DefaultPort: 5800,
		},
	}
}
