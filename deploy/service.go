package deploy

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	"k8s.io/client-go/rest"
)

const (
	defaultMachineNodeTaskLimit = 50
	defaultMachineNodeLogLimit  = 500
	machineNodeDeployTimeout    = 20 * time.Minute
)

type Service struct {
	mu         sync.RWMutex
	ctx        context.Context
	config     *Config
	restConfig *rest.Config
	semaphore  chan struct{}
}

func NewService() *Service {
	return &Service{
		ctx:       context.Background(),
		semaphore: make(chan struct{}, 5),
	}
}

func (s *Service) SetContext(ctx context.Context) {
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			logs.Warning("machine node deployment context is already canceled: %v", ctx.Err())
		}
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
}

func (s *Service) SetConfig(config Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = &config
}

func (s *Service) SetRestConfig(restConfig *rest.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restConfig = restConfig
}

func (s *Service) PreflightMachineNode(req MachineNodeDeployRequest) (map[string]interface{}, error) {
	config, err := s.configSnapshot()
	if err != nil {
		return nil, err
	}
	if _, err = s.restConfigSnapshot(); err != nil {
		return nil, err
	}
	_, deployMachine, err := s.prepareMachineNodeRequest(&req)
	if err != nil {
		return nil, err
	}
	if req.ApiserverURL == "" {
		resolved, err := resolveMachineNodeApiserverURL(deployMachine, defaultNodeDeployApiserverURL(config))
		if err != nil {
			return nil, err
		}
		req.ApiserverURL = resolved
	}
	deployer := NewNodeDeployer(config, nil, nil)
	ctx, cancel := context.WithTimeout(s.contextSnapshot(), 30*time.Second)
	defer cancel()
	result, err := deployer.Preflight(ctx, NodeDeployOptions{
		Machine:      deployMachine,
		NodeName:     req.NodeName,
		ApiserverURL: req.ApiserverURL,
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"nodeName":     req.NodeName,
		"apiserverUrl": req.ApiserverURL,
		"preflight":    result,
	}, nil
}

func (s *Service) DeployMachineNode(req MachineNodeDeployRequest) (*object.MachineNodeDeployTask, error) {
	config, err := s.configSnapshot()
	if err != nil {
		return nil, err
	}
	restConfig, err := s.restConfigSnapshot()
	if err != nil {
		return nil, err
	}
	machine, deployMachine, err := s.prepareMachineNodeRequest(&req)
	if err != nil {
		return nil, err
	}
	if req.ApiserverURL == "" {
		resolved, err := resolveMachineNodeApiserverURL(deployMachine, defaultNodeDeployApiserverURL(config))
		if err != nil {
			return nil, err
		}
		req.ApiserverURL = resolved
	}

	if !s.acquireDeploymentSlot() {
		return nil, fmt.Errorf("too many concurrent node deployments, try again later")
	}

	task, err := object.CreateMachineNodeDeployTask(machine.Owner, machine.Name, req.NodeName, req.ApiserverURL)
	if err != nil {
		s.releaseDeploymentSlot()
		return nil, err
	}
	if err = object.AddMachineNodeDeployLog(task.Id, object.MachineNodeDeployLogLevelInfo, "Node deployment task created"); err != nil {
		s.releaseDeploymentSlot()
		return nil, err
	}

	deployCtx := s.contextSnapshot()
	go func(ctx context.Context) {
		defer s.releaseDeploymentSlot()
		defer func() {
			if v := recover(); v != nil {
				message := fmt.Sprintf("Node deployment task panic: %v", v)
				logs.Error(message)
				cleanupMachineNodeDeployPanic(task.Id, machine.Owner, machine.Name, message)
			}
		}()
		s.runMachineNodeDeployTask(ctx, task.Id, config, restConfig)
	}(deployCtx)

	return task, nil
}

func (s *Service) RepairMachineNode(req MachineNodeDeployRequest) (*object.MachineNodeDeployTask, error) {
	return s.DeployMachineNode(req)
}

func (s *Service) GetMachineNodeTasks(owner, machineName string) ([]*object.MachineNodeDeployTask, error) {
	if owner == "" {
		owner = "admin"
	}
	return object.GetMachineNodeDeployTasks(owner, machineName, defaultMachineNodeTaskLimit)
}

func (s *Service) GetMachineNodeLogs(taskId int64) ([]*object.MachineNodeDeployLog, error) {
	if taskId <= 0 {
		return nil, fmt.Errorf("invalid taskId")
	}
	return object.GetMachineNodeDeployLogs(taskId, defaultMachineNodeLogLimit)
}

func (s *Service) prepareMachineNodeRequest(req *MachineNodeDeployRequest) (*object.Machine, NodeDeployMachine, error) {
	req.normalize()
	if err := req.validate(); err != nil {
		return nil, NodeDeployMachine{}, err
	}
	machine, err := getMachineForNodeDeploy(req.Owner, req.MachineName)
	if err != nil {
		return nil, NodeDeployMachine{}, err
	}
	if req.NodeName == "" {
		req.NodeName = machine.Name
	}
	if err := validateNodeDeployName(req.NodeName); err != nil {
		return nil, NodeDeployMachine{}, err
	}
	deployMachine, err := toNodeDeployMachine(machine)
	if err != nil {
		return nil, NodeDeployMachine{}, err
	}
	return machine, deployMachine, nil
}

func (s *Service) configSnapshot() (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil {
		return Config{}, fmt.Errorf("server config not ready")
	}
	return *s.config, nil
}

func (s *Service) restConfigSnapshot() (*rest.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.restConfig == nil {
		return nil, fmt.Errorf("apiserver not ready")
	}
	return rest.CopyConfig(s.restConfig), nil
}

func (s *Service) contextSnapshot() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *Service) acquireDeploymentSlot() bool {
	select {
	case s.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Service) releaseDeploymentSlot() {
	<-s.semaphore
}

func (s *Service) runMachineNodeDeployTask(parentCtx context.Context, taskId int64, config Config, restConfig *rest.Config) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	logTask := func(level, message string) {
		if err := object.AddMachineNodeDeployLog(taskId, level, message); err != nil {
			logs.Warning("failed to write node deployment log for task %d: %v", taskId, err)
		}
	}

	task, err := object.GetMachineNodeDeployTask(taskId)
	if err != nil || task == nil {
		logTask(object.MachineNodeDeployLogLevelError, "Failed to load node deployment task")
		return
	}
	machine, err := object.GetMachine(fmt.Sprintf("%s/%s", task.Owner, task.MachineName))
	if err != nil || machine == nil {
		logTask(object.MachineNodeDeployLogLevelError, "Failed to load machine")
		_ = object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, "machine not found")
		return
	}

	if err = object.StartMachineNodeDeployTask(taskId, object.MachineNodeDeployPhasePreflight); err != nil {
		logTask(object.MachineNodeDeployLogLevelError, err.Error())
		_ = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusFailed)
		if finishErr := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, err.Error()); finishErr != nil {
			logTask(object.MachineNodeDeployLogLevelError, "Failed to finish node deployment task: "+finishErr.Error())
		}
		return
	}
	if err = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusDeploying); err != nil {
		logTask(object.MachineNodeDeployLogLevelError, err.Error())
		if finishErr := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, err.Error()); finishErr != nil {
			logTask(object.MachineNodeDeployLogLevelError, "Failed to finish node deployment task: "+finishErr.Error())
		}
		return
	}
	deployer := NewNodeDeployer(config, restConfig, func(level, message, phase string) {
		logTask(level, message)
		if phase != "" {
			_ = object.UpdateMachineNodeDeployTaskPhase(taskId, phase)
		}
	})
	deployMachine, err := toNodeDeployMachine(machine)
	if err != nil {
		logTask(object.MachineNodeDeployLogLevelError, err.Error())
		_ = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusFailed)
		if finishErr := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, err.Error()); finishErr != nil {
			logTask(object.MachineNodeDeployLogLevelError, "Failed to finish node deployment task: "+finishErr.Error())
		}
		return
	}

	ctx, cancel := context.WithTimeout(parentCtx, machineNodeDeployTimeout)
	defer cancel()

	result, err := deployer.Deploy(ctx, NodeDeployOptions{
		Machine:      deployMachine,
		NodeName:     task.NodeName,
		ApiserverURL: task.ApiserverURL,
	})
	if err != nil {
		logTask(object.MachineNodeDeployLogLevelError, "Node deployment failed: "+err.Error())
		_ = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusFailed)
		if finishErr := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, err.Error()); finishErr != nil {
			logTask(object.MachineNodeDeployLogLevelError, "Failed to finish node deployment task: "+finishErr.Error())
		}
		return
	}
	if result != nil && result.ManagedPrivateKey != "" {
		if err = object.UpdateMachineNodeDeployCredential(machine, result.ManagedPrivateKey); err != nil {
			logTask(object.MachineNodeDeployLogLevelError, "Failed to save managed SSH key: "+err.Error())
			_ = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusFailed)
			if finishErr := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, err.Error()); finishErr != nil {
				logTask(object.MachineNodeDeployLogLevelError, "Failed to finish node deployment task: "+finishErr.Error())
			}
			return
		}
		logTask(object.MachineNodeDeployLogLevelInfo, "CasOS managed SSH key installed")
	}

	logTask(object.MachineNodeDeployLogLevelInfo, "Node deployment completed")
	if err = object.UpdateMachineNodeDeployStatus(machine.Owner, machine.Name, object.MachineStatusDeployed); err != nil {
		logTask(object.MachineNodeDeployLogLevelError, "Failed to update machine status after completed node deployment: "+err.Error())
		_ = object.FinishMachineNodeDeployTask(taskId, true, object.MachineNodeDeployPhaseReady, "machine status update failed: "+err.Error())
		return
	}
	_ = object.FinishMachineNodeDeployTask(taskId, true, object.MachineNodeDeployPhaseReady, "")
}

