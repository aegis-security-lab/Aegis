import { describe, expect, it } from 'vitest';
import { SaveAgentInputSchema, StartRunInputSchema } from './domain';
import { CreateWebsiteProjectInputSchema, isInitialWebsiteBriefMessage, SendWebsiteMessageInputSchema, WEBSITE_BRIEF_MESSAGE_PREFIX } from './website-builder';

describe('shared contracts', () => {
  it('accepts a credential reference without accepting a raw secret field', () => {
    const result = SaveAgentInputSchema.safeParse({
      name: 'Researcher',
      role: 'Research',
      description: 'Finds evidence',
      capabilities: ['web-search'],
      instructions: 'Cite sources.',
      runtime: {
        kind: 'openai-compatible',
        model: 'example-model',
        baseUrl: 'https://api.example.com/v1',
        credentialKey: 'provider.primary',
        apiKey: 'must-not-cross-ipc',
      },
      status: 'active',
    });

    expect(result.success).toBe(true);
    if (result.success) {
      expect('apiKey' in result.data.runtime).toBe(false);
    }
  });

  it('rejects an orchestration request without participating agents', () => {
    const result = StartRunInputSchema.safeParse({
      objective: 'Explore a product direction',
      agentIds: [],
      mode: 'supervisor',
    });

    expect(result.success).toBe(false);
  });

  it('validates an existing website upgrade request', () => {
    expect(CreateWebsiteProjectInputSchema.safeParse({
      mode: 'reference', referenceUrl: 'https://www.playbook.com/', referenceRequest: '升级首页结构和转化表达',
    }).success).toBe(true);
    expect(CreateWebsiteProjectInputSchema.safeParse({
      mode: 'reference', referenceUrl: 'playbook.com', referenceRequest: '',
    }).success).toBe(false);
  });

  it('rejects blank website chat messages', () => {
    expect(SendWebsiteMessageInputSchema.safeParse({
      projectId: crypto.randomUUID(), message: '   ',
    }).success).toBe(false);
  });

  it('runs the structured upgrade workflow only for the initial brief', () => {
    expect(isInitialWebsiteBriefMessage(`${WEBSITE_BRIEF_MESSAGE_PREFIX}\n现有网站：https://example.com`)).toBe(true);
    expect(isInitialWebsiteBriefMessage('首页按钮间距有问题，请直接修复')).toBe(false);
  });
});
