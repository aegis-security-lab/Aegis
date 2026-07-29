# Aegis Design System — Master

**Direction:** 高信息密度的 Agent 安全运营控制台。克制、精确、可干预；拒绝霓虹 HUD、紫粉 AI 模板、大圆角卡片和装饰性渐变。

## Domain

任务编排、Issue 树、执行会话、审批闸门、运行心跳、安全发现、工作区证据、员工协作。

## Signature

“运行态脉冲”：Logo 健康角标、顶栏运行/审批计数、可钻取状态摘要和行级状态共享同一语义系统。

## System

- Colors: canvas `#F8FAFC`, ink `#172033`, primary `#0F6674`, muted `#667085`, border `#DDE3EA`; dark canvas `#0B1118`.
- Type: Geist Variable; H1 20/600, H2 16/500, body 14/400, caption 12/400, tabular numbers.
- Density: 9/10. 4px grid; panel padding 12–16px; section gap 20–24px; rows 44–56px.
- Radius: control 4px, panel 6px, dialog 8px; pills only for statuses/avatars/progress.
- Depth: borders and quiet surface shifts only. Shadows belong to overlays.
- Motion: 100–200ms, transform/opacity/color only; reduced-motion required.
- Navigation: task-oriented groups, max two levels, real routes, visible approval count.
- Actions: one visible primary action per view; contextual secondary actions nearby; repetitive, destructive, and low-frequency actions in a menu.

Full accessibility and component contract: `bmad-output/DESIGN.md`. Journeys and states: `bmad-output/EXPERIENCE.md`.
