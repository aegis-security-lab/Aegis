# Aegis

Aegis 是一个由 Go AgentCore 驱动的本地任务控制台。它把可复用 Agent 定义、任务发布、自动拆解、并行执行、Issues、会话调试、审批与上下文对话放进一个可观察、可干预的后台，不包含模拟执行器。

## 已实现

- 三步首次初始化：产品说明、模型与认证、工作区与安全策略；AgentCore 已内嵌，无需外部 Runtime 探测
- 多页面后台：总览、任务、计划树、Agent 类型、能力控制台、Skills、Sessions、审批中心与设置
- Gin HTTP API 与 SSE 实时状态流
- SQLite + GORM 标准化持久化 Project、Issue、IssueRelation、Execution、消息、评论、审批与 Agent Wakeup
- Go AgentCore Runtime，支持流式消息、并行工具调用、阶段进度、session 统计与执行期 capability 装配
- Agent 是独立的模型覆盖、系统提示词、工具集、Skills 和权限边界组合；模型留空时继承全局配置
- 首次启动自动创建 Orchestrator、后端工程师、前端工程师和红队攻防工程师等可复用 Agent 类型
- Skill 管理支持标准 `SKILL.md` 的新增、编辑、ZIP/Markdown 导入、ZIP 导出与本地路径安装
- Task 是顶层 Issue；Orchestrator 将其拆成彼此独立的子 Issues，子树调度不创建 `blocks` 依赖
- 任意工作 Agent 都能通过 AgentCore capability 递归拆分 2–8 个彼此独立的子 Issues；支持最多 4 层与幂等重试
- 父 Agent 通过 Phone Board 创建子 Issue 后继续工作；没有有价值动作时可调用 `phone_board_sleep`，心跳、评论或 Phone 消息可提前唤醒
- 顶层任务支持树级取消：在调度临界区内取消所有未完成后代与活跃/排队 AgentCore Execution，并关闭待审批和 Agent Wakeup；已完成历史保持不变
- Issues 页面提供可折叠层级树、直属子项进度、未完成 blocker、等待子树与汇总中状态
- 原子 checkout 使用条件 SQL 校验状态、Agent 所有权、Execution 锁与未完成 blockers，冲突返回 409
- 一个 Issue 可以保留多次 Execution；每次 Execution 对应独立 AgentCore Session、消息、事件、统计和审批
- Issue 评论和 Relay 通过任务内 `TaskAgent` 身份精确路由；同一 Agent 类型可以在一个任务中并行创建多个独立实例
- 普通 Agent 可以通过任务树级广播同步跨 Issue 的关键发现；广播持久化后会实时投递给同一任务中正在执行的其他 Worker，并可由后续 Agent 查询历史
- 唯一 `board_autonomy` 协作模式；工具调用与 Issue 返工仍使用独立审批类型和全局策略
- Execution 停止；Issue 人工复核；运行中 Agent 对话；Session 消息和事件调试
- React 19、Tailwind CSS v4、shadcn/ui Base Nova；桌面与移动端布局

## 环境要求

- Docker Desktop 或兼容的 Docker daemon
- Docker Compose v2

只有不使用容器的本地开发流程才需要 Go 1.25+ 与 Node.js 22.19+。

## 启动

```bash
cd /Users/patrick/Code/aegis
cp .env.example .env
# 编辑 .env，至少替换 AEGIS_PASSWORD
make up
```

打开 `http://localhost:8080`，输入启动密码后进入界面。首次访问会进入初始化向导；模型密钥只会写入持久化数据卷中的 `aegis.db`，API 与 UI 只返回是否已配置，不返回密钥内容。除登录入口和容器存活探针 `/healthz` 外，全部 API、SSE、上传和下载都要求有效的会话 Token。

生产服务由 `compose.yaml` 启动。前端会在镜像构建阶段编译并嵌入 Go 二进制，运行容器不包含项目源码；数据保存在 `aegis-data` named volume。Aegis 需要创建隔离的任务 Worker，因此 Compose 会把宿主机的 Docker socket 挂载进应用容器。该挂载等价于宿主机 Docker 管理权限，只应在可信主机上运行。

