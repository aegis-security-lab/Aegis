# Alvax AI

缘分公司的多 Agent 协同产品研究基座。当前版本聚焦三件事：稳定的 Electron 安全边界、可演进的接口契约、可运行的协同主链路。

## 当前包含

- Electron 43 + Electron Forge 7 + Vite 8 + React 19 + TypeScript 6
- 主进程、preload、renderer、shared contracts 四层隔离
- Zod 校验的类型化 IPC，以及统一的 `ApiResult<T>` 错误模型
- Agent 注册表、运行记录、设置的本地 JSON 原子持久化
- 可替换的 `AgentRuntime` 端口和 `MockAgentRuntime`
- 支持启动、实时事件、取消与结果持久化的研究型编排器
- 调研工作台、Agent 目录、运行记录、架构说明和设置页面
- 严格 CSP、自定义 `alvax://` 协议、沙箱、导航限制、权限拒绝和 Electron Fuses
- TypeScript、ESLint、Vitest 和 Forge package 验证链路

## 快速开始

### 环境

- Node.js 24（见 `.nvmrc`）
- npm 11

```bash
nvm use
npm install
npm run dev
```

Node 24 是有意固定的团队基线。当前 `@electron/packager` 18.4.4 在 Node 26 下可能在解压 Electron 后提前退出，导致命令返回成功但不生成 `out/`；Node 24 已完成真实打包验证。

### 质量检查

```bash
npm run check
npm run build
```

### 制作安装包

```bash
npm run make
```

`make` 只能在对应操作系统上稳定制作原生安装包。正式发布前仍需配置 Apple Developer ID / Windows code signing、notarization、图标和更新源。

## 项目结构

```text
src/
├── main.ts                    # Forge 主进程唯一入口
├── preload.ts                 # Forge preload 唯一入口
├── main/
│   ├── core/                  # 编排用例、端口、应用错误
│   ├── infrastructure/        # JSON 存储、Mock Runtime
│   ├── ipc/                   # IPC 注册、验证、错误序列化
│   ├── protocol/              # alvax:// 安全本地协议
│   └── window/                # BrowserWindow 安全策略
├── preload/                   # 暴露最小 Desktop API
├── renderer/                  # React 调研工作台
└── shared/contracts/          # 跨进程 Schema、DTO、API 类型
```

架构详情见 `docs/architecture.md`，接口与演进规则见 `docs/api-contract.md`，建议路线见 `docs/development-roadmap.md`。

## 数据位置

开发与打包应用都把状态写入 Electron 的 `userData/workspace-v1.json`。首次启动会写入三个演示 Agent。JSON 适合调研阶段；当出现并发写入、全文检索、事件回放或数据量需求时，再实现 `WorkspaceRepository` 的 SQLite adapter。

## 接入真实模型的最短路径

1. 在主进程实现新的 `AgentRuntime`，不要在 renderer 直接调用模型供应商。
2. 实现系统钥匙串 `CredentialVault`，配置中只保存 `credentialKey` 引用。
3. 在主进程添加 runtime registry，根据 Agent 的 `runtime.kind` 选择 adapter。
4. 保持 `AlvaxDesktopApi` 不变；若必须破坏性修改，新增版本化方法或迁移层。

## 安全参考

实现遵循 Electron 官方的 [Security Checklist](https://www.electronjs.org/docs/latest/tutorial/security)、[Context Isolation](https://www.electronjs.org/docs/latest/tutorial/context-isolation) 和 [IPC 指南](https://www.electronjs.org/docs/latest/tutorial/ipc)。Electron 官方明确建议不要把完整 `ipcRenderer` 暴露给页面，本项目因此为每个功能提供独立、可校验的方法。