func cleanupMachineNodeDeployPanic(taskId int64, owner, machineName, message string) {
	defer func() {
		if v := recover(); v != nil {
			logs.Error("panic while cleaning up node deployment task panic: %v", v)
		}
	}()
	if logErr := object.AddMachineNodeDeployLog(taskId, object.MachineNodeDeployLogLevelError, message); logErr != nil {
		logs.Warning("failed to log node deployment panic: %v", logErr)
	}
	if statusErr := object.UpdateMachineNodeDeployStatus(owner, machineName, object.MachineStatusFailed); statusErr != nil {
		logs.Warning("failed to update machine status after node deployment panic: %v", statusErr)
	}
	if err := object.FinishMachineNodeDeployTask(taskId, false, object.MachineNodeDeployPhaseFailed, message); err != nil {
		logs.Warning("failed to finish node deployment task after panic: %v", err)
	}
}

func toNodeDeployMachine(machine *object.Machine) (NodeDeployMachine, error) {
	deployMachine := NodeDeployMachine{
		Host:       machine.Ip,
		Port:       machine.Port,
		Username:   machine.Username,
		Password:   machine.Password,
		PrivateKey: machine.PrivateKey,
	}
	if strings.TrimSpace(deployMachine.Password) == "" && strings.TrimSpace(deployMachine.PrivateKey) == "" {
		credential, err := object.GetMachineNodeDeployCredential(machine.Owner, machine.Name)
		if err != nil {
			return deployMachine, fmt.Errorf("failed to load managed SSH credential for %s/%s: %w", machine.Owner, machine.Name, err)
		}
		if credential == nil || strings.TrimSpace(credential.PrivateKey) == "" {
			return deployMachine, fmt.Errorf("no SSH credential available for machine %s/%s", machine.Owner, machine.Name)
		}
		deployMachine.PrivateKey = credential.PrivateKey
	}
	return deployMachine, nil
}

