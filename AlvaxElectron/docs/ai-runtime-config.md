# AI 运行配置

Alvax Studio 使用以下本机私有配置作为 Pi Agent 和生成式 AI 应用的统一运行配置：

```text
~/Library/Application Support/Alvax Studio/config/ai-runtime.json
```

完整路径通常为：

```text
/Users/<用户名>/Library/Application Support/Alvax Studio/config/ai-runtime.json
```

配置格式：

```json
{
  "version": 1,
  "provider": "opencode-go",
  "model": "deepseek-v4-flash",
  "baseUrl": "https://opencode.ai/zen/go/v1",
  "apiKey": "填写 API Key",
  "providerApiKeyEnv": "OPENCODE_API_KEY"
}
```

应用会把文件权限设置为 `0600`。每次创建新的 Pi RPC 进程时都会重新读取此文件，并注入以下环境变量：

- `ALVAX_AI_API_KEY`
- `ALVAX_AI_BASE_URL`
- `ALVAX_AI_MODEL`
- `ALVAX_AI_PROVIDER`
- `ALVAX_AI_CONFIG_PATH`
- `providerApiKeyEnv` 指定的 Provider 认证变量

Agent 可以使用变量名称、模型和接口地址开发带服务端的 AI 应用，但不得读取或输出 `apiKey`。密钥只能由服务端进程读取，不得使用 `VITE_*` 暴露给浏览器，也不得写入生成的网站源码、日志或构建产物。
