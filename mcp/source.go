package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

// ConnectRequest contains identity plus the durable, non-secret server config.
// Config may contain secret references but never resolved secret values.
type ConnectRequest struct {
	Execution capability.ResolveContext
	Name      string
	Version   string
	Config    map[string]any
}

// Session owns one materialized MCP connection and its adapted tools.
type Session interface {
	io.Closer
	Tools(context.Context) ([]agentcore.Tool, error)
}

// Connector resolves transport, authentication and MCP protocol details.
type Connector interface {
	Connect(context.Context, ConnectRequest) (Session, error)
}

type ConnectorFunc func(context.Context, ConnectRequest) (Session, error)

func (f ConnectorFunc) Connect(ctx context.Context, request ConnectRequest) (Session, error) {
	return f(ctx, request)
}

type Source struct{ Connector Connector }

func (s Source) Resolve(ctx context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Connector == nil {
		return capability.Resolved{}, errors.New("mcp: connector is required")
	}
	session, err := s.Connector.Connect(ctx, ConnectRequest{
		Execution: execution, Name: ref.Name, Version: ref.Version, Config: cloneConfig(ref.Config),
	})
	if err != nil {
		return capability.Resolved{}, fmt.Errorf("mcp: connect %s: %w", ref.Name, err)
	}
	if session == nil {
		return capability.Resolved{}, errors.New("mcp: connector returned nil session")
	}
	tools, err := session.Tools(ctx)
	if err != nil {
		return capability.Resolved{}, errors.Join(fmt.Errorf("mcp: list tools for %s: %w", ref.Name, err), session.Close())
	}
	return capability.Resolved{
		Tools: tools, Closers: []io.Closer{session},
		Snapshot: capability.Snapshot{Kind: capability.KindMCP, Name: ref.Name, Version: ref.Version, Metadata: map[string]string{"toolCount": strconv.Itoa(len(tools))}},
	}, nil
}

func Register(registry *capability.Registry, name string, connector Connector) error {
	if registry == nil {
		return errors.New("mcp: nil capability registry")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("mcp: name is required")
	}
	return registry.Register(capability.KindMCP, name, Source{Connector: connector})
}

func cloneConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}
	copy := make(map[string]any, len(config))
	for key, value := range config {
		copy[key] = value
	}
	return copy
}
