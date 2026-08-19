# Agent Phone 与 AI UI 子协议

> 状态：Accepted / 子规范
> 适用范围：Agent Phone、双前端、页面、REF、Action、Shortcut 与 Phone 审计
> 上位规范：[`agent-application-platform.md`](agent-application-platform.md)
> 核心目标：同一份应用能力同时提供人类可视 Web 前端和 AI 可操作文本前端，并可安装到 Agent Phone 中。

本文不再定义应用的数据所有权、Agent 调度或仓库边界。应用通过上位规范中的 Agent Work SDK 异步申请 Agent，并通过 App Inbox 消费生命周期事件。Board 是普通应用；Task、Issue、验收、返工、Relay 和 `board_autonomy` 都属于 Board。本文后续保留的 Board/Relay 分节用于描述 Phone 页面和迁移基线，其中 Relay 在 V1 是 Board 的内部子模块，不是平台强制安装的软件。

## 1. 背景与目标

当前 Aegis Board 已经包含两类重要协作能力：

- **Board**：任务、Issue、依赖、执行、验收和状态流转；
- **Relay**：评论、mention、广播、Agent 间异步消息和唤醒。

目标架构把完整的 Aegis Board 定义为一个标准 **Agent Application**，Board 和 Relay 是该应用内部的两个 Phone 模块。一个应用可以与平台同进程部署，也可以通过稳定协议独立部署，并可提供两套面向不同使用者的前端：

1. **Web 前端**：供人类查看和操作；
2. **AI 文本前端**：供 Agent 读取当前页面、发现可操作对象并执行点击、输入、返回等动作。

每个 Agent 拥有一台逻辑上的 **内部手机（Agent Phone）**。手机中可以安装多个应用；安装 Board 后可进入 Board 与 Relay 两个页面模块。Agent 可以从 Home 打开软件、浏览页面、执行操作、返回上一页或回到 Home。

本设计希望解决以下问题：

- Agent 不需要理解每个业务服务的内部数据库或私有 API；
- 人类和 Agent 操作同一个业务状态，但分别使用适合自己的表现形式；
- Agent 在每一步都能明确知道“当前看到什么”和“现在可以做什么”；
- 所有 Agent 操作都可以授权、审计、重放和调试；
- Web 前端可以预览 AI 当前看到的文本页面，便于排查误操作。

## 2. 核心概念

### 2.1 Agent 软件

Agent 软件是一个通过 HTTP 暴露业务能力的独立应用。它可以是单体服务，也可以是 Aegis 单体中的逻辑模块，但对外必须遵守统一的 App Contract。

每个软件至少包含：

- 一个唯一 `appId`；
- 软件名称、图标、版本和能力声明；
- Web 前端入口；
- AI 文本前端入口；
- 会话与页面导航能力；
- 结构化动作执行接口；
- 身份认证、权限控制和审计日志；
- 可选的事件订阅接口。

Board 应用的 Phone 模块：

| phoneModuleId | 名称 | 主要用途 |
| --- | --- | --- |
| `aegis.board` | Board | 查看任务/Issues、详情、依赖、执行进度、验收与状态 |
| `aegis.board.relay` | Relay | Board 内的私信、Issue 线程、任务广播、mention 与消息收件箱 |

### 2.2 双前端

双前端不是两套业务逻辑。两者必须读取同一服务端状态、调用同一应用服务层，并遵循同一权限规则。

```text
                   ┌─ Web UI（HTML/React，人类使用）
Agent App Server ──┤
                   └─ AI UI（结构化文本，Agent 使用）
                            │
                            └─ action API（click/input/submit/back...）
```

Web UI 负责视觉布局、图表、拖拽和高密度信息展示。AI UI 负责语义清晰、低 token、确定性导航和可审计操作。

### 2.3 AI 文本页面

AI 文本页面是软件当前状态的可操作文本快照，类似一个专门给 AI 使用的文本浏览器。它必须同时回答三件事：

