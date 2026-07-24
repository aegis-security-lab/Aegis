import { readdir } from 'node:fs/promises';
import path from 'node:path';
import { AppError } from '../core/errors';
import type {
  AcceptanceCheck,
  ChatMessage,
  CreateWebsiteProjectInput,
  RuntimeSettings,
  RuntimeStatus,
  SendWebsiteMessageInput,
  WebsiteBuilderEvent,
  WebsiteBuilderSnapshot,
  WebsiteProject,
} from '../../shared/contracts/website-builder';
import { createEmptySnapshot, type WebsiteBuilderStore } from './store';
import { installTasteSkill, writeWebsiteStarter } from './starter';
import type { PiRpcRuntime } from './pi-rpc-runtime';
import type { PreviewManager } from './preview-manager';
import { runCommand } from './process-utils';

type EventSink = (event: WebsiteBuilderEvent) => void;

export class WebsiteBuilderService {
  private sequence = 0;
  private readonly activeMessages = new Map<string, string>();

  constructor(
    private readonly store: WebsiteBuilderStore,
    private readonly pi: PiRpcRuntime,
    private readonly previews: PreviewManager,
    private readonly emit: EventSink,
  ) {}

  listProjects(): Promise<WebsiteProject[]> {
    return this.store.listProjects();
  }

  async getProject(id: string): Promise<WebsiteBuilderSnapshot> {
    const snapshot = await this.store.get(id);
    if (!snapshot) throw new AppError('NOT_FOUND', '网站项目不存在。');
    return snapshot;
  }

  async createProject(input: CreateWebsiteProjectInput): Promise<WebsiteBuilderSnapshot> {
    const now = new Date().toISOString();
    const project: WebsiteProject = {
      id: crypto.randomUUID(),
      brief: input,
      status: 'ready',
      createdAt: now,
      updatedAt: now,
    };
    const artifacts = await writeWebsiteStarter(this.store.workspacePath(project.id), input);
    const snapshot = await this.store.save(createEmptySnapshot(project, artifacts));
    this.notify(project.id, 'snapshot');
    return snapshot;
  }

  async sendMessage(input: SendWebsiteMessageInput): Promise<WebsiteBuilderSnapshot> {
    const current = await this.getProject(input.projectId);
    if (current.project.status === 'generating') {
      throw new AppError('CONFLICT', 'Pi Agent 正在生成，请等待完成或先取消。');
    }
    const runtime = await this.store.getRuntime();
    const probe = await this.pi.probe(runtime);
    if (!probe.ready) throw new AppError('CONFLICT', probe.message);

    const userMessage: ChatMessage = {
      id: crypto.randomUUID(), projectId: input.projectId, role: 'user',
      content: input.message, state: 'complete', createdAt: new Date().toISOString(),
    };
    const assistant: ChatMessage = {
      id: crypto.randomUUID(), projectId: input.projectId, role: 'assistant',
      content: '', state: 'streaming', createdAt: new Date().toISOString(),
    };
    this.activeMessages.set(input.projectId, assistant.id);
    const snapshot = await this.store.patch(input.projectId, (draft) => {
      draft.project.status = 'generating';
      draft.messages.push(userMessage, assistant);
    });
    this.notify(input.projectId, 'agent');

    const prompt = buildAgentPrompt(current, input.message);
    try {
      await installTasteSkill(this.store.workspacePath(input.projectId));
      await this.pi.prompt(input.projectId, this.store.workspacePath(input.projectId), runtime, prompt, (event) => {
        void this.handlePiEvent(input.projectId, event);
      });
    } catch (error) {
      await this.failGeneration(input.projectId, error);
    }
    return snapshot!;
  }

  async cancel(projectId: string): Promise<WebsiteBuilderSnapshot> {
    await this.pi.abort(projectId);
    const snapshot = await this.store.patch(projectId, (draft) => {
      draft.project.status = 'ready';
      const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
      if (message) message.state = 'complete';
    });
    if (!snapshot) throw new AppError('NOT_FOUND', '网站项目不存在。');
    this.activeMessages.delete(projectId);
    this.notify(projectId, 'agent');
    return snapshot;
  }

