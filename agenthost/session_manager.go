package agenthost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/z3r2ne/agentcore"
)

type DeliveryMode string

const (
	DeliverySteer    DeliveryMode = "steer"
	DeliveryFollowUp DeliveryMode = "follow_up"
	DeliveryNextRun  DeliveryMode = "next_run"
)

type ManagedRunResult struct {
	SessionID string
	Spec      ExecutionSpec
	Result    agentcore.Result
	Err       error
}

// SessionManager owns live native sessions and routes asynchronous messages.
// When a session is running, messages become steering/follow-up input; when it
// is idle, delivery starts a continuation run instead of requiring Pi RPC.
type SessionManager struct {
	Host     *Host
	Store    agentcore.SessionStore
	Sink     func(ExecutionSpec) agentcore.EventSink
	OnResult func(ManagedRunResult)

	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	sessions map[string]*Session
	wg       sync.WaitGroup
	closed   bool
}

func NewSessionManager(parent context.Context, host *Host, store agentcore.SessionStore) (*SessionManager, error) {
	if host == nil {
		return nil, errors.New("agenthost: session manager requires a Host")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &SessionManager{Host: host, Store: store, ctx: ctx, cancel: cancel, sessions: make(map[string]*Session)}, nil
}

func (m *SessionManager) Open(ctx context.Context, spec ExecutionSpec, snapshot *agentcore.SessionSnapshot) (*Session, error) {
	if m == nil {
		return nil, errors.New("agenthost: nil session manager")
	}
	sessionID := strings.TrimSpace(spec.SessionID)
	if sessionID == "" {
		return nil, errors.New("agenthost: managed session requires spec.SessionID")
	}
	m.mu.RLock()
	existing, closed := m.sessions[sessionID], m.closed
	m.mu.RUnlock()
	if closed {
		return nil, errors.New("agenthost: session manager is closed")
	}
	if existing != nil {
		return existing, nil
	}
	options := agentcore.SessionOptions{Store: m.Store, SessionID: sessionID, SteeringMode: agentcore.DeliveryAll, FollowUpMode: agentcore.DeliveryAll}
	session, err := m.Host.NewSession(ctx, spec, snapshot, options)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = session.Close()
		return nil, errors.New("agenthost: session manager is closed")
	}
	if existing = m.sessions[sessionID]; existing != nil {
		m.mu.Unlock()
		_ = session.Close()
		return existing, nil
	}
	m.sessions[sessionID] = session
	m.mu.Unlock()
	return session, nil
}

func (m *SessionManager) Session(id string) (*Session, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	session, ok := m.sessions[strings.TrimSpace(id)]
	m.mu.RUnlock()
	return session, ok
}

// Start starts one asynchronous turn. The manager drains and forwards events,
// checkpoints through AgentCore Session, and reports the terminal result.
func (m *SessionManager) Start(sessionID string, messages ...agentcore.Message) error {
	session, ok := m.Session(sessionID)
	if !ok {
		return fmt.Errorf("agenthost: managed session %q not found", sessionID)
	}
	stream, err := session.Stream(m.ctx, messages)
	if err != nil {
		return err
	}
	var sink agentcore.EventSink
	if m.Sink != nil {
		sink = m.Sink(session.Spec)
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			event, ok := stream.Next()
			if !ok {
				break
			}
			if sink != nil {
				if sinkErr := sink(m.ctx, event); sinkErr != nil {
					_ = session.Abort()
				}
			}
		}
		result, runErr := stream.Result()
		if m.OnResult != nil {
			m.OnResult(ManagedRunResult{SessionID: sessionID, Spec: session.Spec, Result: result, Err: runErr})
		}
	}()
	return nil
}

func (m *SessionManager) Deliver(sessionID string, mode DeliveryMode, message agentcore.Message) error {
	session, ok := m.Session(sessionID)
	if !ok {
		return fmt.Errorf("agenthost: managed session %q not found", sessionID)
	}
	status := session.Status()
	if !status.Running {
		return m.Start(sessionID, message)
	}
	switch mode {
	case DeliverySteer:
		return session.Steer(message)
	case DeliveryFollowUp, DeliveryNextRun:
		return session.FollowUp(message)
	default:
		return fmt.Errorf("agenthost: unsupported delivery mode %q", mode)
	}
}

func (m *SessionManager) CloseSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	session := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if session == nil {
		return nil
	}
	if session.Status().Running {
		_ = session.Abort()
		_ = session.WaitForIdle(ctx)
	}
	return session.Close()
}

func (m *SessionManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.cancel()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()
	for _, session := range sessions {
		if session.Status().Running {
			_ = session.Abort()
		}
	}
	m.wg.Wait()
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close())
	}
	return result
}
