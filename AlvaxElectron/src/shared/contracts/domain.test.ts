import { describe, expect, it } from 'vitest';
import { SaveAgentInputSchema, StartRunInputSchema } from './domain';

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
});
