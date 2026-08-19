# agentapp

`agentapp` 是无具体业务依赖的 Agent Phone Kernel，用来承载同时面向人类和 AI Agent 的 HTTP/文本应用界面。平台总边界见 [`../docs/architecture/agent-application-platform.md`](../docs/architecture/agent-application-platform.md)。

它提供：

- Agent App manifest 与注册表；
- Agent Phone 会话、已安装软件、App 页面栈和表单草稿；
- 带短期 `REF` 和 `pageRevision` 的 AI 文本页面；
- `click`、`input`、`select`、`toggle`、`submit`、`back`、`home`、`refresh`；
- AI-first 语义手势：`scroll`、`swipe`、`long_press`、`drag`、`load_more`；
- 幂等动作、过期页面保护、权限边界和操作审计接口；
- 可发现、最多 20 个、按使用频率裁剪的 Phone 快捷指令；
- 人类 Web UI 与并排 AI View；
- 通用远程 Client 与 AgentCore Capability adapter。

Board、Relay、它们的 Repository 合同和协议演示已经迁入 [`../apps/board/phone`](../apps/board/phone)。Phone Kernel 不内置任何产品应用；宿主必须显式注册 Board 或其他应用的 Phone 模块。

## 独立运行

```bash
go run ./cmd/agentapp
```

打开 `http://localhost:8090`。页面左侧是人类 Web 前端，右侧是 AI 实际读取的文本前端。

独立命令默认把 Phone 状态和系统日志保存到 `data/agentapp.db`。可通过环境变量修改：

```bash
AGENTAPP_DB=/var/lib/aegis/agentapp.db AGENTAPP_ADDR=:9090 go run ./cmd/agentapp
```

可以通过环境变量修改监听地址：

```bash
AGENTAPP_ADDR=:9090 go run ./cmd/agentapp
```

## 嵌入其他服务

```go
registry := agentapp.NewRegistry()
_ = registry.Register(boardphone.NewCoordinatedBoardApp(productionBoardRepository, boardCoordinator))
_ = registry.Register(boardphone.NewRelayApp(productionRelayRepository))

phone := agentapp.NewPhone(registry, productionAuditor)
server := agentapp.NewHTTPServer(registry, phone)

http.Handle("/agent-apps/", http.StripPrefix("/agent-apps", server))
```

SQLite + GORM 持久化模式：

```go
store, _ := agentapp.OpenSQLiteStore("data/agentapp.db")
phone := agentapp.NewPersistentPhone(registry, store, productionAuditor)
server := agentapp.NewHTTPServer(registry, phone)
```

GORM 会自动迁移 `agent_app_phone_sessions` 和 `agent_app_audit_events` 两张表。Phone 状态包含当前 App、完整页面、私有 REF 目标、各 App 导航栈、未提交草稿和幂等结果；新进程通过同一个 `phoneSessionId` 按需恢复。Aegis 集成要求同时提供 `taskId` 与 `taskAgentId`：一部 Phone 属于一个任务内 Agent 实例，不会按 Agent 类型跨任务复用。

生产服务应传入认证器。模块内置了适合测试和内部部署的静态 Bearer Token 实现，也可以接入自己的 JWT/mTLS 验证器：

```go
server := agentapp.NewAuthenticatedHTTPServer(registry, phone, agentapp.BearerTokens{
    "short-lived-token": {
        AgentID: "backend-agent", TaskID: "task-123", TaskAgentID: "task-agent-456",
        ExecutionID: "execution-123",
        WorkspaceID: "/workspace/repo", Scopes: []string{"board:read:all"},
    },
})
```

认证服务会把 Phone Session 绑定到 Token 对应的 Task、TaskAgent 和 Agent 类型。请求不能伪造这些身份或 Token 已绑定的 Execution/Workspace；需要由一个短期服务 Token 代为创建多个 Execution 时，必须显式授予对应 bind scope。其他任务或 TaskAgent 即使获得 Session ID 也不能读取或操作页面。`NewHTTPServer` 明确用于本地开发，会信任创建会话时提交的身份。

Board 生产接入实现以下 Board-owned 接口（定义位于 `apps/board/phone`）：

```go
type BoardRepository interface {
    ListIssues(context.Context, Actor, string) ([]BoardIssue, error)
    GetIssue(context.Context, Actor, string) (BoardIssue, []BoardComment, error)
    AddComment(context.Context, Actor, string, string, string) (BoardComment, error)
}

type RelayRepository interface {
    Inbox(context.Context, Actor) ([]RelayThread, error)
    Messages(context.Context, Actor, string) ([]RelayMessage, error)
    Send(context.Context, Actor, string, string, string) (RelayMessage, error)
}

type BoardCoordinator interface {
    Delegate(context.Context, Actor, BoardDelegationRequest, string) error
    Continue(context.Context, Actor, string, string, string) error
    Wait(context.Context, Actor, []string, int64, string, string) error
}
```

