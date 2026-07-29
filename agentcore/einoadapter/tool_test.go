package einoadapter

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeEinoTool struct{}

func (fakeEinoTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "echo",
		Desc: "echo input",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"text": {Type: schema.String, Required: true},
		}),
	}, nil
}

func (fakeEinoTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	return arguments, nil
}

func TestNewToolAdaptsMetadataAndExecution(t *testing.T) {
	adapted, err := NewTool(context.Background(), fakeEinoTool{})
	if err != nil {
		t.Fatal(err)
	}
	definition := adapted.Definition()
	if definition.Name != "echo" || definition.Description != "echo input" || len(definition.Parameters) == 0 {
		t.Fatalf("definition = %+v", definition)
	}
	result, err := adapted.Execute(context.Background(), []byte(`{"text":"hi"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text() != `{"text":"hi"}` {
		t.Fatalf("result = %+v", result)
	}
}
