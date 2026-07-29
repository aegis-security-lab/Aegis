# DESIGN.md — Aegis 视觉系统

> **LOCKED PLANNING ARTIFACT.** 实现应遵循本文件，设计调整记录在 `decision-log.md`。

**Project:** Aegis
**Track:** Product UI redesign
**Date:** 2026-07-29
**Version:** 1.0

## 1. Design Tokens

### 色彩

| Token      | Light     | Dark                    | 用途                   |
| ---------- | --------- | ----------------------- | ---------------------- |
| Canvas     | `#F8FAFC` | `#0B1118`               | 应用背景与侧栏         |
| Surface    | `#FFFFFF` | `#101820`               | 面板                   |
| Foreground | `#172033` | `#E6EDF3`               | 主文本                 |
| Muted text | `#667085` | `#A7B1BE`               | 辅助文本               |
| Border     | `#DDE3EA` | `rgba(255,255,255,.10)` | 弱边界                 |
| Primary    | `#0F6674` | `#66C2CF`               | 主要动作、焦点、运行态 |
| Success    | `#137A52` | `#5CCF97`               | 完成、健康             |
| Warning    | `#8A5700` | `#E6A23C`               | 审批、预算预警         |
| Error      | `#B42318` | `#FF7B72`               | 失败、阻塞、破坏性动作 |
| Info       | `#175CD3` | `#74A7FF`               | 规划、待执行、信息     |

已验证浅色对比：主文本/Canvas `15.55:1`，Muted/Canvas `4.75:1`，白色/Primary `6.62:1`，Error/Canvas `6.28:1`，Warning/Canvas `5.83:1`，均满足 WCAG 2.1 AA 正文标准。状态不能只靠颜色，必须同时显示文字或图标。

### 字体与密度

- Sans：Geist Variable；Mono：系统等宽栈。
- 页面 H1：20px/600；区块 H2：16px/500；正文：14px/400；说明：12px/400；动态数字使用 `tabular-nums`。
- 移动表单字段保持 16px，避免 iOS 自动缩放。
- 4px 基础网格：4 / 8 / 12 / 16 / 20 / 24 / 32。
- 页面间距 20–24px；面板内边距 12–16px；桌面表格行 44–56px。

### 深度、圆角与响应式

- 深度只使用“表面色差 + 低对比 1px 边界”；阴影仅用于浮层，不允许卡片悬浮抬升。
- 控件 4px、面板 6px、Dialog 8px；胶囊仅用于头像、状态点与进度轨道。
- 断点：320 / 768 / 1024 / 1440px。移动 16px 页边距；桌面 24px；最大内容宽度 1600px。
- 移动触控目标 ≥44px；桌面密集控件可为 28–32px，但必须保留清晰焦点环。

## 2. Component Specifications

- **导航**：移动端 off-canvas，桌面 240px 可折叠侧栏。一级分组为工作、团队与能力、运行与安全；设置和主题位于底部。当前项用淡色表面和字重表达。审批显示数量。
- **顶栏**：左侧当前分组/页面；右侧仅保留运行数、待审批数和全局“发布任务”。不放大型管理弹窗。
- **PageHeader**：始终输出一个 H1、说明和当前页面动作；动作按主、次、更多排序。
- **Button**：主要/次要/幽灵/破坏四层；hover/focus/active/disabled/loading 全覆盖；加载时禁用并显示 Spinner。
- **List/Table**：工具栏包含搜索和正交筛选；整行或标题进入详情；复制、重启、删除等低频动作放行尾菜单。
- **Card/Panel**：只用于真正的内容区域，不将每个数字包成独立卡片。Header、Title、Content 语义完整。
- **Status**：文字 + 语义颜色；警告、失败、运行三类优先于装饰色。
- **Loading/Empty/Error**：加载保持布局；空状态解释原因并提供下一步；错误就近显示原因与重试；全局错误使用 `aria-live`。
- **Dialog/Sheet**：只处理短暂决策或补充信息；复杂管理页使用真实路由。必须有 Title、焦点陷阱、Escape 关闭与焦点返回。

## 3. WCAG 2.1 AA Contract

- 正文对比 ≥4.5:1；大字和 UI 图形 ≥3:1。
- 全部操作可键盘访问；焦点环至少 2px；应用提供 skip link。
- 320px 不产生页面级横向滚动；局部数据工具栏允许带清晰边界的横向滚动。
- 移动触控目标 ≥44×44px，目标之间至少 8px。
- 页面恰好一个 H1，后续使用 H2/H3；动态反馈使用 `role=status` 或 `aria-live`。
- `prefers-reduced-motion` 下移除位移、缩放和循环动画，只保留近乎即时的颜色/透明度变化。

## 4. Product Signature

Aegis 的识别特征是“运行态脉冲”：Logo 角标、顶栏运行计数、可钻取状态摘要、Issue/Session 行中的同一套语义状态。它服务于判断系统是否在推进，不是装饰动画。