  async runAcceptance(projectId: string): Promise<WebsiteBuilderSnapshot> {
    await this.getProject(projectId);
    await this.store.patch(projectId, (draft) => {
      draft.project.status = 'checking';
      draft.checks = createChecks(projectId);
    });
    this.notify(projectId, 'acceptance');

    const runtime = await this.store.getRuntime();
    const npmPath = resolveNpm(runtime.nodePath);
    const workspace = this.store.workspacePath(projectId);
    const commands = [
      { kind: 'install' as const, args: ['install', '--no-audit', '--no-fund'], timeout: 300_000 },
      { kind: 'typecheck' as const, args: ['run', 'typecheck'], timeout: 180_000 },
      { kind: 'build' as const, args: ['run', 'build'], timeout: 180_000 },
    ];
    let passed = true;
    for (const command of commands) {
      await this.updateCheck(projectId, command.kind, { status: 'running' });
      const result = await runCommand(npmPath, command.args, workspace, command.timeout);
      const status = result.code === 0 ? 'passed' : 'failed';
      await this.updateCheck(projectId, command.kind, {
        status,
        durationMs: result.durationMs,
        summary: result.code === 0 ? '执行成功' : result.output.slice(-2_000) || '命令执行失败',
      });
      if (result.code !== 0) { passed = false; break; }
    }

    if (passed) {
      await this.updateCheck(projectId, 'health', { status: 'running' });
      try {
        const url = await this.previews.start(projectId, workspace, npmPath);
        await this.updateCheck(projectId, 'health', { status: 'passed', summary: `HTTP 200 · ${url}` });
        await this.store.patch(projectId, (draft) => {
          draft.project.status = 'previewing';
          draft.preview = { status: 'running', url };
        });
      } catch (error) {
        passed = false;
        await this.updateCheck(projectId, 'health', { status: 'failed', summary: toMessage(error) });
      }
    }
    if (!passed) {
      await this.store.patch(projectId, (draft) => { draft.project.status = 'failed'; });
    }
    this.notify(projectId, passed ? 'preview' : 'error');
    return this.getProject(projectId);
  }

  async startPreview(projectId: string): Promise<WebsiteBuilderSnapshot> {
    const runtime = await this.store.getRuntime();
    try {
      const url = await this.previews.start(projectId, this.store.workspacePath(projectId), resolveNpm(runtime.nodePath));
      await this.store.patch(projectId, (draft) => {
        draft.project.status = 'previewing';
        draft.preview = { status: 'running', url };
      });
    } catch (error) {
      await this.store.patch(projectId, (draft) => {
        draft.project.status = 'failed';
        draft.preview = { status: 'failed', error: toMessage(error) };
      });
    }
    this.notify(projectId, 'preview');
    return this.getProject(projectId);
  }

  async stopPreview(projectId: string): Promise<WebsiteBuilderSnapshot> {
    this.previews.stop(projectId);
    const snapshot = await this.store.patch(projectId, (draft) => {
      draft.project.status = 'ready';
      draft.preview = { status: 'stopped' };
    });
    if (!snapshot) throw new AppError('NOT_FOUND', '网站项目不存在。');
    this.notify(projectId, 'preview');
    return snapshot;
  }

  async getRuntime(): Promise<RuntimeStatus> {
    const settings = await this.store.getRuntime();
    return { settings, ...(await this.pi.probe(settings)) };
  }

  async updateRuntime(settings: RuntimeSettings): Promise<RuntimeStatus> {
    await this.store.saveRuntime(settings);
    return { settings, ...(await this.pi.probe(settings)) };
  }

  shutdown(): void {
    this.pi.shutdown();
    this.previews.shutdown();
  }

