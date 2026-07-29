package agentcore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"aegis/agentcore"
)

type exampleModel struct {
	turn int
}

func (m *exampleModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.turn++
	if m.turn == 1 {
		return &exampleStream{chunks: []agentcore.ModelChunk{{
			ToolCallDeltas: []agentcore.ToolCallDelta{{
				Index: 0, ID: "call-1", Name: "double", ArgumentsDelta: `{"value":2}`,
			}},
			StopReason: agentcore.StopReasonToolUse,
		}}}, nil
	}
	return &exampleStream{chunks: []agentcore.ModelChunk{{
		TextDelta: "Result: 4", StopReason: agentcore.StopReasonStop,
	}}}, nil
}

type exampleStream struct {
	chunks []agentcore.ModelChunk
	index  int
}

func (s *exampleStream) Recv() (agentcore.ModelChunk, error) {
	if s.index == len(s.chunks) {
		return agentcore.ModelChunk{}, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (*exampleStream) Close() error { return nil }

func ExampleAgent_Prompt() {
	double := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "double"},
		ExecuteFunc: func(_ context.Context, arguments json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			return agentcore.TextToolResult("4"), nil
		},
	}
	agent, _ := agentcore.New(agentcore.Config{Model: &exampleModel{}, Tools: []agentcore.Tool{double}})

	var output strings.Builder
	result, _ := agent.Prompt(
		context.Background(),
		agentcore.State{},
		[]agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "Double 2")},
		func(_ context.Context, event agentcore.Event) error {
			if event.Type == agentcore.EventMessageUpdate && event.Delta != nil {
				output.WriteString(event.Delta.TextDelta)
			}
			return nil
		},
	)

	fmt.Printf("%s (%d turns)\n", output.String(), result.Turns)
	// Output: Result: 4 (2 turns)
}