1. 我在哪个软件、哪个页面？
2. 当前最重要的信息是什么？
3. 我可以对哪些对象执行什么操作？

它不是把 HTML 去掉标签，也不是无边界地导出数据库。服务端需要按页面语义主动组织信息，并优先展示 Agent 一眼应看到的内容。例如 Board 首页优先展示当前任务、待处理 Issues、阻塞和 mention，而不是先展示系统设置。

### 2.4 REF

页面中每个可交互对象都分配一个短期有效的 **REF 索引**，例如 `@1`、`@2`。Agent 不需要猜 URL、DOM selector 或数据库主键，而是通过 REF 执行动作。

REF 可以指向：

- 链接、按钮、标签页；
- Issue、消息、附件等业务对象；
- 输入框、选择器、复选框；
- 分页、展开、刷新等页面命令。

REF 仅在当前页面快照或明确的有效期内有效。每次页面变化都会返回新的 `pageRevision` 和 REF 集合，防止 Agent 使用过期引用误操作。

### 2.5 Agent 内部手机

Agent Phone 是 Agent 访问软件的统一客户端和导航容器，不是移动端像素界面的模拟器。它负责：

- 展示已安装软件；
- 打开、切换、关闭软件；
- 维护每个软件的页面栈；
- 提供 Back、Home、App Switcher；
- 保存登录身份和授权范围；
- 管理通知、未读数和深链接；
- 将页面内容提供给 Agent 上下文；
- 将 Agent 动作转发给软件服务端；
- 记录完整的查看与操作审计。

Phone 是客户端，Board 是服务端应用，Relay 是 Board 的协作子模块。不同 Agent 可以拥有不同的应用安装列表和权限。

## 3. 总体架构

```text
┌──────────────────────── Human Browser ────────────────────────┐
│ Web UI                                                        │
│  ├─ 正常视觉页面                                               │
│  └─ AI View 预览窗（只读或调试模式）                            │
└───────────────────────────┬────────────────────────────────────┘
                            │ HTTP / SSE or WebSocket
┌────────────────────── Agent App Server ───────────────────────┐
│ App manifest                                                  │
│ Shared application/domain services                            │
│ Web presentation API      AI page renderer + action resolver  │
│ Auth / policy / audit     REF registry / idempotency           │
│ Persistence               Event publisher                      │
└───────────────────────────┬────────────────────────────────────┘
                            │ HTTP
┌──────────────────────── Agent Phone ──────────────────────────┐
│ Home / installed apps / navigation stacks / notifications     │
│ AI text browser / action client / credentials / local state   │
└───────────────────────────┬────────────────────────────────────┘
                            │ tool call or agentcore Tool
                        Agent Runtime
```

部署方式允许两种形态：

- **独立部署**：完整 Board Application 运行独立 HTTP 服务，内部同时提供 Board 与 Relay Phone 模块；
- **模块化单体**：早期仍运行在平台 Host 内，但使用独立路由、manifest、DataSpace 和应用边界，后续可拆分。

第一阶段建议使用模块化单体，先稳定协议，再根据扩展和隔离需要拆成独立服务。

## 4. Agent App Contract

### 4.1 软件清单

每个软件公开：

```http
GET /.well-known/agent-app.json
```

示例：

```json
{
  "appId": "aegis.board",
  "name": "Board",
  "version": "1.0.0",
  "web": { "entry": "/" },
  "ai": {
    "protocolVersion": "1.0",
    "entry": "/ai/pages/home",
    "actions": "/ai/actions",
    "contentTypes": ["text/agent-ui", "application/agent-ui+json"]
  },
  "events": { "stream": "/api/events" },
  "capabilities": ["navigate", "click", "input", "submit", "refresh"]
}
```

### 4.2 AI 页面接口

```http
GET /ai/pages/{route}
Accept: application/agent-ui+json
Authorization: Bearer <agent-app-token>
X-Agent-Id: <stable-agent-id>
X-Phone-Session-Id: <session-id>
```

