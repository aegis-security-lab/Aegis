# Execution 与 Task 预算

Aegis 使用两层互不替代的预算：

## 子 Issue 单次 Execution 预算

每个子 Issue 在创建一条 `Execution` 时快照当时的预算配置。默认值为：

- 正常工作最多 100 个模型轮次；
- 正常工作最多 20 分钟；
- 超限后总结最多 10 个模型轮次；
- 总结阶段最多 3 分钟。

轮数按模型调用计数，模型内部的 transport retry 不重复消耗业务轮次。预算在 AgentCore 生命周期拦截器中同步执行，而不是依赖后台轮询。

达到正常工作预算后，Execution 从 `active` 进入 `summarizing`：系统切换总结提示词，收缩模型可见工具，并在工具调用拦截器中阻止委派、写入和正常完成提交。Agent 应只整理已有成果、证据、未完成内容与下一步建议。总结完成或总结预算耗尽后：

- Execution 状态为 `budget_exceeded`；
- Issue 状态和执行阶段为 `budget_exceeded`；
- Agent 的总结、触发原因、轮数和时间快照进入数据库、事件与任务导出；
- Coordination 将该结果作为业务结果通知父 Issue，不作为运行时崩溃重试。

父 Issue 必须选择以下一种处理方式：

1. 调用 `coordinate_continue` 将同一 Issue 重新置为 `todo` 并派发，创建全新的 Coordination Execution 和 Agent Execution；该工具也用于恢复普通 `failed` 子 Issue；
2. 创建另一个 Issue 探索不同方向；
3. 接受当前部分结果并停止。

新 Execution 会重新快照、重新计数单次预算。旧 Execution 的预算和用量不会被修改。

## Task 总时钟墙预算

Task 的 `timeBudgetMinutes` 从根 Issue 的 `createdAt` 开始计算。等待子 Issue、Agent 休眠、评论唤醒以及任何 Execution 重跑都不会重置总时钟墙。达到上限时，控制面结束整个 Task，并中止仍在运行的原生 AgentCore 会话。

因此，“为子 Issue 再开一次 Execution”只能补充该次执行额度，不能绕过整个 Task 的截止时间。