  private async handlePiEvent(projectId: string, event: Record<string, unknown>): Promise<void> {
    if (event.type === 'message_update') {
      const detail = event.assistantMessageEvent as Record<string, unknown> | undefined;
      if (detail?.type === 'text_delta' && typeof detail.delta === 'string') {
        await this.store.patch(projectId, (draft) => {
          const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
          if (message) message.content += detail.delta as string;
        });
        this.notify(projectId, 'agent');
      }
    } else if (event.type === 'tool_execution_start') {
      this.notify(projectId, 'agent');
    } else if (event.type === 'agent_settled') {
      await this.store.patch(projectId, (draft) => {
        draft.project.status = 'ready';
        const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
        if (message) {
          message.state = 'complete';
          if (!message.content) message.content = '网站修改已完成，正在执行类型检查、构建和可访问性验收。';
        }
      });
      this.activeMessages.delete(projectId);
      this.notify(projectId, 'agent');
      await this.refreshArtifacts(projectId);
      await this.runAcceptance(projectId);
    } else if (event.type === 'runtime_error') {
      await this.failGeneration(projectId, event.message);
    }
  }

  private async refreshArtifacts(projectId: string): Promise<void> {
    const root = this.store.workspacePath(projectId);
    const paths = await walk(root, root);
    await this.store.patch(projectId, (draft) => {
      draft.artifacts = paths.filter((entry) => !entry.startsWith('node_modules/') && !entry.startsWith('dist/')).map((entry) => ({
        path: entry,
        kind: entry.includes('App') ? 'page' : entry.endsWith('.css') ? 'asset' : 'config',
      }));
    });
  }

  private async failGeneration(projectId: string, error: unknown): Promise<void> {
    await this.store.patch(projectId, (draft) => {
      draft.project.status = 'failed';
      const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
      if (message) { message.state = 'error'; message.content ||= toMessage(error); }
    });
    this.activeMessages.delete(projectId);
    this.notify(projectId, 'error');
  }

  private async updateCheck(
    projectId: string,
    kind: AcceptanceCheck['kind'],
    patch: Partial<AcceptanceCheck>,
  ): Promise<void> {
    await this.store.patch(projectId, (draft) => {
      const check = draft.checks.find((entry) => entry.kind === kind);
      if (check) Object.assign(check, patch);
    });
    this.notify(projectId, 'acceptance');
  }

  private notify(projectId: string, type: WebsiteBuilderEvent['type']): void {
    this.emit({ projectId, type, sequence: ++this.sequence });
  }
}

function buildAgentPrompt(snapshot: WebsiteBuilderSnapshot, message: string): string {
  return `你是 Alvax Studio 的网站开发 Agent。当前工作目录就是网站源码目录。\n\n开始工作前必须读取并遵循项目内置技能：.pi/skills/design-taste-frontend/SKILL.md。先根据技能完成 Design Read，推导 DESIGN_VARIANCE、MOTION_INTENSITY、VISUAL_DENSITY，再进行设计与开发。最终回复中简要说明 Design Read 和三个参数。\n\n产品信息：\n- 名称：${snapshot.project.brief.name}\n- 行业：${snapshot.project.brief.industry}\n- 产品或服务：${snapshot.project.brief.offering}\n- 目标用户：${snapshot.project.brief.audience}\n\n用户本轮要求：${message}\n\n请直接检查并修改源码完成要求。保持 Vite + React + TypeScript + Tailwind 技术栈；可创建首页、Use Cases、FAQ、Blog/Article 等页面。不要启动长期运行的服务，也不要执行 npm install、typecheck 或 build，宿主应用会统一验收。不要修改工作目录之外的文件。结束前执行 taste skill 的 pre-flight check，并用简洁中文总结改动。`;
}

function createChecks(projectId: string): AcceptanceCheck[] {
  return [
    ['install', '依赖安装'], ['typecheck', 'TypeScript 语法检查'],
    ['build', '生产构建'], ['health', '本地服务访问'],
  ].map(([kind, label]) => ({ id: crypto.randomUUID(), projectId, kind: kind as AcceptanceCheck['kind'], label: label!, status: 'pending', summary: '', durationMs: 0 }));
}

function resolveNpm(nodePath: string): string {
  return path.join(path.dirname(nodePath), process.platform === 'win32' ? 'npm.cmd' : 'npm');
}

async function walk(root: string, directory: string): Promise<string[]> {
  const result: string[] = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist') continue;
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) result.push(...await walk(root, absolute));
    else result.push(path.relative(root, absolute));
  }
  return result;
}

function toMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
