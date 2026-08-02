package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/z3r2ne/agentcore"
)

func TestAllowListBlocksUnknownTool(t *testing.T) {
	hook := BeforeToolHook(AllowList{"read": {}})
	decision, err := hook(context.Background(), agentcore.ToolCallContext{Turn: 2, Call: agentcore.ToolCall{Name: "bash", Arguments: json.RawMessage(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Block || decision.Reason == "" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPolicyRunsAfterArgumentRewrite(t *testing.T) {
	authorized := ""
	previous := func(context.Context, agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
		return agentcore.ToolCallDecision{Arguments: json.RawMessage(`{"safe":true}`)}, nil
	}
	hook := ChainBeforeToolHook(previous, AuthorizerFunc(func(_ context.Context, request ToolRequest) (Decision, error) {
		authorized = string(request.Call.Arguments)
		return Decision{Allow: true}, nil
	}))
	decision, err := hook(context.Background(), agentcore.ToolCallContext{Call: agentcore.ToolCall{Name: "write", Arguments: json.RawMessage(`{"safe":false}`)}})
	if err != nil || authorized != `{"safe":true}` || string(decision.Arguments) != `{"safe":true}` {
		t.Fatalf("authorized=%s decision=%+v err=%v", authorized, decision, err)
	}
}
