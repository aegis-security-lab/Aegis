import { randomUUID } from 'node:crypto';
import { AppError } from './errors';
import type {
  AgentExecutionResult,
  AgentRuntime,
  Orchestrator,
  RunEventSink,
  WorkspaceRepository,
} from './ports';
import type {
  AgentDefinition,
  RunEvent,
  RunRecord,
  StartRunInput,
} from '../../shared/contracts/domain';

export class ResearchOrchestrator implements Orchestrator {
  private readonly controllers = new Map<string, AbortController>();

  constructor(
    private readonly repository: WorkspaceRepository,
    private readonly emit: RunEventSink,
    private readonly runtime: AgentRuntime,
  ) {}

  async start(input: StartRunInput): Promise<RunRecord> {
    const allAgents = await this.repository.listAgents();
    const agents = input.agentIds.map((id) => allAgents.find((agent) => agent.id === id));
    if (agents.some((agent) => !agent)) {
      throw new AppError('NOT_FOUND', '参与任务的 Agent 中有一个不存在。');
    }
    if (agents.some((agent) => agent?.status !== 'active')) {
      throw new AppError('CONFLICT', '只有 active 状态的 Agent 可以参与任务。');
    }

    const now = new Date().toISOString();
    const run: RunRecord = {
      id: randomUUID(),
      objective: input.objective,
      agentIds: input.agentIds,
      mode: input.mode,
      status: 'running',
      createdAt: now,
      updatedAt: now,
    };
    await this.repository.saveRun(run);

    const controller = new AbortController();
    this.controllers.set(run.id, controller);
    this.emitEvent(run.id, 'run.started', `任务已启动 · ${input.mode}`);
    void this.execute(run, agents as AgentDefinition[], controller.signal);
    return run;
  }

  async cancel(id: string): Promise<RunRecord> {
    const run = await this.repository.findRun(id);
    if (!run) throw new AppError('NOT_FOUND', '任务不存在。');
    if (run.status !== 'running') {
      throw new AppError('CONFLICT', '只有运行中的任务可以取消。');
    }

    this.controllers.get(id)?.abort();
    const cancelled: RunRecord = {
      ...run,
      status: 'cancelled',
      updatedAt: new Date().toISOString(),
    };
    await this.repository.saveRun(cancelled);
    this.emitEvent(id, 'run.cancelled', '任务已由用户取消');
    this.controllers.delete(id);
    return cancelled;
  }

  shutdown(): void {
    for (const controller of this.controllers.values()) controller.abort();
    this.controllers.clear();
  }

  private async execute(
    run: RunRecord,
    agents: AgentDefinition[],
    signal: AbortSignal,
  ): Promise<void> {
    try {
      const outputs: AgentExecutionResult[] = [];
      for (const agent of agents) {
        this.assertNotAborted(signal);
        this.emitEvent(run.id, 'agent.started', `${agent.name} 正在处理任务`, agent.id);
        const output = await this.runtime.execute(
          {
            runId: run.id,
            objective: run.objective,
            mode: run.mode,
            agent,
            priorOutputs: outputs,
          },
          signal,
        );
        outputs.push(output);
        this.assertNotAborted(signal);
        this.emitEvent(
          run.id,
          'agent.completed',
          `${agent.name} 已提交阶段结果`,
          agent.id,
        );
      }

      const completed: RunRecord = {
        ...run,
        status: 'completed',
        summary: `${agents.length} 个 Agent 已完成协同演示。接入真实 Runtime 后，此处将保存聚合结果。`,
        updatedAt: new Date().toISOString(),
      };
      await this.repository.saveRun(completed);
      this.emitEvent(run.id, 'run.completed', '所有 Agent 已完成，结果已聚合');
    } catch (error) {
      if (signal.aborted) return;
      const failed: RunRecord = {
        ...run,
        status: 'failed',
        summary: error instanceof Error ? error.message : '未知错误',
        updatedAt: new Date().toISOString(),
      };
      await this.repository.saveRun(failed);
      this.emitEvent(run.id, 'run.failed', '任务执行失败');
    } finally {
      this.controllers.delete(run.id);
    }
  }

  private assertNotAborted(signal: AbortSignal): void {
    if (signal.aborted) throw new DOMException('Aborted', 'AbortError');
  }

  private emitEvent(
    runId: string,
    type: RunEvent['type'],
    message: string,
    agentId?: string,
  ): void {
    const event: RunEvent = {
      id: randomUUID(),
      runId,
      type,
      at: new Date().toISOString(),
      message,
      ...(agentId ? { agentId } : {}),
    };
    this.emit(event);
  }
}
