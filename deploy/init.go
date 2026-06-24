package deploy

import (
	"context"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	"k8s.io/client-go/rest"
)

var defaultService = NewService()

func DefaultService() *Service {
	return defaultService
}

func Init(ctx context.Context, config Config) {
	defaultService.SetContext(ctx)
	defaultService.SetConfig(config)
	if err := object.FailActiveMachineNodeDeployTasks("node deployment task was interrupted by service restart"); err != nil {
		logs.Warning("machine node deployment task cleanup: %v", err)
	}
}

func SetRestConfig(restConfig *rest.Config) {
	defaultService.SetRestConfig(restConfig)
}
