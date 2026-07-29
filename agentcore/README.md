# agentcore

`agentcore` is a provider-neutral Go implementation of Pi's core agent-loop
semantics. It is designed to be imported as a library rather than launched as
a CLI or RPC subprocess.

It includes:

- streaming assistant, thinking, and tool-call deltas;
- repeated model → tool → model turns;
- parallel or sequential tool execution;
- deterministic tool-result ordering;
- `context.Context` cancellation;
- Pi-style lifecycle events;
- argument validation and truncated-tool-call protection;
- lifecycle hooks and context transformation;
- synchronous callback and asynchronous iterator APIs;
- an Eino model/tool adapter.

It intentionally excludes sessions, persistence, RPC, TUI, authentication,
provider catalogs, and platform-specific coding tools.

## Core API

```go
tool := agentcore.FuncTool{
    ToolDefinition: agentcore.ToolDefinition{
        Name:        "weather",
        Description: "Get the weather for a city",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"]
        }`),
    },
    ExecuteFunc: func(
        ctx context.Context,
        arguments json.RawMessage,
        update agentcore.ToolUpdateSink,
    ) (agentcore.ToolResult, error) {
        return agentcore.TextToolResult("Sunny, 24 C"), nil
    },
}

agent, err := agentcore.New(agentcore.Config{
    Model:        modelAdapter,
    SystemPrompt: "You are a concise assistant.",
    Tools:        []agentcore.Tool{tool},
})
if err != nil {
    log.Fatal(err)
}

stream := agent.Stream(
    context.Background(),
    agentcore.State{},
    []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, "Weather in Shanghai?")},
)

for {
    event, ok := stream.Next()
    if !ok {
        break
    }
    if event.Type == agentcore.EventMessageUpdate && event.Delta.TextDelta != "" {
        fmt.Print(event.Delta.TextDelta)
    }
}

result, err := stream.Result()
```

For callers that already own an event bus, `Agent.Prompt` invokes an
`EventSink` synchronously instead of allocating an iterator.

## Eino adapter

Construct any Eino `model.ToolCallingChatModel`, then wrap it:

```go
chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
    Model:  "gpt-4.1",
    APIKey: os.Getenv("OPENAI_API_KEY"),
})
if err != nil {
    log.Fatal(err)
}

coreModel := einoadapter.Model{ChatModel: chatModel}
agent, err := agentcore.New(agentcore.Config{Model: coreModel})
```

Existing Eino tools can be converted once during startup:

```go
coreTool, err := einoadapter.NewTool(ctx, einoTool)
```

The Eino adapter preserves the merged raw provider message in memory across
tool turns. Provider-specific reasoning signatures and metadata therefore
survive the next model request without becoming part of the public core API.

## Event order

A run with one tool call emits:

```text
agent_start
turn_start
message_start / message_end        user prompt
message_start / message_update... / message_end
tool_execution_start / update... / end
message_start / message_end        tool result
turn_end
turn_start
message_start / message_update... / message_end
turn_end
agent_end
```

Parallel tools emit completion events in completion order, while tool-result
messages are appended to state in the model's original call order.
