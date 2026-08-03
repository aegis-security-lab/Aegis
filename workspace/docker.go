// Package workspace exposes project filesystem tools whose execution is
// confined to a Task-owned Docker container. The Agent loop may live in the
// control plane, but it never executes project commands or opens project files
// on the host.
package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

const (
	defaultOutputLimit = 64 * 1024
	defaultTimeout     = 60 * time.Second
	maximumTimeout     = time.Hour
	maximumEditBytes   = 4 * 1024 * 1024
)

// DockerSource materializes the standard coding tools against the container
// name passed in control.runtimeId. No tool has a host-filesystem fallback.
type DockerSource struct {
	Binary         string
	MaxOutputBytes int
}

func RegisterDefault(registry *capability.Registry, source DockerSource) error {
	if registry == nil {
		return errors.New("workspace: nil capability registry")
	}
	return registry.Register(capability.KindTool, "workspace", source)
}

func (s DockerSource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	container, _ := execution.Values["control.runtimeId"].(string)
	container = strings.TrimSpace(container)
	root := strings.TrimSpace(filepath.ToSlash(execution.Workspace))
	if container == "" {
		return capability.Resolved{}, errors.New("workspace: task container runtime ID is required")
	}
	if root == "" || !path.IsAbs(root) {
		return capability.Resolved{}, errors.New("workspace: absolute container workspace is required")
	}
	runner := dockerRunner{binary: strings.TrimSpace(s.Binary), container: container, root: path.Clean(root), maximum: s.MaxOutputBytes}
	if runner.binary == "" {
		runner.binary = "docker"
	}
	if runner.maximum <= 0 {
		runner.maximum = defaultOutputLimit
	}
	allowShell, err := capabilityBool(ref.Config, "allowShell", true)
	if err != nil {
		return capability.Resolved{}, err
	}
	allowWrite, err := capabilityBool(ref.Config, "allowWrite", true)
	if err != nil {
		return capability.Resolved{}, err
	}
	tools := make([]agentcore.Tool, 0, 7)
	if allowShell {
		tools = append(tools, runner.bashTool())
	}
	tools = append(tools, runner.readTool())
	if allowWrite {
		tools = append(tools, runner.writeTool(), runner.editTool())
	}
	tools = append(tools, runner.grepTool(), runner.findTool(), runner.listTool())
	return capability.Resolved{
		Tools:        tools,
		Instructions: []capability.Instruction{{Source: "docker-workspace", Content: fmt.Sprintf("All project work is isolated in the Task Docker container. Use only the workspace tools supplied to this execution. The shared Task workspace is %s. Never refer to, request, or invent a host path.", runner.root)}},
		Snapshot: capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: ref.Version, Metadata: map[string]string{
			"runtime": "docker", "workspace": runner.root,
			"allowShell": strconv.FormatBool(allowShell), "allowWrite": strconv.FormatBool(allowWrite),
		}},
	}, nil
}

func capabilityBool(config map[string]any, key string, fallback bool) (bool, error) {
	value, exists := config[key]
	if !exists {
		return fallback, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("workspace: %s must be a boolean", key)
	}
	return result, nil
}

type dockerRunner struct {
	binary, container, root string
	maximum                 int
}

