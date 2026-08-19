# 代码模块与体量基线

更新时间：2026-08-19

本文回答“当前有哪些模块、各自代码占比多少”，并作为应用化拆分前后的量化基线。权威目标架构见 [`agent-application-platform.md`](agent-application-platform.md)。

## 统计口径

- 统计生产源码的物理行数：Go、TypeScript、TSX、JavaScript、JSX、CSS 和 SQL；
- 生产占比不包含 `*_test.go`、前端测试、Markdown、生成后的 `dist`、`node_modules`、运行数据和第三方依赖；
- `src` 目前整体视为 Board Web 应用；`cmd/server` 是 Board 与平台的组合根及现有 Board HTTP API；
- 物理行数用于判断迁移成本，不代表设计质量或业务价值；
- 本次迁移会使总行数略增，因为新增了平台合同、兼容层和契约测试。

## 生产代码分布

生产源码合计 **67,418 LOC**。

| 模块 | 职责 | LOC | 占比 | 目标归属 |
| --- | --- | ---: | ---: | --- |
| `src` | Board Web UI、状态与交互 | 28,538 | 42.33% | Board 应用 |
| `apps/board` | Board 领域、验收、协同、Agent/Prompt、Phone | 26,821 | 39.78% | Board 应用 |
| `coordination` | 通用事件决策、outbox、timer、Execution queue/worker | 3,019 | 4.48% | 平台核心 |
| `agentapp` | 通用 Phone Kernel、Session、导航、审计与 transport | 2,367 | 3.51% | 平台核心 |
| `cmd/server` | 当前组合根、Board HTTP API、认证与静态资源 | 2,101 | 3.12% | Board host；通用启动部分后续下沉平台 |
| `platform` | Application、DataSpace、Agent Work、Event Inbox SDK | 1,364 | 2.02% | 平台核心 |
| `observability` | 日志、指标、trace 与持久化查询 | 862 | 1.28% | 平台核心 |
| `agenthost` | AgentCore 物化、模型/能力解析和执行 Session | 856 | 1.27% | 平台核心 |
| `internal/security` | 服务端安全、密钥与脱敏 | 390 | 0.58% | 平台核心 |
| `storage` | 通用 Agent 事件/Session 存储适配 | 340 | 0.50% | 平台核心 |
| `provider` | 模型 Provider 适配 | 318 | 0.47% | 平台核心 |
| `capability` | Capability 描述、注册与权限规划 | 303 | 0.45% | 平台核心 |
| `mcp` | MCP 通用适配 | 88 | 0.13% | 平台核心 |
| `cmd/agentapp` | Phone 独立进程入口 | 32 | 0.05% | 平台 host |
| `internal/webui` | Web 静态资源嵌入 | 19 | 0.03% | Board host |

按最终所有权合并后：

| 所有权组 | LOC | 占比 |
| --- | ---: | ---: |
| Board 应用：`src` + `apps/board` + 当前 `cmd/server` + `internal/webui` | 57,479 | 85.26% |
| 可复用平台核心 | 9,939 | 14.74% |

这说明拆分在设计上可行，但不是“小范围移动目录”：当前产品的大多数代码本来就应该留在 Board 项目，真正需要稳定抽出的框架约占 15%。风险主要来自 2,101 LOC 的组合/API 入口和 Board runtime 中仍需继续收窄的执行适配，而不是 Agent loop 本身。

## Board 内部体量

| Board 子模块 | 生产 LOC | 说明 |
| --- | ---: | --- |
| `apps/board/control` | 25,263 | Task/Issue、验收返工、持久化、Board Agent/Prompt、执行适配、报告与 API 服务层 |
| `apps/board/phone` | 980 | Board 与 Relay Phone 页面、快捷指令、领域端口 |
| `apps/board/coordination` | 531 | `board_autonomy`、Board 事件/Effect/命令合同 |
| Board manifest/identity | 47 | 应用、DataSpace、Agent、Prompt、Controller 显式声明 |

`apps/board/control` 是下一阶段内部模块化的主要对象。它已经从平台核心迁出，因此不会阻止平台独立；后续可在 Board 项目内部再按 `domain / application / agents / prompts / adapters` 拆细，而不应为了目录美观一次性重写所有业务模型。

## 测试体量

测试不计入上面的生产占比，但它决定拆分能否安全完成：

| 模块 | 测试 LOC | 测试文件 |
| --- | ---: | ---: |
| `apps/board` | 10,901 | 53 |
| `coordination` | 935 | 7 |
| `agentapp` | 891 | 2 |
| `cmd/server` | 863 | 8 |
| `platform` | 507 | 7 |

Board 已有较大的行为测试资产，应作为新 Board 项目的 characterization suite 原样迁移。当前两个 Issue 删除测试依赖本机 Docker daemon；无 Docker 时会明确失败，不能误报为代码回归。

## 与迁移前结构的变化

- 原 `internal/control` 的 106 个 Go 文件与 `aegis-guard.ts` 已迁到 `apps/board/control`；
- 原 `coordination/modes/board_autonomy.go` 及 Board 命令合同已迁到 `apps/board/coordination`；
- 原 `agentapp` 内的 Board、Relay 示例实现已迁到 `apps/board/phone`；
- 新增 `platform/application`、`platform/dataspace`、`platform/work` 和持久化 Inbox；
- 平台核心不再默认注册 Board；宿主必须显式注册 `aegis.board`。

因此，目录占比变化主要是**所有权显性化**，不是删掉业务能力。SQLite 表名、Board API 行为和前端路由在当前迁移阶段保持兼容。