func getMachineForNodeDeploy(owner, machineName string) (*object.Machine, error) {
	if machineName == "" {
		return nil, fmt.Errorf("machineName is required")
	}
	machine, err := object.GetMachine(fmt.Sprintf("%s/%s", owner, machineName))
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, fmt.Errorf("machine not found")
	}
	if machine.Ip == "" || machine.Username == "" {
		return nil, fmt.Errorf("machine ip and username are required")
	}
	if strings.TrimSpace(machine.Password) == "" && strings.TrimSpace(machine.PrivateKey) == "" {
		credential, err := object.GetMachineNodeDeployCredential(machine.Owner, machine.Name)
		if err != nil {
			return nil, err
		}
		if credential != nil && strings.TrimSpace(credential.PrivateKey) != "" {
			return machine, nil
		}
		return nil, fmt.Errorf("machine password or private key is required")
	}
	return machine, nil
}

func resolveMachineNodeApiserverURL(machine NodeDeployMachine, fallbackURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner, err := NewNodeDeploySSHRunner(NodeDeploySSHConfig{
		Host:       machine.Host,
		Port:       machine.Port,
		Username:   machine.Username,
		Password:   machine.Password,
		PrivateKey: machine.PrivateKey,
	})
	if err != nil {
		return "", err
	}
	defer runner.Close()
	return ResolveNodeDeployApiserverURL(ctx, runner, fallbackURL), nil
}

func defaultNodeDeployApiserverURL(config Config) string {
	host := config.AdvertiseAddress
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = config.ApiserverBind
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("https://%s", net.JoinHostPort(host, strconv.Itoa(config.ApiserverPort)))
}