查看日志或停止服务：

```bash
make logs
make down
```

### 手动部署到生产服务器

仓库包含 `.github/workflows/deploy.yml`，可在 GitHub 的 **Actions → Deploy → Run workflow** 中手动执行。工作流会先运行测试、构建生产镜像，再通过 SSH 部署到 `198.46.216.110`；服务器需要预先安装 Docker 与 Docker Compose v2。

在仓库的 **Settings → Secrets and variables → Actions** 中配置：

| Secret | 是否必需 | 用途 |
| --- | --- | --- |
| `DEPLOY_USER` | 是 | 服务器 SSH 用户，需要有执行 Docker 的权限 |
| `DEPLOY_SSH_PRIVATE_KEY` | 是 | SSH 私钥完整内容 |
| `DEPLOY_KNOWN_HOSTS` | 是 | 服务器 SSH host key，例如在可信环境运行 `ssh-keyscan -H 198.46.216.110` 得到的内容 |
| `AEGIS_PASSWORD` | 是 | Aegis Web 登录密码 |
| `OPENAI_API_KEY` | 否 | OpenAI API token |
| `ANTHROPIC_API_KEY` | 否 | Anthropic API token |
| `OPENCODE_API_KEY` | 否 | OpenCode API token |

首次执行前，将 `DEPLOY_SSH_PRIVATE_KEY` 对应的公钥加入服务器用户的 `~/.ssh/authorized_keys`。手动执行时可覆盖 SSH 端口、应用端口和部署目录；默认分别为 `22`、`8080` 和 `/opt/aegis`。持久数据保存在服务器的 `aegis-data` Docker volume 中，后续部署不会删除该 volume。

前后端开发模式：

```bash
cp .env.example .env
# 编辑 .env，至少替换 AEGIS_PASSWORD
make dev
```

开发模式使用 `compose.dev.yaml` 覆盖生产配置：项目源码映射到容器内的 `/app`，Air 监听 Go 源码并重启后端，Vite 提供前端 HMR。依赖、Go 构建缓存和开发数据使用独立 named volume，不会写入源码目录。Vite 地址为 `http://localhost:5173`，`/api` 会代理到同一开发容器的 `http://localhost:8080`。

停止开发容器：

```bash
make dev-stop
```

如需绕过容器在宿主机直接开发，仍可执行 `AEGIS_PASSWORD='...' make dev-local`。

### Release 二进制

GitHub Release 发布后，CI 会自动构建并上传 Linux AMD64、macOS Apple Silicon 和 Windows AMD64 压缩包及 SHA-256 文件。Release 二进制已经嵌入前端资源，不需要额外携带 `dist` 目录：

```bash
./aegis --password '请替换为强密码' --port 8080
```

也可以使用环境变量，避免密码出现在进程参数中：

```bash
AEGIS_PASSWORD='请替换为强密码' ./aegis --port 8080 --data-dir ./data
```

`--password` 或 `AEGIS_PASSWORD` 至少设置一个，否则服务会拒绝启动。登录成功后生成一个 24 小时随机 Token；密码只在进程内用于恒定时间校验，不写入数据库。

## 独立 Agent App 模块

仓库中的 [`agentapp`](agentapp/README.md) 是一个不依赖 `internal/control` 的独立 Go 模块，提供 Agent Phone、双前端协议、REF 动作，以及可替换数据源的 Board/Relay App。可以单独运行协议演示：

```bash
go run ./cmd/agentapp
```

打开 `http://localhost:8090`，左侧为人类 Web 前端，右侧为 AI 实际读取的文本前端。

## 模块化 Go Agent Runtime

Aegis 通过独立 Go module `github.com/z3r2ne/agentcore` 使用 Agent loop；本仓库只保留 `agenthost`、`capability`、`coordination`、`storage`、`skill`、`mcp`、`web`、`provider/openai` 与业务适配器。完整边界见 [`docs/architecture/agent-runtime-modules.md`](docs/architecture/agent-runtime-modules.md)。

