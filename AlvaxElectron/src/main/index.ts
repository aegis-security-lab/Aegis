import path from 'node:path';
import { app, BrowserWindow, protocol } from 'electron';
import { ResearchOrchestrator } from './core/research-orchestrator';
import { JsonWorkspaceRepository } from './infrastructure/json-workspace-repository';
import { MockAgentRuntime } from './infrastructure/mock-agent-runtime';
import { registerIpcHandlers } from './ipc/register-ipc';
import { registerAppProtocol } from './protocol/register-app-protocol';
import { createMainWindow } from './window/create-main-window';
import { IPC_CHANNELS } from '../shared/contracts/api';
import type { RunEvent } from '../shared/contracts/domain';

protocol.registerSchemesAsPrivileged([
  {
    scheme: 'alvax',
    privileges: { standard: true, secure: true, supportFetchAPI: true },
  },
]);

app.setName('Alvax AI');
if (process.platform === 'win32') app.setAppUserModelId('com.yuanfen.alvax-ai');

let mainWindow: BrowserWindow | null = null;
let orchestrator: ResearchOrchestrator | null = null;

function broadcastRunEvent(event: RunEvent): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) window.webContents.send(IPC_CHANNELS.runEvent, event);
  }
}

void app.whenReady().then(() => {
  registerAppProtocol();
  const repository = new JsonWorkspaceRepository(
    path.join(app.getPath('userData'), 'workspace-v1.json'),
  );
  orchestrator = new ResearchOrchestrator(
    repository,
    broadcastRunEvent,
    new MockAgentRuntime(),
  );
  registerIpcHandlers({
    repository,
    orchestrator,
    getMainWindow: () => mainWindow,
  });
  mainWindow = createMainWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) mainWindow = createMainWindow();
  });
});

app.on('before-quit', () => orchestrator?.shutdown());
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
