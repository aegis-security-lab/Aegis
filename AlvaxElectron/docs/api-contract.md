# Desktop API Contract v1

唯一公开入口是 renderer 中的 `window.alvax`。所有 Promise 方法返回：

```ts
type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: { code: ApiErrorCode; message: string; details?: Record<string, string[]> } };
```

## 方法

| Namespace | 方法 | 输入 | 输出 |
|---|---|---|---|
| `system` | `getInfo()` | 无 | `SystemInfo` |
| `agents` | `list()` | 无 | `AgentDefinition[]` |
| `agents` | `save(input)` | `SaveAgentInput` | `AgentDefinition` |
| `agents` | `remove(id)` | Agent ID | `{ id }` |
| `runs` | `list()` | 无 | `RunRecord[]` |
| `runs` | `start(input)` | `StartRunInput` | `RunRecord` |
| `runs` | `cancel(id)` | Run ID | `RunRecord` |
| `runs` | `onEvent(listener)` | callback | unsubscribe function |
| `settings` | `get()` | 无 | `AppSettings` |
| `settings` | `update(input)` | partial settings | `AppSettings` |

## 错误码

- `VALIDATION_ERROR`：输入不符合 schema，`details` 按字段返回。
- `NOT_FOUND`：实体不存在。
- `CONFLICT`：当前状态不允许操作。
- `UNAUTHORIZED`：IPC sender 不可信。
- `INTERNAL_ERROR`：内部错误；不向 renderer 泄露堆栈或敏感信息。

## 演进规则

1. shared contract 是跨进程事实来源，先改 schema，再改 handler 和 UI。
2. 新增 optional 字段属于兼容性变化；删除、改名、改变语义属于破坏性变化。
3. 破坏性变化采用新方法或 `v2` channel，并保留一个版本迁移周期。
4. IPC 只传递 Structured Clone 支持的 plain data，不传 Electron 对象、DOM 对象、class instance 或函数（事件 unsubscribe 除外，它只存在于 preload 暴露层）。
5. API key、session cookie 和系统路径不得进入 DTO。

## 新增 API 的检查清单

1. 在 `src/shared/contracts` 添加输入、输出 schema 和类型。
2. 在 `IPC_CHANNELS` 添加命名空间明确的固定 channel。
3. 在 main handler 中校验 sender 和输入。
4. 在 preload 暴露一个具体方法，不暴露原始 `ipcRenderer`。
5. 增加 schema 单测、handler 用例或端到端验证。
6. 更新本文件。
