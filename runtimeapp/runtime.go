package runtimeapp

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	phonecap "aegis/agentapp/agentcoreadapter"
	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"aegis/mcp"
	"aegis/policy"
	"aegis/skill"
	"aegis/storage"
	webcap "aegis/web"
	"github.com/z3r2ne/agentcore"
)

type Registration struct {
	Kind   capability.Kind
	Name   string
	Source capability.Source
}

type Config struct {
	Models     agenthost.ModelResolver
	Repository coordination.ExecutionRepository
	Events     storage.EventStore
	WorkerID   string

	AgentDefaults agentcore.Config
	Phone         *phonecap.Source
	Skills        map[string]skill.Loader
	MCPServers    map[string]mcp.Connector
	Web           *webcap.Source
	Additional    []Registration
	Authorizer    policy.Authorizer

	DefaultMaxAttempts int
	LeaseDuration      time.Duration
	HeartbeatInterval  time.Duration
}

type Runtime struct {
	Capabilities *capability.Registry
	Host         *agenthost.Host
	Executions   *coordination.ExecutionQueue
	Worker       *coordination.ExecutionWorker
}

func New(config Config) (*Runtime, error) {
	if config.Models == nil {
		return nil, errors.New("runtimeapp: model resolver is required")
	}
	if config.Repository == nil {
		return nil, errors.New("runtimeapp: Coordination execution repository is required")
	}
	if strings.TrimSpace(config.WorkerID) == "" {
		return nil, errors.New("runtimeapp: worker ID is required")
	}
	registry := capability.NewRegistry()
	if config.Phone != nil {
		if err := phonecap.RegisterDefault(registry, *config.Phone); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(config.Skills) {
		if err := skill.Register(registry, name, config.Skills[name]); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(config.MCPServers) {
		if err := mcp.Register(registry, name, config.MCPServers[name]); err != nil {
			return nil, err
		}
	}
	if config.Web != nil {
		if err := webcap.RegisterDefault(registry, *config.Web); err != nil {
			return nil, err
		}
	}
	for _, registration := range config.Additional {
		if err := registry.Register(registration.Kind, registration.Name, registration.Source); err != nil {
			return nil, err
		}
	}
	host, err := agenthost.New(config.Models, registry)
	if err != nil {
		return nil, err
	}
	host.AgentDefaults = config.AgentDefaults
	if config.Authorizer != nil {
		host.AgentDefaults.Hooks.BeforeToolCall = policy.ChainBeforeToolHook(host.AgentDefaults.Hooks.BeforeToolCall, config.Authorizer)
	}
	executions := &coordination.ExecutionQueue{Repository: config.Repository, DefaultMaxAttempts: config.DefaultMaxAttempts}
	worker := &coordination.ExecutionWorker{
		Repository: config.Repository, Executor: host, WorkerID: config.WorkerID,
		LeaseDuration: config.LeaseDuration, HeartbeatInterval: config.HeartbeatInterval,
	}
	if config.Events != nil {
		worker.Sinks = coordination.ExecutionEventSinkFactoryFunc(func(_ context.Context, execution coordination.Execution) (agentcore.EventSink, error) {
			return storage.EventSink(config.Events, execution.ID, execution.Attempt, nil), nil
		})
	}
	return &Runtime{Capabilities: registry, Host: host, Executions: executions, Worker: worker}, nil
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
