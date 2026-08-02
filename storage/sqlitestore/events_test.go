package sqlitestore

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis/storage"
	"github.com/z3r2ne/agentcore"
)

type streamingToolArgumentModel struct{ calls int }

func (m *streamingToolArgumentModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.calls++
	if m.calls == 1 {
		return &streamingToolArgumentStream{chunks: []agentcore.ModelChunk{
			{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "call-1", Name: "phone_action", ArgumentsDelta: `{`}}},
			{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ArgumentsDelta: `"action":`}}},
			{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ArgumentsDelta: `"open_app","ref":"@1"}`}}, StopReason: agentcore.StopReasonToolUse},
		}}, nil
	}
	return &streamingToolArgumentStream{chunks: []agentcore.ModelChunk{{TextDelta: "done", StopReason: agentcore.StopReasonStop}}}, nil
}

type streamingToolArgumentStream struct {
	chunks []agentcore.ModelChunk
	index  int
}

func (s *streamingToolArgumentStream) Recv() (agentcore.ModelChunk, error) {
	if s.index >= len(s.chunks) {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (*streamingToolArgumentStream) Close() error { return nil }

func TestEventsPersistAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := store.AppendExecutionEvent(context.Background(), storage.NewExecutionEvent{ExecutionID: "exec-1", Attempt: 2, CreatedAt: now, Event: agentcore.Event{Type: agentcore.EventAgentStart}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, err := store.ExecutionEvents(context.Background(), "exec-1", 0, 10)
	if err != nil || len(events) != 1 || events[0].Attempt != 2 || events[0].Event.Type != agentcore.EventAgentStart {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}

func TestConcurrentAppendHasContiguousSequence(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.AppendExecutionEvent(context.Background(), storage.NewExecutionEvent{ExecutionID: "exec-race", CreatedAt: time.Now(), Event: agentcore.Event{Type: agentcore.EventMessageUpdate}}); err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	wg.Wait()
	events, err := store.ExecutionEvents(context.Background(), "exec-race", 0, 100)
	if err != nil || len(events) != 16 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("sequence[%d]=%d", index, event.Sequence)
		}
	}
}

func TestStreamingToolArgumentsPersistAsDeltasUntilFinalJSON(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	model := &streamingToolArgumentModel{}
	tool := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "phone_action", Parameters: json.RawMessage(`{"type":"object"}`)},
		ExecuteFunc: func(_ context.Context, arguments json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			if string(arguments) != `{"action":"open_app","ref":"@1"}` {
				t.Fatalf("arguments=%s", arguments)
			}
			return agentcore.TextToolResult("opened"), nil
		},
	}
	agent, err := agentcore.New(agentcore.Config{Model: model, Tools: []agentcore.Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.Prompt(
		context.Background(), agentcore.State{},
		[]agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "open Board")},
		storage.EventSink(store, "exec-stream", 1, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ExecutionEvents(context.Background(), "exec-stream", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	var finalArguments json.RawMessage
	for _, record := range events {
		event := record.Event
		if event.Type == agentcore.EventMessageUpdate && event.Delta != nil {
			for _, delta := range event.Delta.ToolCallDeltas {
				deltas = append(deltas, delta.ArgumentsDelta)
			}
		}
		if event.Type == agentcore.EventMessageEnd && event.Message != nil {
			calls := event.Message.ToolCalls()
			if len(calls) == 1 && calls[0].Name == "phone_action" {
				finalArguments = calls[0].Arguments
			}
		}
	}
	const expected = `{"action":"open_app","ref":"@1"}`
	if got := strings.Join(deltas, ""); got != expected {
		t.Fatalf("deltas=%q", got)
	}
	if string(finalArguments) != expected {
		t.Fatalf("final arguments=%s", finalArguments)
	}
}