建议服务端返回 JSON 包装和规范化文本，Agent 主要读取 `text`，Phone 使用结构化字段做校验：

```json
{
  "appId": "aegis.board",
  "pageId": "issues.inbox",
  "pageRevision": "rev_01K...",
  "title": "My Issues",
  "text": "...规范化文本...",
  "refs": [
    {
      "ref": "@1",
      "kind": "link",
      "label": "ISSUE-142 修复登录超时",
      "actions": ["click"]
    }
  ],
  "navigation": {
    "canBack": true,
    "canHome": true
  }
}
```

对于模型原生只接受文本的场景，也可请求：

```http
Accept: text/agent-ui
```

此时返回相同语义的纯文本表示。

### 4.3 规范化文本格式

V1 使用易读、可流式传输的行式文本，不要求 Agent 解析 HTML：

```text
[APP] Board (aegis.board)
[PAGE] My Issues
[REVISION] rev_01K...
[SUMMARY]
Assigned: 4 | Blocked: 1 | Needs review: 2 | Mentions: 3

[SECTION] Needs attention
@1 [issue] ISSUE-142 · Fix login timeout · in_progress · P0
    Owner: backend-agent · Updated: 8m ago
    Actions: click, open_menu
@2 [issue] ISSUE-139 · Validate release evidence · review · P1
    Actions: click

[SECTION] Actions
@3 [button] Create issue
@4 [input:text name=query] Search issues
@5 [button] Submit search

[NAV]
@back [system] Back
@home [system] Home
@refresh [system] Refresh
```

格式要求：

- 顶部固定包含 App、Page、Revision 和摘要；
- 先展示异常、待办和当前目标，再展示普通列表；
- 每个可操作元素必须有 REF、类型、标签和允许动作；
- 非交互文本不分配 REF；
- 列表默认限制条数并提供分页或“查看更多”；
- 时间同时提供人类可读值和结构化时间戳；
- 危险操作在文本中明确标记 `[DANGEROUS]`；
- 不在页面中泄露 Agent 无权查看的对象及其 REF；
- 页面内容必须稳定排序，避免同一状态产生无意义变化。

### 4.4 动作接口

所有操作统一提交到：

```http
POST /ai/actions
Idempotency-Key: <unique-key>
Content-Type: application/json
```

请求：

```json
{
  "phoneSessionId": "phone_s_123",
  "pageRevision": "rev_01K...",
  "action": "click",
  "ref": "@1",
  "arguments": {}
}
```

成功响应直接返回动作后的新页面，不让 Agent 再额外请求一次：

```json
{
  "status": "ok",
  "effect": "navigated",
  "page": { "pageId": "issue.detail", "pageRevision": "rev_01M...", "text": "..." },
  "toast": "Opened ISSUE-142"
}
```

V1 标准动作：

| 动作 | 用途 | 必要参数 |
| --- | --- | --- |
| `click` | 打开链接、按钮、标签 | `ref` |
| `input` | 向输入控件写入文本 | `ref`, `arguments.value` |
| `select` | 选择单选或下拉项 | `ref`, `arguments.value` |
| `toggle` | 切换复选框/开关 | `ref`, `arguments.checked` |
| `submit` | 提交当前表单 | `ref` |
| `back` | 返回当前 App 上一页 | 系统 REF `@back` |
| `home` | 返回 Phone Home | 系统 REF `@home` |
| `refresh` | 刷新页面快照 | 系统 REF `@refresh` |
| `open_app` | 从 Phone 打开软件 | Phone Home 上的 App REF |
| `scroll` | 语义滚动可加载区域 | `ref`, `direction`, `amount` |
| `swipe` | 系统返回/Home 或 App 声明的滑动 | `ref`, `direction`, `distance` |
| `long_press` | 打开 REF 的上下文操作页 | `ref` |
| `drag` | 将来源 REF 拖到当前页目标 REF | `ref`, `arguments.targetRef` |
| `load_more` | 显式加载更多内容 | `ref` |

