import { randomUUID } from 'node:crypto';
import { mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import path from 'node:path';
import type { WorkspaceRepository } from '../core/ports';
import { AppError } from '../core/errors';
import {
  AgentDefinitionSchema,
  AppSettingsSchema,
  type AgentDefinition,
  type AppSettings,
  type RunRecord,
  type SaveAgentInput,
  type UpdateSettingsInput,
} from '../../shared/contracts/domain';
import {
  WorkspaceStateSchema,
  type WorkspaceState,
} from '../../shared/contracts/state';

function createInitialState(): WorkspaceState {
  const now = new Date().toISOString();
  const shared = {
    description: '',
    runtime: { kind: 'mock' as const, model: 'research-simulator' },
    status: 'active' as const,
    createdAt: now,
    updatedAt: now,
  };

  return {
    schemaVersion: 1,
    agents: [
      {
        ...shared,
        id: 'agent-researcher',
        name: '洞察研究员',
        role: 'Researcher',
        description: '搜集信号、拆解问题并给出带依据的判断。',
        capabilities: ['资料研究', '竞品分析', '事实核查'],
        instructions: '先明确问题，再给出证据、判断和不确定性。',
      },
      {
        ...shared,
        id: 'agent-strategist',
        name: '产品策略师',
        role: 'Strategist',
        description: '把研究结论转化为用户价值、假设和优先级。',
        capabilities: ['需求分析', '机会评估', '路线规划'],
        instructions: '区分事实与假设，优先给出可验证的产品决策。',
      },
      {
        ...shared,
        id: 'agent-critic',
        name: '反方评审员',
        role: 'Critic',
        description: '挑战方案中的盲点、风险与不必要复杂度。',
        capabilities: ['风险审查', '反例推演', '决策复盘'],
        instructions: '寻找最可能让方案失败的三个原因，并提出验证方式。',
      },
    ],
    runs: [],
    settings: {
      theme: 'system',
      language: 'zh-CN',
      telemetryEnabled: false,
    },
  };
}

export class JsonWorkspaceRepository implements WorkspaceRepository {
  private state: WorkspaceState | undefined;
  private readonly initialized: Promise<void>;
  private mutationQueue: Promise<void> = Promise.resolve();

  constructor(private readonly statePath: string) {
    this.initialized = this.load();
  }

  async listAgents(): Promise<AgentDefinition[]> {
    const state = await this.readState();
    return structuredClone(state.agents).sort((a, b) =>
      a.createdAt.localeCompare(b.createdAt),
    );
  }

  async saveAgent(input: SaveAgentInput): Promise<AgentDefinition> {
    return this.mutate((state) => {
      const now = new Date().toISOString();
      const existing = input.id
        ? state.agents.find((agent) => agent.id === input.id)
        : undefined;
      if (input.id && !existing) {
        throw new AppError('NOT_FOUND', '没有找到要更新的 Agent。');
      }

      const agent = AgentDefinitionSchema.parse({
        ...input,
        id: existing?.id ?? randomUUID(),
        createdAt: existing?.createdAt ?? now,
        updatedAt: now,
      });
      const index = state.agents.findIndex((item) => item.id === agent.id);
      if (index >= 0) state.agents[index] = agent;
      else state.agents.push(agent);
      return structuredClone(agent);
    });
  }

  async removeAgent(id: string): Promise<void> {
    await this.mutate((state) => {
      const index = state.agents.findIndex((agent) => agent.id === id);
      if (index < 0) throw new AppError('NOT_FOUND', 'Agent 不存在。');
      if (state.runs.some((run) => run.status === 'running' && run.agentIds.includes(id))) {
        throw new AppError('CONFLICT', 'Agent 正在参与任务，暂时不能删除。');
      }
      state.agents.splice(index, 1);
    });
  }

  async listRuns(): Promise<RunRecord[]> {
    const state = await this.readState();
    return structuredClone(state.runs).sort((a, b) =>
      b.createdAt.localeCompare(a.createdAt),
    );
  }

  async findRun(id: string): Promise<RunRecord | undefined> {
    const state = await this.readState();
    const run = state.runs.find((item) => item.id === id);
    return run ? structuredClone(run) : undefined;
  }

  async saveRun(run: RunRecord): Promise<RunRecord> {
    return this.mutate((state) => {
      const index = state.runs.findIndex((item) => item.id === run.id);
      if (index >= 0) state.runs[index] = structuredClone(run);
      else state.runs.unshift(structuredClone(run));
      state.runs = state.runs.slice(0, 100);
      return structuredClone(run);
    });
  }

  async getSettings(): Promise<AppSettings> {
    const state = await this.readState();
    return structuredClone(state.settings);
  }

  async updateSettings(input: UpdateSettingsInput): Promise<AppSettings> {
    return this.mutate((state) => {
      state.settings = AppSettingsSchema.parse({ ...state.settings, ...input });
      return structuredClone(state.settings);
    });
  }

  private async load(): Promise<void> {
    try {
      const raw = await readFile(this.statePath, 'utf8');
      this.state = WorkspaceStateSchema.parse(JSON.parse(raw));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
      this.state = createInitialState();
      await this.persist();
    }
  }

  private async readState(): Promise<WorkspaceState> {
    await this.initialized;
    await this.mutationQueue;
    return this.requireState();
  }

  private async mutate<T>(operation: (state: WorkspaceState) => T): Promise<T> {
    await this.initialized;
    const mutation = this.mutationQueue.then(async () => {
      const result = operation(this.requireState());
      await this.persist();
      return result;
    });
    this.mutationQueue = mutation.then(
      () => undefined,
      () => undefined,
    );
    return mutation;
  }

  private requireState(): WorkspaceState {
    if (!this.state) throw new AppError('INTERNAL_ERROR', 'Workspace 尚未初始化。');
    return this.state;
  }

  private async persist(): Promise<void> {
    const state = this.requireState();
    await mkdir(path.dirname(this.statePath), { recursive: true });
    const temporaryPath = `${this.statePath}.${process.pid}.tmp`;
    await writeFile(temporaryPath, `${JSON.stringify(state, null, 2)}\n`, 'utf8');
    await rename(temporaryPath, this.statePath);
  }
}
