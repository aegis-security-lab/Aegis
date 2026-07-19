# Aegis

Aegis 是一个由真实 Pi Agent 驱动的本地任务控制台。它把可复用 Agent 定义、任务发布、自动拆解、并行执行、Issues、会话调试、审批与上下文对话放进一个可观察、可干预的后台，不包含模拟执行器。

## 已实现

- 四步首次初始化：产品说明、Pi Runtime 探测、模型与认证、工作区与安全策略
- 多页面后台：总览、任务、Issues、Agents、Skills、Sessions、审批中心与设置
- Gin HTTP API 与 SSE 实时状态流
- SQLite + GORM 标准化持久化 Project、Issue、IssueRelation、Execution、消息、评论、审批与 Agent Wakeup
- Pi JSONL RPC Runtime，支持真实 session、流式消息、工具调用、阶段进度和 session 统计
- Agent 是独立的模型覆盖、系统提示词、工具集、Skills 和权限边界组合；模型留空时继承全局配置
- 首次启动自动创建 Orchestrator、后端工程师、前端工程师和红队攻防工程师，并为三个专业 Agent 分配不同 Skills
- Skill 管理支持标准 `SKILL.md` 的新增、编辑、ZIP/Markdown 导入、ZIP 导出与本地路径安装
- Task 是顶层 Issue；Orchestrator 将其拆成子 Issues，父子层级与 `blocks` 依赖图相互独立
- 任意工作 Agent 都能通过真实 Pi 扩展工具递归拆分 2–8 个子 Issues；支持最多 4 层、每层依赖与幂等重试
- 父 Issue 拆分后进入 `waiting_children` 并释放 checkout；直属子项全部结束后创建 `continuation` Execution 汇总、验证并决定完成或再次拆分
- 顶层任务支持树级取消：在调度临界区内取消所有未完成后代与活跃/排队 Execution，终止 Pi Sessions，并关闭待审批和 Agent Wakeup；已完成历史保持不变
- Issues 页面提供可折叠层级树、直属子项进度、未完成 blocker、等待子树与汇总中状态
- 原子 checkout 使用条件 SQL 校验状态、Agent 所有权、Execution 锁与未完成 blockers，冲突返回 409
- 一个 Issue 可以保留多次 Execution；每次 Execution 对应独立 Pi Session、消息、事件、统计和审批
- Issue 评论支持稳定 Agent ID mention；调度器把 mention 转成持久化 Wakeup 并启动目标 Agent 的真实 Execution
- Agent 回复可以继续 mention 另一个 Agent，形成真实的链式协作
- 普通 Agent 可以通过任务树级广播同步跨 Issue 的关键发现；广播持久化后会实时投递给同一任务中正在执行的其他 Worker，并可由后续 Agent 查询历史
- guided / autonomous 两种运行模式；工具调用与 Issue 返工使用独立审批类型和全局策略，Agent 可分别覆盖默认值
- Execution 停止；Issue 人工复核；运行中 Agent 对话；Session 消息和事件调试
- React 19、Tailwind CSS v4、shadcn/ui Base Nova；桌面与移动端布局

## 环境要求

- Go 1.25+
- Node.js 22.19+
- 一个已经构建的 Pi Coding Agent 仓库

本机默认可使用：

```text
Node: /Users/patrick/.nvm/versions/node/v26.5.0/bin/node
Pi:   /Users/patrick/Code/pi/packages/coding-agent/dist/cli.js
```

如果 Pi 尚未构建：

```bash
cd /Users/patrick/Code/pi
npm ci --ignore-scripts
npm run build
```

## 启动

```bash
cd /Users/patrick/Code/aegis
npm install
make run
```

打开 `http://localhost:8080`。首次访问会进入初始化向导；模型密钥只会写入本机 `data/aegis.db`，API 与 UI 只返回是否已配置，不返回密钥内容。

前后端开发模式：

```bash
make dev
```

Vite 地址为 `http://localhost:5173`，`/api` 会代理到 `http://localhost:8080`。

## Agent、Session 与协作

Agent 是可复用定义，Issue 是工作对象，Execution 是一次执行尝试，Session 是该 Execution 的 Pi 运行实例：

- `/agents` 编辑每个 Agent 自己的模型覆盖、系统提示词、工具、Skills 与权限边界。
- `/skills` 管理 `SKILL.md` 能力包以及它们与 Agent 的引用关系。
- `/sessions` 查看 Pi session ID、Execution、PID、模型快照、阶段进度、消息、事件和 token/cost 统计。

