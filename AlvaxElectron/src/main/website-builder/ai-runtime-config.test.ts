import { mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { AiRuntimeConfigManager } from './ai-runtime-config';

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) => rm(directory, { recursive: true, force: true })));
});

describe('AiRuntimeConfigManager', () => {
  it('creates a private config and exposes secrets only through environment variables', async () => {
    const directory = await mkdtemp(path.join(os.tmpdir(), 'alvax-ai-config-'));
    temporaryDirectories.push(directory);
    const filePath = path.join(directory, 'config', 'ai-runtime.json');
    const manager = new AiRuntimeConfigManager(filePath);
    const config = {
      version: 1 as const,
      provider: 'opencode-go',
      model: 'test-model',
      baseUrl: 'https://example.com/v1',
      apiKey: 'sk-test-private-value',
      providerApiKeyEnv: 'OPENCODE_API_KEY',
    };

    await manager.ensure(config);
    const loaded = await manager.load();
    const environment = manager.environment(loaded);
    const context = manager.agentContext(loaded);

    expect((await stat(filePath)).mode & 0o777).toBe(0o600);
    expect(JSON.parse(await readFile(filePath, 'utf8'))).toEqual(config);
    expect(environment.ALVAX_AI_API_KEY).toBe(config.apiKey);
    expect(environment.OPENCODE_API_KEY).toBe(config.apiKey);
    expect(context).toContain(config.baseUrl);
    expect(context).not.toContain(config.apiKey);
  });
});