`input` 默认先更新 Phone 侧表单草稿，不立即产生业务副作用；只有 `submit` 才提交表单。涉及删除、发布、批准、取消任务等高风险动作时，服务端返回确认页面或审批要求，不能因为一次普通 `click` 直接完成不可逆操作。

AI 前端优先使用 `load_more`、状态变更等语义动作；仅在滑动本身具有业务含义时使用 `scroll/swipe/drag`。手势参数使用有限枚举，不接受像素坐标。`drag` 的来源和目标必须都是当前 `pageRevision` 中的有效 REF，目标还必须声明为 `drop_target`，服务端解析后才会把内部对象交给 App。

### 4.5 错误与冲突

统一错误码：

- `STALE_PAGE`：`pageRevision` 已过期，响应附最新页面；
- `INVALID_REF`：REF 不存在或不属于当前页面；
- `ACTION_NOT_ALLOWED`：该 REF 不支持请求动作；
- `VALIDATION_ERROR`：输入不符合约束，返回字段错误和保留的草稿；
- `PERMISSION_DENIED`：Agent 无权查看或执行；
- `CONFIRMATION_REQUIRED`：需要进入确认页；
- `APP_UNAVAILABLE`：软件服务不可用；
- `RATE_LIMITED`：操作过于频繁；
- `CONFLICT`：业务对象已被其他操作者修改。

失败响应也应尽可能返回当前可继续操作的页面，避免 Agent 卡死在只有错误字符串的状态。

## 5. Agent Phone 设计

### 5.1 Phone 状态

```go
type AgentPhone struct {
    ID            string
    AgentID       string
    InstalledApps []InstalledApp
    ActiveAppID   string
    AppStacks     map[string][]PageLocation
    Notifications []PhoneNotification
    SessionID     string
}
```

一个 Agent 类型不拥有全局 Phone。系统在 Task 中为每个被分派的 Issue 领取一个 `TaskAgent` 身份，并给该身份创建一部 Phone；同一 Agent 类型可在同一 Task 中存在多个实例。Phone 可跨该实例的 Execution 恢复导航、草稿、未读通知和安装列表，但任何状态都不能跨 Task 读取或复用。

### 5.2 Home 页面

```text
[PHONE] backend-agent's phone
[PAGE] Home
[SUMMARY] 2 apps | 4 unread notifications

[SECTION] Apps
@1 [app] Board · 3 assigned · 1 blocked
@2 [app] Relay · 4 unread · 2 mentions

[SECTION] Notifications
@3 [notification] ISSUE-142 has a new reviewer comment
@4 [notification] frontend-agent mentioned you in Release channel

[NAV]
@home [system] Home
```

点击通知使用深链接直接打开对应 App 和页面，同时把该页面压入该 App 的导航栈。

### 5.3 提供给 Agent Runtime 的工具

Phone 在 Agent Runtime 中暴露少量稳定工具，而不是为每个软件生成大量业务工具：

```text
phone_view()                         获取当前页面
phone_action(action, ref, arguments) 执行当前页面动作
phone_back()                         返回上一页
phone_home()                         返回 Home
```

其中 `phone_back` 和 `phone_home` 也可以仅作为 `phone_action` 的语法糖。工具结果始终返回完整的新页面文本、revision 和简短操作结果。

Phone Session 应限制连续操作步数、单页字符数和列表条数。达到限制时 Agent 必须总结目的并决定继续、等待或退出 App，防止无目的浏览消耗 token。

## 6. Board Agent App

### 6.1 Web 前端

沿用现有任务与 Issues 页面，补充：

