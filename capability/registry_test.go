package capability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/z3r2ne/agentcore"
)

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func testTool(name string) agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{Name: name}, ExecuteFunc: func(context.Context, json.RawMessage, agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		return agentcore.TextToolResult(name), nil
	}}
}

func TestRegistryResolvesInOrderAndClosesInReverse(t *testing.T) {
	registry := NewRegistry()
	var closed []string
	for _, name := range []string{"first", "second"} {
		name := name
		err := registry.Register(KindTool, name, SourceFunc(func(_ context.Context, execution ResolveContext, ref Ref) (Resolved, error) {
			if execution.ExecutionID != "execution-1" || ref.Name != name {
				t.Fatalf("execution=%+v ref=%+v", execution, ref)
			}
			return Resolved{
				Tools:        []agentcore.Tool{testTool(name)},
				Instructions: []Instruction{{Source: name, Content: name + " instructions"}},
				Closers:      []io.Closer{closerFunc(func() error { closed = append(closed, name); return nil })},
			}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := registry.Resolve(context.Background(), ResolveContext{ExecutionID: "execution-1"}, []Ref{{Kind: KindTool, Name: "first"}, {Kind: KindTool, Name: "second"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{bundle.Tools[0].Definition().Name, bundle.Tools[1].Definition().Name}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("tools = %v", got)
	}
	if err := bundle.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closed, []string{"second", "first"}) {
		t.Fatalf("closed = %v", closed)
	}
}

func TestRegistryRejectsToolNameConflictAndCleansUp(t *testing.T) {
	registry := NewRegistry()
	closed := false
	if err := registry.Register(KindTool, "one", SourceFunc(func(context.Context, ResolveContext, Ref) (Resolved, error) {
		return Resolved{Tools: []agentcore.Tool{testTool("same")}, Closers: []io.Closer{closerFunc(func() error { closed = true; return nil })}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(KindTool, "two", SourceFunc(func(context.Context, ResolveContext, Ref) (Resolved, error) {
		return Resolved{Tools: []agentcore.Tool{testTool("same")}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Resolve(context.Background(), ResolveContext{}, []Ref{{Kind: KindTool, Name: "one"}, {Kind: KindTool, Name: "two"}})
	if err == nil || !closed {
		t.Fatalf("err=%v closed=%v", err, closed)
	}
}

func TestRegistryOptionalAndSourceErrors(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Resolve(context.Background(), ResolveContext{}, []Ref{{Kind: KindMCP, Name: "missing", Optional: true}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(KindMCP, "broken", SourceFunc(func(context.Context, ResolveContext, Ref) (Resolved, error) {
		return Resolved{}, errors.New("offline")
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(context.Background(), ResolveContext{}, []Ref{{Kind: KindMCP, Name: "broken"}}); err == nil {
		t.Fatal("expected source error")
	}
}

func TestRegistryDynamicKindSourceResolvesNewNames(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterKind(KindSkill, SourceFunc(func(_ context.Context, _ ResolveContext, ref Ref) (Resolved, error) {
		return Resolved{Snapshot: Snapshot{Kind: ref.Kind, Name: ref.Name}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	ref := Ref{Kind: KindSkill, Name: "created-after-host-start"}
	if !registry.Has(ref) {
		t.Fatal("dynamic source should advertise kind availability")
	}
	bundle, err := registry.Resolve(context.Background(), ResolveContext{}, []Ref{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Snapshots) != 1 || bundle.Snapshots[0].Name != ref.Name {
		t.Fatalf("bundle=%+v", bundle)
	}
}
