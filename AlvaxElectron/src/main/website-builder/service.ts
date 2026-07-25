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
import { WEBSITE_BRIEF_MESSAGE_PREFIX } from '../../shared/contracts/website-builder';
import { createEmptySnapshot, type WebsiteBuilderStore } from './store';
import { installBundledSkills, writeWebsiteStarter } from './starter';
import type { PiRpcRuntime } from './pi-rpc-runtime';
import type { PreviewManager } from './preview-manager';
import { runCommand } from './process-utils';
import type { AiRuntimeConfigManager } from './ai-runtime-config';

type EventSink = (event: WebsiteBuilderEvent) => void;

export class WebsiteBuilderService {
  private sequence = 0;
  private readonly activeMessages = new Map<string, string>();
  private readonly activeTools = new Map<string, Map<string, string>>();
  private readonly repairAttempts = new Map<string, number>();

  constructor(
    private readonly store: WebsiteBuilderStore,
    private readonly pi: PiRpcRuntime,
    private readonly previews: PreviewManager,
    private readonly emit: EventSink,
    private readonly aiConfig: AiRuntimeConfigManager,
    private readonly logError: (scope: string, error: unknown, context?: unknown) => string = () => '',
  ) {}

  listProjects(): Promise<WebsiteProject[]> {
    return this.store.listProjects();
  }

  async getProject(id: string): Promise<WebsiteBuilderSnapshot> {
    const snapshot = await this.store.get(id);
    if (!snapshot) throw new AppError('NOT_FOUND', '网站项目不存在。');
    return snapshot;
  }

  async removeProject(id: string): Promise<{ id: string }> {
    const current = await this.getProject(id);
    if (current.project.status === 'generating') await this.pi.abort(id);
    this.previews.stop(id);
    const removed = await this.store.remove(id);
    if (!removed) throw new AppError('NOT_FOUND', '网站项目不存在。');
    this.activeMessages.delete(id);
    this.activeTools.delete(id);
    this.repairAttempts.delete(id);
    return { id };
  }

  async createProject(input: CreateWebsiteProjectInput): Promise<WebsiteBuilderSnapshot> {
    const now = new Date().toISOString();
    const project: WebsiteProject = {
      id: crypto.randomUUID(),
      brief: input,
      status: 'generating',
      createdAt: now,
      updatedAt: now,
    };
    const artifacts = await writeWebsiteStarter(this.store.workspacePath(project.id), input);
    const request = buildInitialRequest(input);
    const snapshot = await this.store.save(createEmptySnapshot(project, artifacts, request));
    this.notify(project.id, 'snapshot');
    this.repairAttempts.set(project.id, 0);
    const runtime = await this.store.getRuntime();
    void this.startAgent(project.id, runtime, buildAgentPrompt(snapshot, request));
    return snapshot;
  }

  async sendMessage(input: SendWebsiteMessageInput): Promise<WebsiteBuilderSnapshot> {
    const current = await this.getProject(input.projectId);
    if (current.project.status === 'generating') {
      throw new AppError('CONFLICT', 'Pi Agent 正在生成，请等待完成或先取消。');
    }
    const runtime = await this.store.getRuntime();
    const config = await this.aiConfig.load();
    const effectiveRuntime = { ...runtime, provider: config.provider, model: config.model };
    const probe = await this.pi.probe(effectiveRuntime);
    if (!probe.ready) throw new AppError('CONFLICT', probe.message);

    const userMessage: ChatMessage = {
      id: crypto.randomUUID(), projectId: input.projectId, role: 'user',
      content: input.message, state: 'complete', createdAt: new Date().toISOString(),
    };
    const snapshot = await this.store.patch(input.projectId, (draft) => {
      draft.project.status = 'generating';
      draft.messages.push(userMessage);
    });
    this.notify(input.projectId, 'agent');
    this.repairAttempts.set(input.projectId, 0);
    await this.startAgent(input.projectId, runtime, buildAgentPrompt(current, input.message));
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
    const config = await this.aiConfig.load();
    const env = this.aiConfig.environment(config);
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
      const result = await runCommand(npmPath, command.args, workspace, command.timeout, env);
      const status = result.code === 0 ? 'passed' : 'failed';
      await this.updateCheck(projectId, command.kind, {
        status,
        durationMs: result.durationMs,
        summary: result.code === 0 ? '执行成功' : result.output.slice(-2_000) || '命令执行失败',
      });
      if (result.code === 0) await delay(420);
      if (result.code !== 0) { passed = false; break; }
    }

