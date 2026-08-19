package control

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	phonecap "aegis/agentapp/agentcoreadapter"
	"aegis/agenthost"
	"aegis/capability"
	"aegis/policy"
	openai "aegis/provider/openai"
	skillcap "aegis/skill"
	webcap "aegis/web"
	workspacecap "aegis/workspace"
	"github.com/z3r2ne/agentcore"
)

// SkillLoader adapts the control registry to the provider-neutral Skill port.
// It returns detached values so capability resolution cannot mutate Store state.
type SkillLoader struct{ Store *Store }

func (l SkillLoader) LoadSkill(ctx context.Context, name, version string) (skillcap.Document, error) {
	if err := contextError(ctx); err != nil {
		return skillcap.Document{}, err
	}
	if l.Store == nil {
		return skillcap.Document{}, errors.New("control: nil skill store")
	}
	for _, item := range l.Store.Skills() {
		if item.ID != name {
			continue
		}
		if version != "" && item.Version != version {
			return skillcap.Document{}, fmt.Errorf("control: skill %s version %s not found", name, version)
		}
		return skillcap.Document{
			Name: item.ID, Version: item.Version, Content: item.Content,
			Metadata: map[string]string{
				"displayName": item.DisplayName, "source": item.Source,
				"builtin": fmt.Sprintf("%t", item.Builtin), "updatedAt": item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
			},
		}, nil
	}
	return skillcap.Document{}, fmt.Errorf("control: skill %s not found", name)
}

// WebSearcher keeps the configured endpoint and credential inside control;
// neither is exposed through capability refs or model-visible tool results.
type WebSearcher struct{ Store *Store }