当前固定依赖 `agentcore v0.2.2`，不再使用本地 `replace`。如果仓库保持私有，新的开发机或 CI 需要配置私有 module 与 GitHub SSH：

```bash
go env -w GOPRIVATE=github.com/z3r2ne/*
git config --global url."ssh://git@ssh.github.com:443/".insteadOf https://github.com/
```

- Go AgentHost 与 SQLite Coordination Runtime 始终启用，不存在旧执行链或切流 feature flag；
- `AEGIS_COORDINATION_MODE=board_autonomy`：唯一协作模式；根 Agent 通过 Board 指派子 Issue，同时继续自己的工作；
- `AEGIS_AGENTAPP_URL` 和 `AEGIS_AGENTAPP_TOKEN`：可把进程内 Board/Relay Agent Phone 替换为远程 Phone transport；
- Coordination 同时持有模式决策与可靠执行。两者在代码中分层，但不会产生两套任务状态所有者。

`GET /api/coordination/modes` 只返回 `board_autonomy`。Task binding、事件 inbox、effect outbox、重试、租约和定时唤醒均保存在 Aegis SQLite；Agent 使用 Phone Board 快捷指令 `phone_board_delegate` 创建并指派子 Issue，使用 `phone_board_sleep` 主动休眠。每分钟心跳会提供耗时、子 Issue 变化、进度与当前活动，评论或 Relay 消息会直接 steer 正在运行的 loop，或唤醒休眠实例。

Coordination 现在也是统一能力控制平面：`capabilityPolicy` 按 Task 控制 Agent 可获得的 Skill、MCP、Phone、Web 和工具。Board/Relay 作为 Phone App 暴露，delegation 可只开放 `aegis.board`。`GET /api/coordination/capabilities` 查看已注册插件，`POST /api/coordination/capabilities/plan` 在执行前预览最终计划，`POST /api/coordination/invoke` 可由操作者主动调用指定 Agent。默认只允许 Agent 自身能力；新增插件必须显式列入 binding 的 `allowed`。详细配置见 [`coordination/README.md`](coordination/README.md)。

## 可观测性与任务证据导出

Aegis 启动时会启用统一的结构化日志、关联 ID、轻量 trace 和进程内指标。HTTP、Coordination、AgentHost、模型流、工具调用与 Web Search 使用同一套 `taskId`、`issueId`、`coordinationId`、`executionId`、`traceId` 关联字段。日志同时输出 JSON 到标准输出，并持久化到 `AEGIS_DATA_DIR/aegis.db` 的 `observability_logs` 表；常见 token、Authorization、API Key 以及当前和历史配置密钥会在写入前脱敏。

- `GET /api/observability/metrics`：读取计数器、耗时分布和 Gauge 快照；
- `GET /api/observability/logs?taskId=...&coordinationId=...&traceId=...&limit=500`：按关联字段查询持久化日志；
- `GET /api/tasks/:id/export`：按 Task ID 或其任意 Issue ID 下载 `aegis.task-evidence/v1` ZIP；默认包含附件并脱敏 JSON；
- `includeArtifacts=false`：只导出元数据，不复制附件；`redactSecrets=false`：显式关闭 JSON 脱敏。

证据包包含 manifest 与逐文件 SHA-256、任务/Issue 树、所有 Execution 和完整对话、工具输入输出事件、进度/评论/验收/审批、Relay、Coordination、Agent Phone 审计、Agent/Skill/知识库快照、相关结构化日志、附件以及面向后续评价的 `evaluation_context.json`。原始附件不会改写，可能包含业务秘密；响应头和 manifest 会明确标识脱敏、完整性和活动中快照状态。详细格式见 [`docs/architecture/observability.md`](docs/architecture/observability.md)。

## Docker Worker 镜像

项目根目录的 `Dockerfile` 用于构建 AgentCore 的固定工具沙箱镜像，镜像内包含 Node.js、Python、
Go、Java、常用编译工具，以及 `agent-browser` 浏览器自动化
工具，镜像名统一为 `aegis-worker:latest`。可以在“容器管理”
页面点击“构建 Worker 镜像”，也可以手动执行：