全局设置只提供默认模型；Agent 的模型字段留空时继承全局 Provider、Model、Base URL 和 Thinking。工具集不会从全局共享，每个 Agent 都保存自己的独立列表；`aegis_create_subissues`、`aegis_report_progress`、`aegis_broadcast`、`aegis_list_broadcasts` 等 Aegis 控制面工具是所有普通 Agent 的固有能力，不能移除。Agent 在完成调查、设计、实现或验证等实质阶段后会被要求调用 `aegis_report_progress`；发现可能影响其他 Issues 的接口、约束、证据、风险或阻塞时则使用任务广播。

### Issue 评论与 Agent-to-Agent 调用

进入任一 Issue 详情页即可查看评论线程、心跳状态并发布评论。通过页面中的 Agent 选择器插入结构化 mention，例如：

```text
[@红队攻防工程师](agent://red-team-engineer) 请独立复核这个结果。
```

调度器会持久化一条 `issue_comment_mentioned` Wakeup，并优先续接该 Issue 下该 Agent 最近的 Pi RPC session：会话仍在线时直接投递，进程已退出时使用原 session ID 恢复；只有该 Agent 从未处理过此 Issue 时才创建新的 mention Execution。目标 Agent 的回复会写回原 Issue 评论线程；回复中包含另一个有效的结构化 mention 时，会继续为下一个 Agent 创建 Wakeup。普通的 `@red-team-engineer` 文本不会触发唤醒，避免误调用。

Mention 不会改变 Issue 所有权，也不会重新打开已经完成的 Issue。这里的 Agent ID 是定义 ID，Execution ID 和 Pi Session ID 只用于运行时诊断。

已完成或待复核的 Issue 收到明确的补做、重新拆解或重新执行要求时，Agent 可以调用 `aegis_request_rework` 发起 `issue_rework` 审批。批准后 Aegis 创建新的工作 Execution、重新取得该 Issue 的 checkout，并将批准的返工要求作为新一轮 Prompt；若该类型配置为自动批准则直接启动。工具调用与 Issue 返工可分别配置全局审批策略，每个 Agent 的对应配置留空时继承全局，非空时覆盖。

### 运行时递归拆解

任意 Agent 判断当前 Issue 过大时，都可以调用 Pi 中真实注册的 `aegis_create_subissues` 工具。该工具不是文本约定或模拟结果：它通过当前 Execution 独有的随机令牌调用 Gin 控制面，并在一个 SQLite 事务里完成：

1. 创建带 `parentId` 和 `requestDepth` 的子 Issues；
2. 创建子项间的 `blocks` 依赖，并让每个子项阻塞父 Issue；
3. 写入带 `requestKey` 的 `IssueDecomposition`，保证工具重试不重复创建；
4. 将父 Issue 切换到 `waiting_children`、释放 checkout，并交还调度器。

调度器只启动未被阻塞的子 Issue，遵循全局并发限制。所有直属子项进入 `done` 或 `cancelled` 后，父 Issue 不会被自动标记完成，而是启动新的 `continuation` Execution，读取子项结果、检查真实工作区并执行父级验收。子 Issue 同样可以继续调用该工具，因此形成受深度和数量边界保护的递归执行树。

可用环境变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `PORT` | `8080` | Gin 服务端口 |
| `AEGIS_DATA_DIR` | `data` | SQLite 与运行时扩展目录 |
| `AEGIS_DIST` | `dist` | 前端静态文件目录 |

也可以在初始化页选择环境变量认证。Pi 内置 Provider 的标准变量仍然有效，例如 `OPENCODE_API_KEY`。

## 验证

```bash
make test
make build
```

## 结构

```text
cmd/server/                 Gin API、SSE、静态文件托管与优雅关闭
internal/control/models.go  Paperclip 风格的领域模型与 API DTO
internal/control/store.go   GORM/SQLite 标准化仓库、关系图与原子 checkout
internal/control/decomposition.go Agent 子 Issue 拆解事务、幂等键与层级边界
internal/control/registry.go Agent/Skill 注册表、默认数据与 Skill 包处理
internal/control/runtime.go Pi RPC、递归调度、continuation、审批、对话与 Agent Wakeup
internal/control/aegis-guard.ts 运行时控制工具、审批、工作区与权限边界守卫
src/pages/                  后台各业务页面和初始化流程
src/components/             应用壳、运行对话和共享组件
src/components/ui/          shadcn/ui Base UI 原语
src/lib/                    API、SSE 和全局状态客户端
```

## 安全说明

Aegis 会让 Pi 在所选工作目录中真实读写文件并执行工具。每个 Agent 的网络、Shell、写入与审批权限会由运行时守卫检查，文件路径也会限制在任务工作区内；这仍然不是操作系统级沙箱。请只选择明确授权的目录，生产部署还应增加进程级隔离、凭据托管和操作审计。
