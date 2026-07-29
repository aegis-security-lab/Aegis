# UX Decision Log

### 2026-07-29 — 从模块堆叠改为任务导向控制台

**Context:** 原界面隐藏页面标题，以大卡片和巨型管理弹窗承载大量功能，审批、运行状态和员工工作台难以发现。
**Decision:** 采用工作、团队与能力、运行与安全三组主导航；管理页面使用真实路由；概览按人工介入优先；任务使用高密度列表；低频操作进入就近菜单。
**Alternatives considered:** 保留齿轮大弹窗、霓虹安全/HUD、紫粉 AI SaaS、大圆角卡片网格；均因不可深链、移动端风险、低信息密度或与专业定位冲突而放弃。
**Impact:** App shell、PageHeader、Dashboard、Tasks、Issues、Workspace chat、Settings 和全局 tokens。
**Author:** bmad-ux skill