- 顶栏提供 `Visual / AI View` 切换；
- 桌面端可打开右侧 AI View 调试抽屉；
- 抽屉展示当前路由对应的 AI 文本页面、revision 和 REF；
- 人类点击一个 REF 时，高亮 Web 页面对应元素；
- Web 页面操作后，AI View 自动刷新；
- 调试模式可以模拟指定 Agent 身份，检查其权限下能看到什么；
- 生产环境默认只允许管理员使用身份模拟。

AI View 是预览和调试工具。人类在其中执行动作时必须明确标记为人类调试操作，审计日志不能伪装成 Agent 行为。

### 6.2 AI 页面信息架构

Board 首页按以下优先级输出：

1. 当前 Agent 的紧急 mention、审批和返工；
2. 已分配且正在处理的 Issues；
3. 被阻塞或执行异常的 Issues；
4. 待验收和即将超预算的 Issues；
5. 其他任务和搜索入口。

主要页面：

- `board.home`
- `issues.my`
- `issue.detail`
- `issue.comments`
- `issue.dependencies`
- `issue.executions`
- `execution.detail`
- `task.tree`
- `approvals.inbox`
- `search.results`

Issue 详情页必须优先展示：目标、状态、负责人、阻塞原因、最新进度、未读评论、交付证据和允许动作。长消息、完整事件和附件正文通过 REF 按需展开。

### 6.3 Board 动作

首期支持：

- 打开 Issue/Task/Execution；
- 搜索和筛选；
- 创建 Issue；
- 添加评论和结构化 mention；
- checkout、释放或完成允许的 Issue；
- 请求返工；
- 提交验收结果；
- 查看附件和证据清单；
- 取消任务、批准等高风险动作的确认流程。

## 7. Board Relay Phone 模块

### 7.1 定位

Relay 是 Board 应用内部的 Agent 通信模块，不负责改变 Issue 的业务状态。它传递消息、引用和通知；Board domain 仍是任务事实源。Relay 消息可以引用 Board 对象，点击引用通过 Phone 深链接打开 Board。若未来 Relay 需要独立服务多个应用，必须按上位规范注册为独立 Application，并通过跨应用事件和权限合同通信。

### 7.2 通信模型

V1 支持：

- **Direct Thread**：两个或多个 Agent 的持续会话；
- **Issue Thread**：绑定一个 Issue 的讨论；
- **Task Channel**：任务树范围的广播；
- **Inbox**：与当前 Agent 有关的未读、mention 和系统通知。

消息结构至少包含：

```json
{
  "id": "msg_123",
  "threadId": "thread_456",
  "sender": { "type": "agent", "id": "backend-agent" },
  "body": "API contract updated; please use v2.",
  "mentions": ["frontend-agent"],
  "references": [
    { "appId": "aegis.board", "type": "issue", "id": "ISSUE-142" }
  ],
  "createdAt": "2026-07-29T10:00:00Z"
}
```

### 7.3 Relay AI 页面

主要页面：

- `relay.inbox`
- `relay.thread.list`
- `relay.thread.detail`
- `relay.compose`
- `relay.search`

Inbox 先展示 mention、等待回复和高优先级系统消息。Thread 详情默认只展示最近消息和未读边界，较早历史通过 REF 加载，避免每次把完整聊天记录注入上下文。

首期动作：

- 打开会话；
- 回复、mention Agent；
- 新建 Direct Thread；
- 向 Task Channel 广播；
- 标记已读/未读；
- 搜索消息；
- 打开 Board 引用；
- 引用或转发消息。

### 7.4 服务端投递

Relay 服务端负责：

- 持久化消息后再确认发送成功；
- 为接收方生成未读和 Phone 通知；
- mention 触发 Agent Wakeup，但必须做幂等和频率限制；
- 在线 Agent 通过 SSE/WebSocket 接收事件；
- 离线 Agent 在下次打开 Phone 时读取未处理通知；
- 保存投递、读取、失败、重试和唤醒审计记录。

Relay 不能因为一条消息直接绕过 Board checkout、审批或权限约束。消息是输入，不自动成为可信事实。

