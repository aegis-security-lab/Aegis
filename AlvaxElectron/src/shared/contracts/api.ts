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
import type {
  CreateWebsiteProjectInput,
  RuntimeSettings,
  RuntimeStatus,
  SendWebsiteMessageInput,
  WebsiteBuilderEvent,
  WebsiteBuilderSnapshot,
  WebsiteProject,
} from './website-builder';

export const IPC_CHANNELS = {
  systemInfo: 'alvax:system:info',
  systemLogError: 'alvax:system:log-error',
  agentList: 'alvax:agent:list',
  agentSave: 'alvax:agent:save',
  agentRemove: 'alvax:agent:remove',
  runList: 'alvax:run:list',
  runStart: 'alvax:run:start',
  runCancel: 'alvax:run:cancel',
  runEvent: 'alvax:run:event',
  settingsGet: 'alvax:settings:get',
  settingsUpdate: 'alvax:settings:update',
  websiteProjectList: 'alvax:website-project:list',
  websiteProjectCreate: 'alvax:website-project:create',
  websiteProjectGet: 'alvax:website-project:get',
  websiteProjectRemove: 'alvax:website-project:remove',
  websiteMessageSend: 'alvax:website-message:send',
  websiteMessageCancel: 'alvax:website-message:cancel',
  websiteAcceptanceRun: 'alvax:website-acceptance:run',
  websitePreviewStart: 'alvax:website-preview:start',
  websitePreviewStop: 'alvax:website-preview:stop',
  websitePreviewOpenWindow: 'alvax:website-preview:open-window',
  websiteRuntimeGet: 'alvax:website-runtime:get',
  websiteRuntimeUpdate: 'alvax:website-runtime:update',
  websiteEvent: 'alvax:website:event',
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
    logError(input: { scope: string; message: string; stack?: string }): Promise<ApiResult<{ errorId: string }>>;
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
  websiteBuilder: {
    listProjects(): Promise<ApiResult<WebsiteProject[]>>;
    createProject(input: CreateWebsiteProjectInput): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    getProject(id: string): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    removeProject(id: string): Promise<ApiResult<{ id: string }>>;
    sendMessage(input: SendWebsiteMessageInput): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    cancel(projectId: string): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    runAcceptance(projectId: string): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    startPreview(projectId: string): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    stopPreview(projectId: string): Promise<ApiResult<WebsiteBuilderSnapshot>>;
    openPreviewWindow(projectId: string): Promise<ApiResult<{ opened: true }>>;
    getRuntime(): Promise<ApiResult<RuntimeStatus>>;
    updateRuntime(input: RuntimeSettings): Promise<ApiResult<RuntimeStatus>>;
    onEvent(listener: (event: WebsiteBuilderEvent) => void): () => void;
  };
}