需要委派、继续和休眠功能时，使用 `boardphone.NewCoordinatedBoardApp(repository, coordinator)`；普通 `boardphone.NewBoardApp` 仍提供列表、详情和评论。Board 与 Relay 都实现可选的 `ShortcutApp`，Phone 只会发现当前 Session 已安装 App 的快捷指令，按 `Frequency` 从高到低选择前 20 个。

## HTTP 接口

| Method | Path | 用途 |
| --- | --- | --- |
| `GET` | `/.well-known/agent-apps.json` | 列出已注册软件 |
| `GET` | `/apps/{appId}/.well-known/agent-app.json` | 获取软件 manifest |
| `POST` | `/phone/sessions` | 创建 Phone 会话并返回 Home |
| `GET` | `/phone/sessions?taskId={taskId}&taskAgentId={taskAgentId}` | 列出该任务实例可恢复的 Phone 及当前 App/页面 |
| `GET` | `/phone/sessions/{id}/page` | 读取当前页面 |
| `GET` | `/phone/sessions/{id}/logs?limit=100` | 读取成功与失败操作日志（最新在前） |
| `GET` | `/phone/sessions/{id}/shortcuts` | 发现当前 Phone 最多 20 个高频快捷指令 |
| `POST` | `/phone/actions` | 对当前页面的 REF 执行动作 |
| `POST` | `/phone/shortcuts` | 通过 Phone 执行一个语义快捷指令 |

页面接口指定 `Accept: text/agent-ui` 时返回纯文本；默认返回 JSON 和同一份规范化文本。

内存模式下每个 Phone Session 默认最多保留 2,000 条日志；GORM 模式不受该内存上限约束。日志覆盖 Phone 启动、所有页面动作、幂等重放和动作错误，包含 App、Page、Revision、REF、结果、Effect、错误码与错误信息。输入值会在日志中脱敏。宿主额外传入 `Auditor` 时，事件也会同步投递到宿主审计系统。

Agent Runtime 可以使用模块自带的通用 HTTP `Client`，不需要依赖具体软件：

```go
client := agentapp.Client{BaseURL: "http://127.0.0.1:8090"}
started, _ := client.Start(ctx, agentapp.StartSessionRequest{
    AgentID: "backend-agent", TaskID: "task-123", TaskAgentID: "task-agent-456",
})
next, _ := client.Act(ctx, agentapp.ActionRequest{
    PhoneSessionID: started.PhoneSessionID,
    PageRevision: started.Page.Revision,
    Action: agentapp.ActionOpenApp,
    Ref: "@1",
    IdempotencyKey: "open-board-once",
})

shortcuts, _ := client.Shortcuts(ctx, started.PhoneSessionID)
delegated, _ := client.RunShortcut(ctx, agentapp.ShortcutRequest{
    PhoneSessionID: started.PhoneSessionID,
    Name: "phone_board_delegate",
    Arguments: map[string]any{"children": []any{
        map[string]any{"agentId": "backend-agent", "prompt": "实现并验证接口"},
    }},
    IdempotencyKey: "delegate-child-once",
})
```

## AI-first 手势

AI 操作不使用像素坐标。页面文本会为每个 REF 输出允许动作，例如：

```text
@1 [issue] ISSUE-142 · Fix timeout
    Actions: click, long_press
@6 [viewport] Load more issues · 3 remaining
    Actions: scroll, load_more
@back [system] Back · Actions: back, swipe
```

滚动和滑动使用有限枚举：

```json
{"action":"scroll","ref":"@6","arguments":{"direction":"down","amount":"page"}}
{"action":"swipe","ref":"@back","arguments":{"direction":"right","distance":"medium"}}
```

拖动同时校验来源和目标 REF，不接受业务 ID 或像素坐标：

```json
{"action":"drag","ref":"@card","arguments":{"targetRef":"@done-column"}}
```

目标必须是当前页面声明的 `drop_target`。Phone 会在服务端将目标 REF 解析成内部对象，再交给 App 执行。所有成功、参数错误、无效目标和 App 错误都会进入系统日志。

## 边界

当前包只负责 Phone 协议、导航、会话、幂等、持久化、审计和 transport，不负责 Board、Agent 调度、DataSpace 业务模型或领域迁移。应用通过平台 SDK 申请 Agent Work，并在自己的持久化 Inbox 消费生命周期事件。
