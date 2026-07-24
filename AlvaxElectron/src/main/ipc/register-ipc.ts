import type { BrowserWindow, IpcMainInvokeEvent } from 'electron';
import { app, ipcMain } from 'electron';
import { z } from 'zod';
import { AppError } from '../core/errors';
import type { Orchestrator, WorkspaceRepository } from '../core/ports';
import { IPC_CHANNELS, type ApiError, type ApiResult } from '../../shared/contracts/api';
import {
  IdSchema,
  SaveAgentInputSchema,
  StartRunInputSchema,
  UpdateSettingsInputSchema,
} from '../../shared/contracts/domain';

interface IpcDependencies {
  repository: WorkspaceRepository;
  orchestrator: Orchestrator;
  getMainWindow(): BrowserWindow | null;
}

export function registerIpcHandlers(dependencies: IpcDependencies): void {
  const { repository, orchestrator } = dependencies;
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
        return { ok: false, error: serializeError(error) };
      }
    });
  };

  handle(IPC_CHANNELS.systemInfo, z.undefined(), () => ({
    appVersion: app.getVersion(),
    platform: process.platform,
    arch: process.arch,
    isPackaged: app.isPackaged,
  }));
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
}

function serializeError(error: unknown): ApiError {
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
  console.error('[IPC]', error);
  return { code: 'INTERNAL_ERROR', message: '应用遇到了内部错误。' };
}