    if (passed) {
      await this.updateCheck(projectId, 'health', { status: 'running' });
      try {
        const url = await this.previews.start(projectId, workspace, npmPath, env);
        await this.updateCheck(projectId, 'health', { status: 'passed', summary: `HTTP 200 · ${url}` });
        await delay(420);
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
    if (!passed) void this.requestAutomaticRepair(projectId);
    return this.getProject(projectId);
  }

  async startPreview(projectId: string): Promise<WebsiteBuilderSnapshot> {
    const runtime = await this.store.getRuntime();
    try {
      const config = await this.aiConfig.load();
      const url = await this.previews.start(projectId, this.store.workspacePath(projectId), resolveNpm(runtime.nodePath), this.aiConfig.environment(config));
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
    const config = await this.aiConfig.load();
    const effective = { ...settings, provider: config.provider, model: config.model };
    return { settings: effective, ...(await this.pi.probe(effective)) };
  }

  async updateRuntime(settings: RuntimeSettings): Promise<RuntimeStatus> {
    await this.store.saveRuntime(settings);
    const config = await this.aiConfig.load();
    const effective = { ...settings, provider: config.provider, model: config.model };
    return { settings: effective, ...(await this.pi.probe(effective)) };
  }

  shutdown(): void {
    this.pi.shutdown();
    this.previews.shutdown();
  }

  private async handlePiEvent(projectId: string, event: Record<string, unknown>): Promise<void> {
    if (event.type === 'message_update') {
      const detail = event.assistantMessageEvent as Record<string, unknown> | undefined;
      if (detail?.type === 'text_start') {
        const id = crypto.randomUUID();
        this.activeMessages.set(projectId, id);
        await this.store.patch(projectId, (draft) => {
          draft.messages.push(createMessage(projectId, 'assistant', '', 'streaming', id));
        });
        this.notify(projectId, 'agent');
      } else if (detail?.type === 'text_delta' && typeof detail.delta === 'string') {
        await this.store.patch(projectId, (draft) => {
          const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
          if (!message) return;
          const parts = `${message.content}${detail.delta as string}`.split(/\n\s*\n/);
          if (parts.length === 1) {
            message.content = parts[0]!;
            return;
          }
          message.content = parts.shift()!;
          message.state = 'complete';
          for (const [index, content] of parts.entries()) {
            const next = createMessage(projectId, 'assistant', content, index === parts.length - 1 ? 'streaming' : 'complete');
            draft.messages.push(next);
            if (index === parts.length - 1) this.activeMessages.set(projectId, next.id);
          }
        });
        this.notify(projectId, 'agent');
      } else if (detail?.type === 'text_end') {
        await this.completeActiveMessage(projectId);
      }
    } else if (event.type === 'tool_execution_start') {
      await this.completeActiveMessage(projectId);
      const toolCallId = String(event.toolCallId ?? crypto.randomUUID());
      const message = createMessage(projectId, 'tool', formatToolStart(String(event.toolName ?? 'tool'), event.args), 'streaming');
      const tools = this.activeTools.get(projectId) ?? new Map<string, string>();
      tools.set(toolCallId, message.id);
      this.activeTools.set(projectId, tools);
      await this.store.patch(projectId, (draft) => { draft.messages.push(message); });
      this.notify(projectId, 'agent');
    } else if (event.type === 'tool_execution_end') {
      const toolCallId = String(event.toolCallId ?? '');
      const messageId = this.activeTools.get(projectId)?.get(toolCallId);
      await this.store.patch(projectId, (draft) => {
        const message = draft.messages.find((entry) => entry.id === messageId);
        if (message) {
          message.state = event.isError === true ? 'error' : 'complete';
          message.content += event.isError === true ? ' · 失败' : ' · 完成';
        }
      });
      this.activeTools.get(projectId)?.delete(toolCallId);
      this.notify(projectId, 'agent');
    } else if (event.type === 'agent_settled') {
      await this.completeActiveMessage(projectId);
      await this.store.patch(projectId, (draft) => {
        draft.project.status = 'ready';
        draft.messages.push(createMessage(projectId, 'system', '开发完成，开始自动验收。', 'complete'));
      });
      this.activeMessages.delete(projectId);
      this.activeTools.delete(projectId);
      this.notify(projectId, 'agent');
      await this.refreshArtifacts(projectId);
      await this.runAcceptance(projectId);
    } else if (event.type === 'runtime_error') {
      await this.failGeneration(projectId, event.message);
    } else if (event.type === 'runtime_stderr') {
      this.logError('Pi stderr', event.message, { projectId });
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
    this.logError('Website generation', error, { projectId });
    await this.store.patch(projectId, (draft) => {
      draft.project.status = 'failed';
      const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
      if (message) { message.state = 'error'; message.content ||= toMessage(error); }
      else draft.messages.push(createMessage(projectId, 'system', `AI 生成启动失败：${toMessage(error)}`, 'error'));
    });
    this.activeMessages.delete(projectId);
    this.notify(projectId, 'error');
  }

  private async startAgent(projectId: string, runtime: RuntimeSettings, prompt: string): Promise<void> {
    try {
      await installBundledSkills(this.store.workspacePath(projectId));
      const config = await this.aiConfig.load();
      const effectiveRuntime = { ...runtime, provider: config.provider, model: config.model };
      const systemPrompt = this.aiConfig.agentContext(config);
      await this.pi.prompt(projectId, this.store.workspacePath(projectId), effectiveRuntime, prompt, (event) => {
        void this.handlePiEvent(projectId, event);
      }, this.aiConfig.environment(config), systemPrompt);
    } catch (error) {
      await this.failGeneration(projectId, error);
    }
  }

  private async completeActiveMessage(projectId: string): Promise<void> {
    const id = this.activeMessages.get(projectId);
    if (!id) return;
    await this.store.patch(projectId, (draft) => {
      const message = draft.messages.find((entry) => entry.id === id);
      if (message) message.state = 'complete';
    });
    this.activeMessages.delete(projectId);
    this.notify(projectId, 'agent');
  }

  private async requestAutomaticRepair(projectId: string): Promise<void> {
    const attempt = (this.repairAttempts.get(projectId) ?? 0) + 1;
    if (attempt > 2) return;
    this.repairAttempts.set(projectId, attempt);
    const snapshot = await this.getProject(projectId);
    const failed = snapshot.checks.find((check) => check.status === 'failed');
    if (!failed) return;
    const instruction = `自动验收发现「${failed.label}」未通过：\n${failed.summary}\n\n请定位问题、修复源码，然后重新交付。`;
    await this.store.patch(projectId, (draft) => {
      draft.project.status = 'generating';
      draft.messages.push(createMessage(projectId, 'system', instruction, 'complete'));
    });
    this.notify(projectId, 'agent');
    const runtime = await this.store.getRuntime();
    await this.startAgent(projectId, runtime, buildAgentPrompt(snapshot, instruction));
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

function buildInitialRequest(input: CreateWebsiteProjectInput): string {
  const purposeLabels: Record<string, string> = {
    brand: '品牌展示', product: '产品介绍', conversion: '获客转化', content: '内容发布',
  };
  return [
    WEBSITE_BRIEF_MESSAGE_PREFIX,
    `项目：${input.name}`,
    `行业：${input.industry}`,
    `产品或服务：${input.offering}`,
    `目标用户：${input.audience}`,
    `网站用途：${input.purposes.map((purpose) => purposeLabels[purpose] ?? purpose).join('、')}`,
    ...(input.notes ? [`补充说明：${input.notes}`] : []),
  ].join('\n');
}

function buildAgentPrompt(snapshot: WebsiteBuilderSnapshot, message: string): string {
  return `你是 Alvax Studio 的网站开发 Agent。当前工作目录就是网站源码目录。\n\n开始工作前必须依次读取并遵循两个项目内置技能：\n1. .pi/skills/design-taste-frontend/SKILL.md\n2. .pi/skills/shadcn/SKILL.md\n\n先根据 design-taste-frontend 完成 Design Read，推导 DESIGN_VARIANCE、MOTION_INTENSITY、VISUAL_DENSITY；再按 shadcn 技能核对项目上下文、组件组合、表单、图标与样式规范，然后进行设计与开发。最终回复中简要说明 Design Read、三个参数以及使用的 shadcn 组件。\n\n产品信息：\n- 名称：${snapshot.project.brief.name}\n- 行业：${snapshot.project.brief.industry}\n- 产品或服务：${snapshot.project.brief.offering}\n- 目标用户：${snapshot.project.brief.audience}\n\n用户本轮要求：${message}\n\n请直接检查并修改源码完成要求。保持 Vite + React + TypeScript + Tailwind 技术栈；可创建首页、Use Cases、FAQ、Blog/Article 等页面。不要启动长期运行的服务，也不要执行 npm install、typecheck 或 build，宿主应用会统一验收。不要修改工作目录之外的文件。结束前执行 taste skill 的 pre-flight check，并用简洁中文总结改动。`;
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

function createMessage(
  projectId: string,
  role: ChatMessage['role'],
  content: string,
  state: ChatMessage['state'],
  id = crypto.randomUUID(),
): ChatMessage {
  return { id, projectId, role, content, state, createdAt: new Date().toISOString() };
}

function formatToolStart(name: string, args: unknown): string {
  const labels: Record<string, string> = {
    bash: '工具 · 执行命令',
    read: '工具 · 读取文件',
    write: '工具 · 写入文件',
    edit: '工具 · 修改文件',
  };
  let detail = '';
  if (args && typeof args === 'object') {
    const value = args as Record<string, unknown>;
    detail = String(value.path ?? value.file_path ?? value.command ?? value.cmd ?? '');
  }
  const compact = detail.replace(/\s+/g, ' ').trim().slice(0, 120);
  return `${labels[name] ?? `工具 · 调用 ${name}`}${compact ? ` · ${compact}` : ''}`;
}

function delay(durationMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, durationMs));
}
