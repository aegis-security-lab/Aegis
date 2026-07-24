import { contextBridge, ipcRenderer } from 'electron';
import { IPC_CHANNELS, type AlvaxDesktopApi, type ApiResult } from '../shared/contracts/api';
import { RunEventSchema } from '../shared/contracts/domain';

const invoke = <T>(channel: string, input?: unknown): Promise<ApiResult<T>> =>
  ipcRenderer.invoke(channel, input) as Promise<ApiResult<T>>;

const api: AlvaxDesktopApi = {
  system: {
    getInfo: () => invoke(IPC_CHANNELS.systemInfo),
  },
  agents: {
    list: () => invoke(IPC_CHANNELS.agentList),
    save: (input) => invoke(IPC_CHANNELS.agentSave, input),
    remove: (id) => invoke(IPC_CHANNELS.agentRemove, id),
  },
  runs: {
    list: () => invoke(IPC_CHANNELS.runList),
    start: (input) => invoke(IPC_CHANNELS.runStart, input),
    cancel: (id) => invoke(IPC_CHANNELS.runCancel, id),
    onEvent: (listener) => {
      const handler = (_event: Electron.IpcRendererEvent, payload: unknown): void => {
        const parsed = RunEventSchema.safeParse(payload);
        if (parsed.success) listener(parsed.data);
      };
      ipcRenderer.on(IPC_CHANNELS.runEvent, handler);
      return () => ipcRenderer.removeListener(IPC_CHANNELS.runEvent, handler);
    },
  },
  settings: {
    get: () => invoke(IPC_CHANNELS.settingsGet),
    update: (input) => invoke(IPC_CHANNELS.settingsUpdate, input),
  },
};

contextBridge.exposeInMainWorld('alvax', api);
