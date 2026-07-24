import type {
  AgentExecutionRequest,
  AgentExecutionResult,
  AgentRuntime,
} from '../core/ports';

export class MockAgentRuntime implements AgentRuntime {
  constructor(private readonly latencyMs = 650) {}

  async execute(
    request: AgentExecutionRequest,
    signal: AbortSignal,
  ): Promise<AgentExecutionResult> {
    const startedAt = performance.now();
    await abortableDelay(this.latencyMs, signal);
    return {
      agentId: request.agent.id,
      content: `${request.agent.name} 已针对「${request.objective}」提交模拟阶段结果。`,
      durationMs: Math.round(performance.now() - startedAt),
    };
  }
}

function abortableDelay(durationMs: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, durationMs);
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        reject(new DOMException('Aborted', 'AbortError'));
      },
      { once: true },
    );
  });
}
