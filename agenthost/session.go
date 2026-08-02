package agenthost

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"aegis/capability"
	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

// Session owns a stateful AgentCore session and keeps execution-scoped
// capability resources alive until Close.
type Session struct {
	Spec         ExecutionSpec
	Capabilities []capability.Snapshot

	core   *agentcore.Session
	bundle capability.Bundle
	mu     sync.Mutex
	closed bool
}

func (h *Host) NewSession(ctx context.Context, spec ExecutionSpec, snapshot *agentcore.SessionSnapshot, options agentcore.SessionOptions) (*Session, error) {
	ctx = withExecution(ctx, spec)
	observability.Default().Info(ctx, "agenthost.session.open", slog.String("session_id", spec.SessionID), slog.Bool("restored", snapshot != nil), slog.Int("capability_count", len(spec.Capabilities)))
	materialized, err := h.materialize(ctx, spec)
	if err != nil {
		return nil, err
	}
	if options.SessionID == "" {
		options.SessionID = spec.SessionID
	}
	var core *agentcore.Session
	if snapshot == nil {
		core, err = agentcore.NewSession(materialized.agent, agentcore.State{}, options)
	} else {
		core, err = agentcore.NewSessionFromSnapshot(materialized.agent, *snapshot, options)
	}
	if err != nil {
		_ = materialized.bundle.Close()
		return nil, err
	}
	return &Session{Spec: spec, Capabilities: append([]capability.Snapshot(nil), materialized.bundle.Snapshots...), core: core, bundle: materialized.bundle}, nil
}

func (s *Session) Prompt(ctx context.Context, prompts []agentcore.Message, sink agentcore.EventSink) (agentcore.Result, error) {
	if err := s.available(); err != nil {
		return agentcore.Result{}, err
	}
	return s.core.Prompt(s.executionContext(ctx), prompts, sink)
}

func (s *Session) Stream(ctx context.Context, prompts []agentcore.Message) (*agentcore.EventStream, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	return s.core.Stream(s.executionContext(ctx), prompts), nil
}

// executionContext rebuilds the request-scoped values needed by hooks for
// every Session run. The context created while materializing the Session must
// not be retained because its cancellation and deadline belong to that one
// call, but the materialized tool authorization remains valid for the
// lifetime of the Session's capability bundle.
func (s *Session) executionContext(ctx context.Context) context.Context {
	ctx = withExecution(ctx, s.Spec)
	return withAuthorizedTools(ctx, s.bundle.Tools)
}

func (s *Session) Steer(messages ...agentcore.Message) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.core.Steer(messages...)
}

func (s *Session) FollowUp(messages ...agentcore.Message) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.core.FollowUp(messages...)
}

func (s *Session) Abort() error {
	if err := s.available(); err != nil {
		return err
	}
	return s.core.Abort()
}

func (s *Session) Status() agentcore.SessionStatus {
	if s == nil || s.core == nil {
		return agentcore.SessionStatus{}
	}
	return s.core.Status()
}

func (s *Session) Snapshot() (agentcore.SessionSnapshot, error) {
	if err := s.available(); err != nil {
		return agentcore.SessionSnapshot{}, err
	}
	return s.core.Snapshot()
}

func (s *Session) WaitForIdle(ctx context.Context) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.core.WaitForIdle(ctx)
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	if s.core != nil && s.core.Status().Running {
		_ = s.core.Abort()
	}
	return s.bundle.Close()
}

func (s *Session) available() error {
	if s == nil || s.core == nil {
		return errors.New("agenthost: nil session")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("agenthost: session is closed")
	}
	return nil
}