## 8. 服务端公共能力

### 8.1 身份与授权

Phone 使用短期 Agent App Token 调用软件。Token 至少绑定：

- `agentId`；
- `phoneSessionId`；
- `appId`；
- scopes；
- workspace/task 范围；
- 过期时间。

服务端必须在生成页面和执行动作两个阶段分别检查权限。不能只隐藏 Web 按钮而允许 AI action API 直接调用。

### 8.2 REF 注册表

REF 由服务端页面渲染器创建，推荐保存如下映射：

```text
(phoneSessionId, appId, pageRevision, ref)
    -> targetType, targetId, allowedActions, fieldSchema, expiresAt
```

映射可存在短期内存/Redis 中，也可以把不可伪造的目标信息签名编码在 opaque token 中。对 V1 单机 Aegis，使用带 TTL 的内存注册表即可，服务重启后要求刷新页面。

### 8.3 幂等和并发

- 所有产生副作用的动作必须带 `Idempotency-Key`；
- action 先校验页面 revision，再校验对象版本；
- Issue、消息和审批等对象使用版本号或更新时间做乐观并发；
- 重复动作返回第一次结果，不重复创建评论、消息或 Issue；
- 页面陈旧时返回最新页面，让 Agent 重新决策，不自动套用旧操作。

### 8.4 事件

应用可以发布：

```text
app.notification.created
page.resource.updated
board.issue.updated
board.assignment.created
relay.message.created
relay.mention.created
```

事件用于刷新页面、生成 Phone 通知和唤醒 Agent，不把高频事件逐条塞入模型上下文。Phone 应聚合通知，例如“3 个 Issues 有更新”，由 Agent 主动打开查看。

### 8.5 审计

每次操作记录：

- Agent、人类或系统身份；
- Phone Session、App、页面和 revision；
- 动作、REF、解析后的目标；
- 输入摘要及敏感字段脱敏结果；
- 权限决策；
- 业务结果与错误码；
- 幂等键、请求 ID、时间和耗时。

页面“被查看”也可按安全等级记录，但不应永久保存每一份可能包含敏感信息的完整文本快照。需要重放时保存页面模板版本、对象版本和经过脱敏的摘要。

## 9. Web 与 AI 前端一致性

双前端必须共享领域查询与命令，不允许各自直接拼业务规则：

```text
BoardQueryService.GetIssueView(actor, issueId)
        ├─ Web presenter -> React view model
        └─ AI presenter  -> AgentPage + refs + text

BoardCommandService.AddComment(actor, command)
        ├─ Web API
        └─ AI action resolver
```

应为每个关键页面维护“语义契约测试”：给定同一身份和数据，Web 与 AI 前端必须展示同一核心状态、允许相同业务动作。允许表现和信息密度不同，但不能出现 Web 显示 `done`、AI 显示 `in_progress`，或一端允许越权操作的情况。

## 10. 数据模型建议

新增或抽象以下实体：

| 实体 | 关键字段 | 用途 |
| --- | --- | --- |
| `AgentApp` | id, name, version, baseURL, manifest, status | 软件注册 |
| `AgentAppInstallation` | agentId, appId, config, scopes, installedAt | Agent 安装的软件 |
| `PhoneSession` | id, taskId, taskAgentId, agentId, executionId, activeAppId | 一个任务内 Agent 实例的持久手机会话 |
| `PhoneNavigationState` | sessionId, appId, stack, draft | 页面栈和表单草稿 |
| `PhoneNotification` | agentId, appId, deepLink, priority, readAt | 跨软件通知 |
| `AgentAppActionAudit` | actor, appId, pageRevision, ref, action, result | 操作审计 |
| `RelayThread` | id, type, taskId, issueId | 通信线程 |
| `RelayParticipant` | threadId, actorType, actorId, readCursor | 参与者和已读游标 |
| `RelayMessage` | id, threadId, sender, body, references, createdAt | 持久消息 |

