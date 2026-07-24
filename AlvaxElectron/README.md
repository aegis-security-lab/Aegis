# Alvax AI

缘分公司的 AI 产品开发桌面端。当前版本提供由 Pi Agent 驱动的对话式网站生成、自动验收和本地预览闭环。

## 当前包含

- Electron 43 + Electron Forge 7 + Vite 8 + React 19 + TypeScript 6
- 主进程、preload、renderer、shared contracts 四层隔离
- Zod 校验的类型化 IPC，以及统一的 `ApiResult<T>` 错误模型
- 网站项目、对话、文件交付和验收结果的本地 JSON 原子持久化
- Pi Coding Agent 严格 LF JSONL RPC，支持流式输出、取消和独立项目会话
- 行业、服务、目标用户与网站用途引导，生成 Vite + React + TypeScript + Tailwind 网站
- 自动执行依赖安装、TypeScript 检查、生产构建、预览启动与 HTTP 健康检查
- 现代化聊天工作台，以及预览、文件、验收三栏交付面板
- shadcn/ui Base Nova + Tailwind CSS v4
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
│   ├── window/                # BrowserWindow 安全策略
│   └── website-builder/       # Pi RPC、starter、验收与预览服务
├── preload/                   # 暴露最小 Desktop API
├── renderer/                  # React 调研工作台
└── shared/contracts/          # 跨进程 Schema、DTO、API 类型
```

架构详情见 `docs/architecture.md`，接口与演进规则见 `docs/api-contract.md`，建议路线见 `docs/development-roadmap.md`。对话式网站生成器的专项方案见 `docs/website-builder-development-plan.md`。

## 数据位置

网站项目状态写入 Electron `userData/website-builder-v1.json`，生成源码位于 `userData/website-projects/{projectId}/workspace`。JSON 适合调研阶段；当出现全文检索、事件回放或多人协作需求时再迁移 SQLite。

## 配置 Pi Agent

点击右上角设置，填写 Node.js 可执行文件和 Pi 的 `packages/coding-agent/dist/cli.js` 路径。Provider、Model 可留空以使用 Pi 默认配置。供应商密钥沿用 Pi 支持的环境变量或本地认证；renderer 不接触密钥与子进程。

## 安全参考

实现遵循 Electron 官方的 [Security Checklist](https://www.electronjs.org/docs/latest/tutorial/security)、[Context Isolation](https://www.electronjs.org/docs/latest/tutorial/context-isolation) 和 [IPC 指南](https://www.electronjs.org/docs/latest/tutorial/ipc)。Electron 官方明确建议不要把完整 `ipcRenderer` 暴露给页面，本项目因此为每个功能提供独立、可校验的方法。