```bash
docker build -t aegis-worker:latest .
```

也可以使用项目提供的构建脚本：

```bash
./build-worker-image.sh
```

脚本会检查 Docker、构建固定镜像，并启动一次临时容器验证 Node.js、Python、Go、Java 和 `agent-browser`。额外参数会原样传递给
`docker build`，例如 `./build-worker-image.sh --no-cache`。

也可以通过脚本参数覆盖镜像名、工具版本和目标平台：

```bash
./build-worker-image.sh \
  --image aegis-worker:dev \
  --platform linux/arm64 \
  --pull
```

执行 `./build-worker-image.sh --help` 可以查看全部参数；CI 中不需要启动临时验证容器时，可增加 `--no-verify`。也可以执行 `make worker-image` 使用默认配置构建。

Worker 镜像基于 Kali Linux Rolling，并从 Kali 官方镜像构建。需要使用镜像代理或固定快照时，可以覆盖基础镜像：

```bash
AEGIS_KALI_BASE_IMAGE=kalilinux/kali-rolling:latest \
./build-worker-image.sh
```

Node.js、Python、Go、Java 以及常用的基础安全工具均通过 Kali 软件源安装；
`agent-browser` 通过 npm 安装，并默认使用 Kali 软件源提供的 Chromium；AMD64 构建还会执行其浏览器安装步骤，ARM64 则直接使用系统 Chromium，以兼容 Apple Silicon。

每个 Task 在创建时自动绑定一个专属容器和 Docker named volume，卷固定挂载为 `/workspace`。同一 Task 的根 Agent、所有子 Agent、重试和唤醒 Execution 始终复用这一个容器工作区；不同 Task 使用不同容器和卷。容器环境不再接受宿主机工作目录映射。

当根 Issue 进入 `done`、`failed`、`budget_exceeded`、`cancelled` 或 `in_review`，且整棵 Issue 树已结束、没有活跃 Execution 时，Aegis 会自动停止该 Task 的容器。自动回收只停止 runtime，不删除容器记录或 named volume；继续任务时会原地启动容器，`/workspace` 数据保持不变。

Go AgentCore loop 属于控制平面；`bash/read/write/edit/grep/find/ls` 编码能力全部通过 `docker exec` 在对应 Task 容器内执行，且没有宿主机回退。输入附件直接写入任务卷，Agent 生成的文件默认只存在于卷内。只有显式调用 `aegis_publish_attachment` 的文件才会流式进入 Aegis 附件存储，并显示在 Issue 评论/任务证据包中。任务详情页不会浏览、复制或映射容器卷内容。

## Agent、Session 与协作

Agent 是可复用类型，TaskAgent 是任务内实例，Issue 是工作对象，Execution 是一次执行尝试，Session 是该 TaskAgent 在对应 Issue 上的持续会话：

- `/agents` 编辑 Agent 类型的模型覆盖、系统提示词、Skills、知识库与权限边界。
- `/capabilities` 查看能力注册、Agent 默认值、Task policy 和最终 Execution bundle。
- `/skills` 管理 `SKILL.md` 能力包以及它们与 Agent 的引用关系。
- `/sessions` 查看 Execution、模型快照、阶段进度、完整消息、事件和 token/cost 统计。

全局设置只提供默认模型；Agent 类型的模型字段留空时继承全局 Provider、Model、Base URL 和 Thinking。发布根任务或分派子 Issue 时，系统从任务内名字池领取一个 `TaskAgent` 身份，并为它创建独立 Session 与 Agent Phone。Phone 的页面栈、草稿、Relay 收件箱和审计记录写入 SQLite，所有读写同时校验 `taskId + taskAgentId`，任务结束后不会成为另一个任务的上下文。

### Board Autonomy

所有工作都由 Board Issue 表示。根 TaskAgent 可使用 Phone Board 快捷指令 `phone_board_delegate` 创建并指派子 Issue；每次指派都会创建新的 TaskAgent，即使同一种 Agent 类型在同一 Task 中并行出现多次，也不会共享身份、会话或 Phone。父 TaskAgent 发出委派后继续自己的工作，不会被隐式挂起。