现有 Issue、Comment、Broadcast、Wakeup 数据无需立即迁移。第一阶段可以让 Relay App 适配现有表，协议稳定后再统一为 Relay 数据模型。

## 11. 安全约束

- 文本页面是新的数据出口，必须复用与 Web API 相同或更严格的行级权限；
- 页面渲染前过滤凭据、密钥、访问令牌和高敏附件；
- 外部消息、Issue 内容和附件均是不可信输入，不能把其中伪造的 `[REF]` 当作真实控件；
- REF 只能来自服务端独立结构，文本中的 `@1` 字符串不能自行获得权限；
- 危险动作必须确认或审批，并对 Agent 权限、任务范围和授权时间窗再次检查；
- 应限制单次页面尺寸、动作频率、导航深度和跨 App 跳转次数；
- Relay mention 必须限制自唤醒、循环 mention 和多 Agent 唤醒风暴；
- Agent App Token 只能发送到安装的软件域名，禁止任意 URL 转发；
- 附件访问使用短期签名 URL 或受控读取接口，不在文本页面内直接暴露宿主机路径。

## 12. 可观察性指标

至少记录：

- 各 App 的页面打开次数、操作成功率和错误码；
- `STALE_PAGE`、`INVALID_REF`、权限拒绝和确认取消比例；
- Agent 完成一次目标所需的页面数、动作数和 token；
- Phone Home、Back 和跨 App 跳转频率；
- Board 中从分配到完成/验收的耗时；
- Relay 的投递延迟、已读延迟、mention 唤醒次数与循环率；
- Web 与 AI 页面语义契约不一致次数；
- AI View 中人类调试复现成功率。

## 13. 实施阶段

### Phase 0：协议原型

- 定义 `AgentPage`、`Ref`、`ActionRequest/Response` Go 类型；
- 实现 `text/agent-ui` 渲染器和页面 revision；
- 为一个只读 Board “My Issues” 页面生成文本；
- 实现 `click`、`back`、`home`、`refresh`；
- 在 Web 页增加只读 AI View 抽屉；
- 完成过期 REF、越权 REF 和提示注入测试。

验收：Agent 能从 Phone Home 打开 Board，读取 Issues，点击进入详情并返回；人类能在 Web 中看到完全相同的 AI 页面。

### Phase 1：Board App 可操作化

- 增加 input/select/submit 和表单草稿；
- 支持评论、mention、搜索、Issue 状态操作；
- 接入幂等、乐观并发、确认页和审计；
- 将现有 `aegis_board` 工具逐步改为 Phone/Board App 适配器；
- 保留旧工具兼容层，记录两种路径的行为差异。

验收：Agent 不调用 Board 私有数据库或旧专用工具，也能完成“发现 Issue → 查看详情 → 评论 → 提交结果”的闭环。

### Phase 2：Board Relay 模块

- 将现有 Comment、Broadcast、Mention、Wakeup 暴露为 Relay 页面和动作；
- 实现 Inbox、Direct Thread、Issue Thread、Task Channel；
- 支持未读游标、引用、深链接和通知聚合；
- 将 `aegis_relay` 工具改为 Phone/Relay App 适配器；
- 加入 mention 循环检测和唤醒限流。

验收：两个 Agent 通过 Relay 交换消息，接收者从通知进入线程，并能点击 Board 引用打开对应 Issue。

### Phase 3：完整 Agent Phone

- 持久化软件安装、通知和导航状态；
- 提供 App Store/管理员安装策略；
- 支持软件健康检查、版本协商和下线状态；
- 将 Phone 工具集成到 agentcore；
- 增加会话动作预算、跨 App trace 和恢复机制。

验收：不同 Agent 拥有不同软件和权限；服务重启或 Execution 恢复后，Phone 能安全恢复通知与合理的导航状态。

### Phase 4：开放第三方 Agent Application

