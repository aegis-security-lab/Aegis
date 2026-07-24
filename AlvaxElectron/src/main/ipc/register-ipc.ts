import type { IpcMainInvokeEvent } from 'electron';
import { app, BrowserWindow, ipcMain } from 'electron';
import { z } from 'zod';
import { AppError } from '../core/errors';
import type { Orchestrator, WorkspaceRepository } from '../core/ports';
import type { WebsiteBuilderService } from '../website-builder/service';
import { IPC_CHANNELS, type ApiError, type ApiResult } from '../../shared/contracts/api';
import {
  IdSchema,
  SaveAgentInputSchema,
  StartRunInputSchema,
  UpdateSettingsInputSchema,
} from '../../shared/contracts/domain';
import {
  CreateWebsiteProjectInputSchema,
  RuntimeSettingsSchema,
  SendWebsiteMessageInputSchema,
} from '../../shared/contracts/website-builder';

interface IpcDependencies {
  repository: WorkspaceRepository;
  orchestrator: Orchestrator;
  websiteBuilder: WebsiteBuilderService;
  getMainWindow(): BrowserWindow | null;
  logPath: string;
  logError(scope: string, error: unknown, context?: unknown): string;
}

export function registerIpcHandlers(dependencies: IpcDependencies): void {
  const { repository, orchestrator, websiteBuilder } = dependencies;
  const trusted = (event: IpcMainInvokeEvent): boolean => {
    const window = dependencies.getMainWindow();
    return Boolean(
      window &&
        !window.isDestroyed() &&
        event.sender.id === window.webContents.id &&
        event.senderFrame === event.sender.mainFrame,
    );
  };

  const handle = <I, O>(
    channel: string,
    schema: z.ZodType<I>,
    operation: (input: I) => Promise<O> | O,
  ): void => {
    ipcMain.handle(channel, async (event, rawInput: unknown): Promise<ApiResult<O>> => {
      try {
        if (!trusted(event)) throw new AppError('UNAUTHORIZED', 'IPC 调用来源不可信。');
        const input = schema.parse(rawInput);
        return { ok: true, data: await operation(input) };
      } catch (error) {
        return { ok: false, error: serializeError(error, dependencies, channel) };
      }
    });
  };

  handle(IPC_CHANNELS.systemInfo, z.undefined(), () => ({
    appVersion: app.getVersion(),
    platform: process.platform,
    arch: process.arch,
    isPackaged: app.isPackaged,
    logPath: dependencies.logPath,
  }));
  handle(IPC_CHANNELS.systemLogError, z.object({
    scope: z.string().max(80),
    message: z.string().max(10_000),
    stack: z.string().max(30_000).optional(),
  }), (input) => ({ errorId: dependencies.logError(input.scope, new Error(input.message), input.stack) }));
  handle(IPC_CHANNELS.agentList, z.undefined(), () => repository.listAgents());
  handle(IPC_CHANNELS.agentSave, SaveAgentInputSchema, (input) =>
    repository.saveAgent(input),
  );
  handle(IPC_CHANNELS.agentRemove, IdSchema, async (id) => {
    await repository.removeAgent(id);
    return { id };
  });
  handle(IPC_CHANNELS.runList, z.undefined(), () => repository.listRuns());
  handle(IPC_CHANNELS.runStart, StartRunInputSchema, (input) => orchestrator.start(input));
  handle(IPC_CHANNELS.runCancel, IdSchema, (id) => orchestrator.cancel(id));
  handle(IPC_CHANNELS.settingsGet, z.undefined(), () => repository.getSettings());
  handle(IPC_CHANNELS.settingsUpdate, UpdateSettingsInputSchema, (input) =>
    repository.updateSettings(input),
  );
  handle(IPC_CHANNELS.websiteProjectList, z.undefined(), () => websiteBuilder.listProjects());
  handle(IPC_CHANNELS.websiteProjectCreate, CreateWebsiteProjectInputSchema, (input) =>
    websiteBuilder.createProject(input),
  );
  handle(IPC_CHANNELS.websiteProjectGet, IdSchema, (id) => websiteBuilder.getProject(id));
  handle(IPC_CHANNELS.websiteMessageSend, SendWebsiteMessageInputSchema, (input) =>
    websiteBuilder.sendMessage(input),
  );
  handle(IPC_CHANNELS.websiteMessageCancel, IdSchema, (id) => websiteBuilder.cancel(id));
  handle(IPC_CHANNELS.websiteAcceptanceRun, IdSchema, (id) => websiteBuilder.runAcceptance(id));
  handle(IPC_CHANNELS.websitePreviewStart, IdSchema, (id) => websiteBuilder.startPreview(id));
  handle(IPC_CHANNELS.websitePreviewStop, IdSchema, (id) => websiteBuilder.stopPreview(id));
  handle(IPC_CHANNELS.websitePreviewOpenWindow, IdSchema, async (id) => {
    // A persisted preview URL may point to a server from a previous app process.
    // Starting here makes the action self-healing after an app restart.
    const snapshot = await websiteBuilder.startPreview(id);
    const url = snapshot.preview.url;
    if (!url || snapshot.preview.status !== 'running') {
      const errorId = dependencies.logError('Preview window', snapshot.preview.error ?? '预览服务启动失败', { projectId: id });
      throw new AppError('CONFLICT', `预览服务启动失败（错误编号 ${errorId}）。日志：${dependencies.logPath}`);
    }
    const parsed = new URL(url);
    if (!['127.0.0.1', 'localhost'].includes(parsed.hostname)) {
      throw new AppError('UNAUTHORIZED', '只能在新窗口中打开本地预览。');
    }
    const previewWindow = new BrowserWindow({
      width: 1280,
      height: 820,
      minWidth: 720,
      minHeight: 520,
      title: `${snapshot.project.brief.name} · 预览`,
      backgroundColor: '#ffffff',
      webPreferences: {
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: true,
        webSecurity: true,
        devTools: !process.env.CI,
      },
    });
    previewWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
    previewWindow.webContents.on('will-navigate', (event, target) => {
      if (new URL(target).origin !== parsed.origin) event.preventDefault();
    });
    await previewWindow.loadURL(url);
    return { opened: true as const };
  });
  handle(IPC_CHANNELS.websiteRuntimeGet, z.undefined(), () => websiteBuilder.getRuntime());
  handle(IPC_CHANNELS.websiteRuntimeUpdate, RuntimeSettingsSchema, (input) =>
    websiteBuilder.updateRuntime(input),
  );
}

function serializeError(error: unknown, dependencies: IpcDependencies, channel: string): ApiError {
  if (error instanceof AppError) {
    return {
      code: error.code,
      message: error.message,
      ...(error.details ? { details: error.details } : {}),
    };
  }
  if (error instanceof z.ZodError) {
    const details: Record<string, string[]> = {};
    for (const issue of error.issues) {
      const key = issue.path.join('.') || '_root';
      details[key] = [...(details[key] ?? []), issue.message];
    }
    return { code: 'VALIDATION_ERROR', message: '提交的数据格式不正确。', details };
  }
  const errorId = dependencies.logError('IPC', error, { channel });
  return {
    code: 'INTERNAL_ERROR',
    message: `应用遇到了内部错误（错误编号 ${errorId}）。日志：${dependencies.logPath}`,
  };
}