没有高价值动作时，TaskAgent 可以使用 `phone_board_sleep` 保留同一 Session 并进入睡眠。每分钟持久化心跳会提供耗时、直属子 Issue 状态、进度摘要和当前活动；Board 评论与 Relay/Phone 消息通过同一个 Coordination outbox 精确 steer 运行中的 TaskAgent，或提前唤醒睡眠实例。旧睡眠定时器带代次令牌，不会误唤醒后续的新睡眠周期。

消息会标注发送方，属于软提示：TaskAgent 可以回复、纠偏、停止或重新指派子 Issue，也可以先继续当前工作。已经终止的 Issue 不会因为迟到评论被重新唤醒。

### Agent Phone

Phone 是 `agentapp` 模块提供的任务内软件运行环境。Coordination 在执行入队前以 `taskId + taskAgentId` 幂等创建或恢复 Phone，AgentHost 再把它物化成 `phone_view`、`phone_action`、`phone_back`、`phone_home`，以及由已安装 App 动态发现的高频快捷指令。快捷指令最多 20 个，按频率排序，仍然经过 Phone Session、权限、幂等、页面状态持久化和审计，不允许绕过 Phone 直连 Board/Relay 后端。

当前内置快捷指令共 9 个：`phone_board_list_issues`、`phone_board_get_issue`、`phone_board_comment_issue`、`phone_board_delegate`、`phone_board_continue_issue`、`phone_board_sleep`、`phone_relay_list_threads`、`phone_relay_get_thread`、`phone_relay_send_message`。Board 采用类似 Linear 的紧凑 Issue 列表、注意事项优先、集中详情和动作菜单，但保留 Aegis 的父子 Issue、Execution 预算与任务隔离语义。

SQLite 表 `agent_app_phone_sessions` 保存当前 App、页面栈、草稿和幂等动作/快捷指令结果，`agent_app_audit_events` 保存完整操作审计。任务详情页按 Task 展示编队和所有独立 Phone；任务证据导出同时包含 Phone 状态与审计记录。

UI 与领域边界、页面到后端职责的映射见 [`docs/architecture/ui-and-domain-boundaries.md`](docs/architecture/ui-and-domain-boundaries.md)。

可用环境变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `PORT` | `8080` | Gin 服务端口 |
| `AEGIS_PASSWORD` | 无 | Web 访问密码；必须设置，也可用 `--password` 指定 |
| `AEGIS_DATA_DIR` | `data` | SQLite 与运行时扩展目录 |
| `AEGIS_DIST` | `dist` | 前端静态文件目录 |
| `AEGIS_LOG_LEVEL` | `info` | JSON/SQLite 日志级别：debug、info、warn、error |

也可以在初始化页选择环境变量认证。AgentCore Provider 会读取对应的标准环境变量，例如 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 或 `OPENCODE_API_KEY`。

## 验证

```bash
make test
make build
```

## 结构

```text
../agentcore/               独立仓库：Agent loop、Tool/Skill/Interceptor 与 Session
agenthost/                  ExecutionSpec 和 capability 物化宿主
capability/                 能力注册、引用、Source 与快照
coordination/               唯一协同决策和可靠执行控制面
agentapp/                   TaskAgent Phone、Board/Relay App 与 SQLite 状态
skill/ mcp/ web/ provider/  可插拔能力模块
storage/ observability/     事件、Artifact、结构化日志、trace 与指标
internal/control/           Aegis Task/Issue 领域与各模块适配器
cmd/server/                 组合根、Gin API、SSE 与静态文件托管
src/                        React 管理端
```

## 安全说明

Aegis 的 AgentCore loop 位于服务控制面，文件、Shell 与浏览器工具只通过任务专属 Docker 容器执行，不回退到宿主机工作区。每个 Agent 的网络、写入与审批权限由 capability policy 检查；请继续限制容器网络、资源与凭据，并只处理明确授权的数据。
