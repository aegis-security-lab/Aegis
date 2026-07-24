import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { z } from 'zod';

const AiRuntimeConfigSchema = z.object({
  version: z.literal(1),
  provider: z.string().trim().min(1).max(80),
  model: z.string().trim().min(1).max(160),
  baseUrl: z.url(),
  apiKey: z.string().trim().min(1),
  providerApiKeyEnv: z.string().regex(/^[A-Z][A-Z0-9_]*$/).default('OPENCODE_API_KEY'),
});

export type AiRuntimeConfig = z.infer<typeof AiRuntimeConfigSchema>;

export class AiRuntimeConfigManager {
  constructor(readonly filePath: string) {}

  async load(): Promise<AiRuntimeConfig> {
    try {
      return AiRuntimeConfigSchema.parse(JSON.parse(await readFile(this.filePath, 'utf8')));
    } catch (error) {
      throw new Error(`AI 运行配置无效：${this.filePath}\n${toMessage(error)}`, { cause: error });
    }
  }

  async ensure(defaults: AiRuntimeConfig): Promise<void> {
    try {
      await readFile(this.filePath, 'utf8');
      await chmod(this.filePath, 0o600);
      return;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
      await mkdir(path.dirname(this.filePath), { recursive: true });
      await writeFile(this.filePath, `${JSON.stringify(defaults, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
      await chmod(this.filePath, 0o600);
    }
  }

  environment(config: AiRuntimeConfig): NodeJS.ProcessEnv {
    return {
      ...process.env,
      [config.providerApiKeyEnv]: config.apiKey,
      ALVAX_AI_API_KEY: config.apiKey,
      ALVAX_AI_BASE_URL: config.baseUrl,
      ALVAX_AI_MODEL: config.model,
      ALVAX_AI_PROVIDER: config.provider,
      ALVAX_AI_CONFIG_PATH: this.filePath,
    };
  }

  agentContext(config: AiRuntimeConfig): string {
    return `AI 应用运行配置：\n- Provider：${config.provider}\n- Model：${config.model}\n- Base URL：${config.baseUrl}\n- 密钥环境变量：ALVAX_AI_API_KEY\n- 其他环境变量：ALVAX_AI_BASE_URL、ALVAX_AI_MODEL、ALVAX_AI_PROVIDER\n- 私有配置路径：${this.filePath}\n\n如果需要开发 AI 应用，只能在服务端读取这些环境变量。禁止读取、输出或复制私有配置中的 apiKey；禁止将密钥写入 VITE_*、浏览器代码、日志或最终回复。`;
  }
}

export async function readExistingPiApiKey(homePath: string, provider: string): Promise<string> {
  try {
    const auth = JSON.parse(await readFile(path.join(homePath, '.pi', 'agent', 'auth.json'), 'utf8')) as Record<string, { key?: unknown }>;
    const key = auth[provider]?.key;
    return typeof key === 'string' ? key : '';
  } catch {
    return '';
  }
}

function toMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