- 发布 App Contract SDK 和一致性测试套件；
- 支持远程软件注册和受控网络访问；
- 增加软件签名、信任级别、权限申请与人工安装审批；
- 提供开发者 AI View 调试器。

## 14. 测试策略

### 单元测试

- 相同状态生成稳定页面与稳定排序；
- REF 只允许声明的动作；
- 页面 revision 变化后旧 REF 失效；
- 输入验证和草稿保留；
- 不同权限身份生成不同但正确的页面；
- 文本中的伪 REF、指令注入和恶意 Markdown 不产生控件。

### 契约测试

- manifest、页面、动作和错误结构符合协议；
- Web/AI 共享核心字段和允许动作；
- Board 与 Relay 深链接可互相打开；
- App 版本不兼容时 Phone 给出可恢复错误。

### 端到端测试

1. Agent 打开 Phone Home；
2. 打开 Board，发现一个分配 Issue；
3. 进入详情并读取未读评论；
4. 打开 Relay 引用，与另一 Agent 沟通；
5. 返回 Board，提交评论和工作结果；
6. 人类在 Web 与 AI View 中复核全过程；
7. 审计记录能串起同一个跨 App trace。

### 安全测试

- 越权对象枚举和 REF 伪造；
- 过期 revision 重放；
- 重复 submit；
- 恶意消息诱导 Agent 执行危险动作；
- mention 循环和通知风暴；
- Token 被错误 App 使用；
- AI View 身份模拟越权。

## 15. 关键设计决定

1. **AI 前端是服务端语义渲染，不是 HTML 转文本。** 这样才能保证信息优先级、低 token 和稳定动作。
2. **REF 是短期页面能力，不是永久对象 ID。** 这可以防止旧上下文和猜测 ID 造成误操作。
3. **动作成功后直接返回新页面。** 减少一次网络与模型工具调用，并保证 Agent 立刻获得新状态。
4. **Phone 只提供少量通用工具。** 新软件可以通过协议安装，不需要每安装一个软件就重新扩展模型工具定义。
5. **Web 与 AI 前端共享应用服务层。** 双前端只是两种表现方式，不能产生两套事实和权限逻辑。
6. **Board 应用是任务与协同事实源，Relay 是其内部通信模块。** Relay 可以引用和通知，但不能隐式修改 Board 状态。
7. **优先模块化单体，协议稳定后再拆服务。** 先验证交互模型，避免早期把精力消耗在分布式部署上。

## 16. 暂不纳入 V1

- 让 Agent 读取或操作原始 DOM、CSS selector 和像素坐标；
- 为 AI 完整复制 Web 页面所有装饰性内容；
- 无限制加载完整 Issue 树、聊天历史或 Session 日志；
- Agent 自由安装未经管理员信任的远程软件；
- 跨组织公开 App Store 和付费体系；
- 使用自然语言猜测动作而不提供结构化 action/ref；
- 允许 Relay 消息直接绕过审批执行高风险命令。

## 17. 最小可行验收清单

- [ ] Board 有独立 `appId` 与 manifest，Relay 作为 Board Phone 模块注册；
- [ ] Board 通过共享应用服务提供 Web 与 AI 两套前端；
- [ ] AI 页面包含 App、Page、Revision、摘要、REF 与可用动作；
- [ ] Agent 可执行 click、input、select、submit、back、home、refresh；
- [ ] 动作后返回新的完整页面内容；
- [ ] Web 页面可切换或并排预览 AI View；
- [ ] 每个 Agent 有独立 Phone、软件安装列表和权限；
- [ ] Phone 安装 Board 后可访问 Board 与 Relay 模块，并支持通知和深链接；
- [ ] Board 应用服务负责多方状态和消息传递；
- [ ] 所有副作用支持鉴权、幂等、并发保护和审计；
- [ ] 文本页面中的不可信内容不能伪造 REF；
- [ ] E2E 测试覆盖 Agent 从 Home 到协作再回到任务完成的完整流程。
