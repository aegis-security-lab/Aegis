import type {
  AgentDefinition,
  AppSettings,
  RunEvent,
  RunRecord,
  SaveAgentInput,
  StartRunInput,
  SystemInfo,
  UpdateSettingsInput,
} from './domain';

export const IPC_CHANNELS = {
  systemInfo: 'alvax:system:info',
  agentList: 'alvax:agent:list',
  agentSave: 'alvax:agent:save',
  agentRemove: 'alvax:agent:remove',
  runList: 'alvax:run:list',
  runStart: 'alvax:run:start',
  runCancel: 'alvax:run:cancel',
  runEvent: 'alvax:run:event',
  settingsGet: 'alvax:settings:get',
  settingsUpdate: 'alvax:settings:update',
} as const;

export type ApiErrorCode =
  | 'VALIDATION_ERROR'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'UNAUTHORIZED'
  | 'INTERNAL_ERROR';

export interface ApiError {
  code: ApiErrorCode;
  message: string;
  details?: Record<string, string[]>;
}

export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: ApiError };

export interface AlvaxDesktopApi {
  system: {
    getInfo(): Promise<ApiResult<SystemInfo>>;
  };
  agents: {
    list(): Promise<ApiResult<AgentDefinition[]>>;
    save(input: SaveAgentInput): Promise<ApiResult<AgentDefinition>>;
    remove(id: string): Promise<ApiResult<{ id: string }>>;
  };
  runs: {
    list(): Promise<ApiResult<RunRecord[]>>;
    start(input: StartRunInput): Promise<ApiResult<RunRecord>>;
    cancel(id: string): Promise<ApiResult<RunRecord>>;
    onEvent(listener: (event: RunEvent) => void): () => void;
  };
  settings: {
    get(): Promise<ApiResult<AppSettings>>;
    update(input: UpdateSettingsInput): Promise<ApiResult<AppSettings>>;
  };
}