func (s WebSearcher) Search(ctx context.Context, request webcap.SearchRequest) (webcap.SearchResult, error) {
	if s.Store == nil {
		return webcap.SearchResult{}, errors.New("control: nil web search store")
	}
	result, err := executeWebSearch(ctx, s.Store.Config().WebSearch, WebSearchInput{
		Query: request.Query, Topic: request.Topic, SearchDepth: request.Depth,
		IncludeAnswer: request.IncludeAnswer, MaxResults: request.MaxResults,
	})
	if err != nil {
		return webcap.SearchResult{}, err
	}
	items := make([]webcap.SearchItem, len(result.Results))
	for index, item := range result.Results {
		items[index] = webcap.SearchItem{Title: item.Title, URL: item.URL, Content: item.Content, Score: item.Score}
	}
	return webcap.SearchResult{Provider: result.Engine, Query: result.Query, Answer: result.Answer, Results: items}, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

var _ skillcap.Loader = SkillLoader{}
var _ webcap.Searcher = WebSearcher{}

// NativeModelResolver reads the latest effective Agent configuration on every
// execution, allowing settings changes without rebuilding the Host.
type NativeModelResolver struct{ Store *Store }

func (r NativeModelResolver) ResolveModel(ctx context.Context, ref agenthost.ModelRef) (agentcore.Model, error) {
	if r.Store == nil {
		return nil, errors.New("control: nil native model store")
	}
	execution, ok := agenthost.ExecutionFromContext(ctx)
	if !ok || execution.AgentID == "" {
		return nil, errors.New("control: native model execution identity is missing")
	}
	agent, err := r.Store.GetAgent(execution.AgentID)
	if err != nil {
		return nil, err
	}
	cfg := r.Store.effectiveAgentConfig(agent)
	if cfg.Provider != ref.Provider || cfg.Model != ref.Model {
		return nil, errors.New("control: durable model ref no longer matches effective Agent configuration")
	}
	if !strings.EqualFold(cfg.Provider, "openai") && cfg.BaseURL == "" {
		return nil, fmt.Errorf("control: provider %s requires an OpenAI-compatible base URL for native execution", cfg.Provider)
	}
	apiKey := ""
	switch cfg.AuthMode {
	case "api_key":
		apiKey = cfg.APIKey
	case "environment":
		if key := providerEnv(cfg.Provider); key != "" {
			apiKey = os.Getenv(key)
		}
	default:
		return nil, fmt.Errorf("control: unsupported native authentication mode %s", cfg.AuthMode)
	}
	if apiKey == "" {
		return nil, errors.New("control: native provider credential is unavailable")
	}
	return openai.NewModel(openai.Config{BaseURL: cfg.BaseURL, APIKey: apiKey}, cfg.Model)
}

type NativeToolAuthorizer struct{ Store *Store }

func (a NativeToolAuthorizer) AuthorizeTool(ctx context.Context, request policy.ToolRequest) (policy.Decision, error) {
	if a.Store == nil {
		return policy.Decision{}, errors.New("control: nil native policy store")
	}
	agent, err := a.Store.GetAgent(request.Execution.AgentID)
	if err != nil {
		return policy.Decision{}, err
	}
	switch request.Call.Name {
	case "web_search":
		return policy.Decision{Allow: agent.Permissions.AllowNetwork, Reason: "Agent network permission is disabled"}, nil
	case "phone_view", "phone_action", "phone_back", "phone_home":
		return policy.Decision{Allow: true}, nil
	default:
		if agenthost.ToolAuthorizedFromContext(ctx, request.Call.Name) {
			return policy.Decision{Allow: true}, nil
		}
		return policy.Decision{Allow: false, Reason: "tool is not present in the Coordination-approved capability bundle"}, nil
	}
}

// NewNativeAgentHost assembles the current control Skill registry and Web
// search configuration around the provider-neutral AgentHost.
type NativeHostOptions struct {
	Phone   *phonecap.Source
	Sources []NativeCapabilitySource
}

// NativeCapabilitySource is an embedding extension point for MCP servers,
// custom tools, software adapters, or any future capability family.
type NativeCapabilitySource struct {
	Kind   capability.Kind
	Name   string
	Source capability.Source
}

func NewNativeAgentHost(store *Store, options ...NativeHostOptions) (*agenthost.Host, error) {
	if store == nil {
		return nil, errors.New("control: native AgentHost requires a Store")
	}
	registry := capability.NewRegistry()
	loader := SkillLoader{Store: store}
	if err := registry.RegisterKind(capability.KindSkill, skillcap.Source{Loader: loader}); err != nil {
		return nil, err
	}
	if err := webcap.RegisterDefault(registry, webcap.Source{Searcher: WebSearcher{Store: store}}); err != nil {
		return nil, err
	}
	if err := workspacecap.RegisterDefault(registry, workspacecap.DockerSource{}); err != nil {
		return nil, err
	}
	if err := registry.Register(capability.KindTool, "delivery", NativeDeliverySource{Store: store}); err != nil {
		return nil, err
	}
	if err := registry.Register(capability.KindTool, "task-evidence", TaskEvidenceSource{Store: store}); err != nil {
		return nil, err
	}
	if len(options) > 0 && options[0].Phone != nil {
		if err := phonecap.RegisterDefault(registry, *options[0].Phone); err != nil {
			return nil, err
		}
	}
	if len(options) > 0 {
		for _, item := range options[0].Sources {
			if err := registry.Register(item.Kind, item.Name, item.Source); err != nil {
				return nil, err
			}
		}
	}
	host, err := agenthost.New(NativeModelResolver{Store: store}, registry)
	if err != nil {
		return nil, err
	}
	host.AgentDefaults = agentcore.Config{
		// Child work Executions are reduced to their snapshotted work+summary
		// budget by executionBudgetController. A root coordinator is governed by
		// the Task wall clock instead, so its emergency AgentCore guard must not
		// masquerade as the 100-turn child budget.
		MaxTurns: 1000, MaxToolConcurrency: 4,
		ModelRetry:    agentcore.RetryPolicy{MaxAttempts: 3, InitialDelay: 250 * time.Millisecond, MaxDelay: 2 * time.Second},
		ToolPolicy:    agentcore.ToolPolicy{Timeout: 2 * time.Minute, MaxAttempts: 2, RetryDelay: 100 * time.Millisecond},
		ContextPolicy: agentcore.ContextPolicy{MaxToolResultBytes: 64 * 1024},
	}
	host.Configurer = agenthost.AgentConfigurerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, config *agentcore.Config) error {
		if configurer, ok := spec.Runtime.(agenthost.AgentConfigurer); ok {
			return configurer.ConfigureAgent(ctx, spec, config)
		}
		return nil
	})
	host.AgentDefaults.Hooks.BeforeToolCall = policy.BeforeToolHook(NativeToolAuthorizer{Store: store})
	return host, nil
}

var _ agenthost.ModelResolver = NativeModelResolver{}
var _ policy.Authorizer = NativeToolAuthorizer{}
