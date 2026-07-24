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
import type { WebsiteBuilderEvent, RuntimeSettings } from '../shared/contracts/website-builder';
import { WebsiteBuilderStore } from './website-builder/store';
import { PiRpcRuntime } from './website-builder/pi-rpc-runtime';
import { PreviewManager } from './website-builder/preview-manager';
import { WebsiteBuilderService } from './website-builder/service';
import { AppLogger } from './logging/app-logger';
import { AiRuntimeConfigManager, readExistingPiApiKey } from './website-builder/ai-runtime-config';

protocol.registerSchemesAsPrivileged([
  {
    scheme: 'alvax',
    privileges: { standard: true, secure: true, supportFetchAPI: true },
  },
]);

app.setName('Alvax Studio');
if (process.platform === 'win32') app.setAppUserModelId('com.yuanfen.alvax-studio');

let mainWindow: BrowserWindow | null = null;
let orchestrator: ResearchOrchestrator | null = null;
let websiteBuilder: WebsiteBuilderService | null = null;

function broadcastRunEvent(event: RunEvent): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) window.webContents.send(IPC_CHANNELS.runEvent, event);
  }
}

function broadcastWebsiteEvent(event: WebsiteBuilderEvent): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) window.webContents.send(IPC_CHANNELS.websiteEvent, event);
  }
}

void app.whenReady().then(async () => {
  app.setAppLogsPath(path.join(app.getPath('userData'), 'logs'));
  const logger = new AppLogger(app.getPath('logs'));
  logger.info('Application', `Started ${app.getVersion()} (${process.platform}/${process.arch})`);
  process.on('uncaughtException', (error) => { logger.error('uncaughtException', error); });
  process.on('unhandledRejection', (reason) => { logger.error('unhandledRejection', reason); });
  registerAppProtocol();
  const repository = new JsonWorkspaceRepository(
    path.join(app.getPath('userData'), 'workspace-v1.json'),
  );
  orchestrator = new ResearchOrchestrator(
    repository,
    broadcastRunEvent,
    new MockAgentRuntime(),
  );
  const defaultRuntime: RuntimeSettings = {
    nodePath: process.env.PI_NODE_PATH ?? path.join(process.env.NVM_BIN ?? '/usr/local/bin', 'node'),
    piPath: process.env.PI_PATH ?? path.join(app.getPath('home'), 'Code/pi/packages/coding-agent/dist/cli.js'),
    provider: '',
    model: '',
  };
  const websiteStore = new WebsiteBuilderStore(
    path.join(app.getPath('userData'), 'website-builder-v1.json'),
    path.join(app.getPath('userData'), 'website-projects'),
    defaultRuntime,
  );
  const aiRuntimeConfig = new AiRuntimeConfigManager(
    path.join(app.getPath('userData'), 'config', 'ai-runtime.json'),
  );
  const provider = 'opencode-go';
  await aiRuntimeConfig.ensure({
    version: 1,
    provider,
    model: 'deepseek-v4-flash',
    baseUrl: 'https://opencode.ai/zen/go/v1',
    apiKey: process.env.OPENCODE_API_KEY ?? await readExistingPiApiKey(app.getPath('home'), provider),
    providerApiKeyEnv: 'OPENCODE_API_KEY',
  });
  logger.info('AI configuration', `Loaded from ${aiRuntimeConfig.filePath}`);
  websiteBuilder = new WebsiteBuilderService(
    websiteStore,
    new PiRpcRuntime(),
    new PreviewManager(),
    broadcastWebsiteEvent,
    aiRuntimeConfig,
    (scope, error, context) => logger.error(scope, error, context),
  );
  registerIpcHandlers({
    repository,
    orchestrator,
    websiteBuilder,
    getMainWindow: () => mainWindow,
    logPath: logger.filePath,
    logError: (scope, error, context) => logger.error(scope, error, context),
  });
  mainWindow = createMainWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) mainWindow = createMainWindow();
  });
});

app.on('before-quit', () => {
  orchestrator?.shutdown();
  websiteBuilder?.shutdown();
});
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
