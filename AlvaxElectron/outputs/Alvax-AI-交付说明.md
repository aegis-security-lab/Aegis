# Alvax AI 调研脚手架交付说明

已在当前工作区完成可运行的 Electron 项目基座，项目名为 **Alvax AI**。

核心交付：

- Electron Forge + Vite + React + TypeScript 工程；
- 安全的 main / preload / renderer 隔离与类型化 IPC；
- Agent、Run、Settings 契约及本地原子持久化；
- 可替换 `AgentRuntime` 和可运行的 Mock 多 Agent 编排链路；
- 调研工作台、Agent 目录、运行记录、架构页和设置页；
- TypeScript、ESLint、Vitest、ASAR 和 macOS arm64 package 验证；
- 架构设计、API 约定和后续开发路线文档。

本机验证结果：

- `npm run check`：通过；
- 3 个测试文件、4 个测试：通过（最终结果以项目终检输出为准）；
- Electron Forge macOS arm64 package：通过；
- ASAR 内主进程、preload 和 renderer 静态资源：已核验；
- 打包应用：已启动并成功加载 `alvax://app/main_window/index.html`。

团队开发应使用 Node 24。当前 Node 26 与 `@electron/packager` 18.4.4 存在实际打包兼容问题，项目已通过 `.nvmrc` 和 `engines` 固定运行时范围。
