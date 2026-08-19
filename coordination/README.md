# coordination

`coordination` 是一个不依赖 Aegis、GORM、聊天软件或 AgentCore 的 Go 协同与执行控制平面。它是“谁在何时做什么、能使用哪些能力、结果送到哪里”的唯一权威；AgentHost、Board、Relay、Phone、Web 和 MCP 都是被它授权或驱动的执行端。

核心边界只有三步：

1. 应用提交幂等 `Event`；
2. 当前 Task 绑定的 `Mode` 根据只读 `Snapshot` 返回 `PlannedEffect`；
3. Engine 原子提交决策，Effect Worker 再写入 Coordination Execution，或调用应用提供的 Board、Relay、Session、timer adapter。

协作决策接口为：

```go
type Mode interface {
    Name() string
    Version() string
    Decide(context.Context, Event, Binding, Snapshot) ([]PlannedEffect, error)
}
```

模式不得直接发消息、写 Board 或启动 Agent。它只描述 Effects；这样模式可单测、可版本固定、可组合，也不会在进程崩溃时出现“数据库没记住但消息已经发出”的双写窗口。

`MemoryRepository` 用于嵌入与测试，`sqlitestore` 提供 WAL、原子 claim、租约 fencing、幂等 outbox、延迟 effect 和重启恢复。默认失败八次后进入 `failed`，避免永久错误饿死队列。

Aegis 适配位于 `apps/board/control/coordination_bridge.go`；它不是该库的依赖。AgentCore 工具适配位于 `agentcoreadapter`。

## 能力与插件控制

Agent 只能获得 `CapabilityPlanner` 最终批准的能力引用。能力分为 `tool`、`skill`、`mcp`、`phone`、`web`，也允许宿主注册自定义 kind。每次执行都保存实际 materialize 后的 capability snapshot，任务证据导出会包含它。

选择语义：

- `inherit`：只使用 Agent 默认能力；
- `merge`：在默认能力上增加或收窄请求能力；
- `replace`：不用默认能力，只使用请求能力；
- Coordination 自身所需的控制工具始终作为 system-required 能力加入。

安全默认值不是 allow-all。没有 `allowed` 时，运行期请求只能覆盖 Agent 已有默认能力的配置，例如把 Phone 限制到 Board；新增 MCP、Skill 或自定义软件必须在 Task binding 中显式放行。显式请求被拒时整次计划失败，不会静默少一个工具继续工作。

Task 根的 binding 固定使用 `board_autonomy`，同时保存能力策略：

```json
{
  "mode": "board_autonomy",
  "version": "1",
  "config": {
    "capabilityPolicy": {
      "defaultSelection": "merge",
      "allowed": [
        {"kind": "tool", "name": "coordination"},
        {"kind": "skill", "name": "*"},
        {"kind": "phone", "name": "default"},
        {"kind": "mcp", "name": "github"}
      ],
      "denied": [{"kind": "web", "name": "*"}],
      "maxCapabilities": 12
    }
  }
}
```

一旦配置 `allowed`，它也会过滤 Agent 默认能力，因此必须包含 `phone/default`。策略中的 `required` 可强制每个执行携带指定能力。Capability config 只能保存非敏感参数或 secret reference，出现 `apiKey`、`password`、`authorization`、裸 `token`/`secret` 会被拒绝。

Board 和 Relay 已通过 `phone/default` 转成通用 Phone 工具。一次执行可以只开放 Board：

```json
{
  "capabilitySelection": "replace",
  "capabilities": [
    {
      "kind": "phone",
      "name": "default",
      "config": {"installedApps": ["aegis.board"]}
    }
  ]
}
```

Phone Source 会再次检查所选 App 和 scope 是否属于服务端 allowlist。MCP 或其他插件由宿主注册为 `capability.Source`；Coordination 只保存引用、决定是否授权，不持有连接和密钥。

## 主动调用与统一执行

所有入口最终都提交 Coordination Event：

- 父 Agent 调用 Phone Board 快捷指令 `phone_board_delegate`；
- 操作者调用 `POST /api/coordination/invoke`；
- Board Issue 被创建、指派或手动 dispatch；
- Relay 消息到达；
- durable wakeup 到期。

运行时只注册 `board_autonomy`：子任务表现为 Board Issue，父 Agent 不因委派而暂停；每分钟 delayed effect 汇总子项状态、最新进展和当前活动。Agent 没有可继续推进的动作时调用 Phone Board 快捷指令 `phone_board_sleep`，心跳、Board 评论和带发送者身份的 Relay/Phone 消息都可以提前唤醒或直接 steer 当前 loop。

Mode 产生 outbox effect 后，Execution Worker、Board、Relay 或 Agent Session adapter 才执行副作用。Agent 执行由 `ExecutionQueue` 持久化，`ExecutionWorker` 使用带 fencing token 的租约领取并调用 `agenthost.Runner`。Aegis 的公开 `IssueDispatcher` 只指向 Coordination gateway，不存在第二套执行控制面。

决策和执行仍是两个清晰的子域，但共享一个 Coordination 所有权边界：

- Event/Mode/Effect 决定协作方式；
- Execution/Worker 负责排队、并发、租约、重试、取消与结果；
- AgentHost 负责把已批准的 `ExecutionSpec` 物化成 Agent loop；
- SQLite adapter 分表保存 effect 与 execution，二者通过 Coordination ID 关联。

控制接口：

- `GET /api/coordination/modes`：返回唯一的 `board_autonomy`；
- `PUT /api/issues/:id/coordination`：保存 `board_autonomy` binding 和 `capabilityPolicy`；
- `GET /api/issues/:id/coordination`：读取 Task 根绑定；
- `GET /api/coordination/capabilities`：查看宿主已注册能力；
- `POST /api/coordination/capabilities/plan`：执行前预览最终能力计划；
- `POST /api/coordination/invoke`：主动让指定 Agent 工作。

这套边界刻意区分“注册/安装”和“授权”：宿主负责安装 Source 或 MCP Connector，Coordination 负责逐 Task、逐 delegation 决定是否把它交给 Agent。
