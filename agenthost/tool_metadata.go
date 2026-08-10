package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/z3r2ne/agentcore"
)

const invocationDescriptionParameter = "invocationDescription"

type invocationMetadataTool struct {
	base       agentcore.Tool
	definition agentcore.ToolDefinition
}

func decorateInvocationMetadata(tools []agentcore.Tool) ([]agentcore.Tool, error) {
	result := make([]agentcore.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			return nil, errors.New("agenthost: cannot decorate a nil tool")
		}
		definition := tool.Definition()
		parameters, err := addInvocationDescription(definition.Parameters)
		if err != nil {
			return nil, fmt.Errorf("agenthost: add invocation description to tool %q: %w", definition.Name, err)
		}
		definition.Parameters = parameters
		result = append(result, invocationMetadataTool{base: tool, definition: definition})
	}
	return result, nil
}

func addInvocationDescription(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	if _, exists := properties[invocationDescriptionParameter]; !exists {
		properties[invocationDescriptionParameter] = map[string]any{
			"type":        "string",
			"minLength":   1,
			"maxLength":   240,
			"description": "用一句简短的话说明本次调用的原因、用途和预期获得的结果；不要复述工具名或泄露秘密。",
		}
	}
	return json.Marshal(schema)
}

func stripInvocationDescription(raw json.RawMessage) (json.RawMessage, error) {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	delete(input, invocationDescriptionParameter)
	return json.Marshal(input)
}

func (t invocationMetadataTool) Definition() agentcore.ToolDefinition { return t.definition }

func (t invocationMetadataTool) Execute(ctx context.Context, raw json.RawMessage, sink agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
	stripped, err := stripInvocationDescription(raw)
	if err != nil {
		return agentcore.ToolResult{}, err
	}
	return t.base.Execute(ctx, stripped, sink)
}

func (t invocationMetadataTool) Validate(raw json.RawMessage) error {
	validator, ok := t.base.(agentcore.ToolValidator)
	if !ok {
		return nil
	}
	stripped, err := stripInvocationDescription(raw)
	if err != nil {
		return err
	}
	return validator.Validate(stripped)
}

func (t invocationMetadataTool) ExecutionMode() agentcore.ToolExecutionMode {
	if provider, ok := t.base.(agentcore.ToolExecutionModeProvider); ok {
		return provider.ExecutionMode()
	}
	return agentcore.ToolExecutionParallel
}

func (t invocationMetadataTool) ToolPolicy() agentcore.ToolPolicy {
	if provider, ok := t.base.(agentcore.ToolPolicyProvider); ok {
		return provider.ToolPolicy()
	}
	return agentcore.ToolPolicy{}
}
