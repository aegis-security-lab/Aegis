import { describe, expect, it } from 'vitest';
import { SaveAgentInputSchema, StartRunInputSchema } from './domain';
import { CreateWebsiteProjectInputSchema, SendWebsiteMessageInputSchema } from './website-builder';

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

  it('validates the required website discovery brief', () => {
    expect(CreateWebsiteProjectInputSchema.safeParse({
      mode: 'create',
      name: 'Nova',
      industry: '企业服务',
      offering: 'AI 客户支持平台',
      audience: '中小企业客户成功团队',
      purposes: ['brand', 'conversion'],
      notes: '',
    }).success).toBe(true);
    expect(CreateWebsiteProjectInputSchema.safeParse({
      mode: 'create', name: '', industry: '', offering: '', audience: '', purposes: [], notes: '',
    }).success).toBe(false);
  });

  it('validates a reference website request independently from product fields', () => {
    expect(CreateWebsiteProjectInputSchema.safeParse({
      mode: 'reference', referenceUrl: 'https://www.playbook.com/', referenceRequest: '参考并复刻首页',
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
});
