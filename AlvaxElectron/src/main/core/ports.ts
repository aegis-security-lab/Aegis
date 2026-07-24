import type {
  AgentDefinition,
  AppSettings,
  CoordinationMode,
  RunEvent,
  RunRecord,
  SaveAgentInput,
  StartRunInput,
  UpdateSettingsInput,
} from '../../shared/contracts/domain';

export interface WorkspaceRepository {
  listAgents(): Promise<AgentDefinition[]>;
  saveAgent(input: SaveAgentInput): Promise<AgentDefinition>;
  removeAgent(id: string): Promise<void>;
  listRuns(): Promise<RunRecord[]>;
  findRun(id: string): Promise<RunRecord | undefined>;
  saveRun(run: RunRecord): Promise<RunRecord>;
  getSettings(): Promise<AppSettings>;
  updateSettings(input: UpdateSettingsInput): Promise<AppSettings>;
}

export interface Orchestrator {
  start(input: StartRunInput): Promise<RunRecord>;
  cancel(id: string): Promise<RunRecord>;
  shutdown(): void;
}

export type RunEventSink = (event: RunEvent) => void;

export interface AgentExecutionRequest {
  runId: string;
  objective: string;
  mode: CoordinationMode;
  agent: AgentDefinition;
  priorOutputs: readonly AgentExecutionResult[];
}

export interface AgentExecutionResult {
  agentId: string;
  content: string;
  durationMs: number;
}

/** Runs only in the main process. Provider credentials must never cross IPC. */
export interface AgentRuntime {
  execute(
    request: AgentExecutionRequest,
    signal: AbortSignal,
  ): Promise<AgentExecutionResult>;
}
