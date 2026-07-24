import { describe, expect, it } from 'vitest';
import { MockAgentRuntime } from './mock-agent-runtime';

describe('MockAgentRuntime', () => {
  it('implements the provider-independent runtime contract', async () => {
    const runtime = new MockAgentRuntime(1);
    const result = await runtime.execute(
      {
        runId: 'run-1',
        objective: 'Validate the orchestration boundary',
        mode: 'supervisor',
        priorOutputs: [],
        agent: {
          id: 'agent-1',
          name: 'Researcher',
          role: 'Research',
          description: '',
          capabilities: [],
          instructions: '',
          runtime: { kind: 'mock', model: 'research-simulator' },
          status: 'active',
          createdAt: '2026-07-24T00:00:00.000Z',
          updatedAt: '2026-07-24T00:00:00.000Z',
        },
      },
      new AbortController().signal,
    );

    expect(result.agentId).toBe('agent-1');
    expect(result.content).toContain('Researcher');
  });
});