type commandResult struct {
	ExitCode  int    `json:"exitCode"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (r dockerRunner) run(ctx context.Context, timeout time.Duration, stdin []byte, arguments ...string) (commandResult, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > maximumTimeout {
		timeout = maximumTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := &limitedBuffer{maximum: r.maximum}
	command := exec.CommandContext(ctx, r.binary, arguments...)
	command.Stdout, command.Stderr = output, output
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	err := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return commandResult{}, ctxErr
	}
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return commandResult{}, err
		}
		exitCode = exitError.ExitCode()
	}
	return commandResult{ExitCode: exitCode, Output: output.String(), Truncated: output.truncated}, nil
}

func (r dockerRunner) shell(ctx context.Context, timeout time.Duration, stdin []byte, script string, arguments ...string) (commandResult, error) {
	args := []string{"exec", "-i", "--workdir", r.root, r.container, "sh", "-c", script, "aegis-workspace"}
	args = append(args, arguments...)
	return r.run(ctx, timeout, stdin, args...)
}

func toolResult(result commandResult) (agentcore.ToolResult, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return agentcore.ToolResult{}, err
	}
	value := agentcore.TextToolResult(string(encoded))
	value.IsError = result.ExitCode != 0
	value.Details = result
	return value, nil
}

func (r dockerRunner) bashTool() agentcore.Tool {
	type input struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
	}
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "bash", Description: "Run a shell command inside the Task Docker container with /workspace as its working directory.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","minLength":1},"timeoutSeconds":{"type":"integer","minimum":1,"maximum":3600}},"required":["command"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		if strings.TrimSpace(request.Command) == "" {
			return agentcore.ToolResult{}, errors.New("workspace: command is required")
		}
		result, err := r.run(ctx, time.Duration(request.TimeoutSeconds)*time.Second, nil, "exec", "-i", "--workdir", r.root, r.container, "sh", "-lc", request.Command)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) readTool() agentcore.Tool {
	type input struct {
		Path   string `json:"path"`
		Offset int    `json:"offset,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "read", Description: "Read a UTF-8 text file from the shared Task workspace by line range.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"offset":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":2000}},"required":["path"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionParallel, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if request.Offset <= 0 {
			request.Offset = 1
		}
		if request.Limit <= 0 {
			request.Limit = 500
		}
		end := request.Offset + request.Limit - 1
		result, err := r.shell(ctx, defaultTimeout, nil, `sed -n "$2,$3p" -- "$1"`, target, strconv.Itoa(request.Offset), strconv.Itoa(end))
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) writeTool() agentcore.Tool {
	type input struct{ Path, Content string }
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "write", Description: "Create or replace a file in the shared Task workspace.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		result, err := r.shell(ctx, defaultTimeout, []byte(request.Content), `umask 077; mkdir -p -- "$(dirname -- "$1")" && cat > "$1"`, target)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if result.ExitCode == 0 {
			result.Output = fmt.Sprintf("wrote %d bytes to %s", len(request.Content), target)
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) editTool() agentcore.Tool {
	type input struct {
		Path, OldText, NewText string
		ReplaceAll             bool `json:"replaceAll,omitempty"`
	}
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "edit", Description: "Replace an exact text fragment in a workspace file. By default the old text must occur exactly once.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"oldText":{"type":"string","minLength":1},"newText":{"type":"string"},"replaceAll":{"type":"boolean"}},"required":["path","oldText","newText"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		reader := r
		reader.maximum = maximumEditBytes + 1
		current, err := reader.shell(ctx, defaultTimeout, nil, `cat -- "$1"`, target)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if current.ExitCode != 0 {
			return toolResult(current)
		}
		if current.Truncated || len(current.Output) > maximumEditBytes {
			return agentcore.ToolResult{}, errors.New("workspace: file is too large to edit")
		}
		count := strings.Count(current.Output, request.OldText)
		if count == 0 || (!request.ReplaceAll && count != 1) {
			return agentcore.ToolResult{}, fmt.Errorf("workspace: oldText occurrences = %d; expected %s", count, map[bool]string{true: "at least one", false: "exactly one"}[request.ReplaceAll])
		}
		updated := strings.Replace(current.Output, request.OldText, request.NewText, 1)
		if request.ReplaceAll {
			updated = strings.ReplaceAll(current.Output, request.OldText, request.NewText)
		}
		result, err := r.shell(ctx, defaultTimeout, []byte(updated), `umask 077; cat > "$1"`, target)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if result.ExitCode == 0 {
			result.Output = fmt.Sprintf("replaced %d occurrence(s) in %s", map[bool]int{true: count, false: 1}[request.ReplaceAll], target)
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) grepTool() agentcore.Tool {
	type input struct{ Pattern, Path string }
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "grep", Description: "Recursively search workspace text files with an extended regular expression.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","minLength":1},"path":{"type":"string"}},"required":["pattern"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionParallel, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		result, err := r.shell(ctx, defaultTimeout, nil, `grep -RInE -- "$2" "$1"`, target, request.Pattern)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		// grep uses status 1 for a valid search with no matches.
		if result.ExitCode == 1 {
			result.ExitCode = 0
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) findTool() agentcore.Tool {
	type input struct{ Pattern, Path string }
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "find", Description: "Find files in the workspace by shell-style basename pattern.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","minLength":1},"path":{"type":"string"}},"required":["pattern"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionParallel, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		result, err := r.shell(ctx, defaultTimeout, nil, `find "$1" -type f -name "$2" -print | sort`, target, request.Pattern)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) listTool() agentcore.Tool {
	type input struct {
		Path  string `json:"path,omitempty"`
		Depth int    `json:"depth,omitempty"`
	}
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "ls", Description: "List entries in a workspace directory to a bounded depth.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"depth":{"type":"integer","minimum":1,"maximum":6}},"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionParallel, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var request input
		if err := json.Unmarshal(raw, &request); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := r.target(request.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if request.Depth <= 0 {
			request.Depth = 2
		}
		result, err := r.shell(ctx, defaultTimeout, nil, `find "$1" -mindepth 1 -maxdepth "$2" -print | sort`, target, strconv.Itoa(request.Depth))
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		return toolResult(result)
	}}
}

func (r dockerRunner) target(value string) (string, error) {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" || value == "." {
		return r.root, nil
	}
	var target string
	if path.IsAbs(value) {
		target = path.Clean(value)
	} else {
		target = path.Join(r.root, value)
	}
	if target != r.root && !strings.HasPrefix(target, r.root+"/") {
		return "", errors.New("workspace: path escapes the Task workspace")
	}
	return target, nil
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	maximum   int
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(value)
	remaining := b.maximum - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buffer.Write(value)
	}
	if written > remaining {
		b.truncated = true
	}
	return written, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

var _ capability.Source = DockerSource{}
