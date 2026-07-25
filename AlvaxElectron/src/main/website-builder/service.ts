import { mkdir, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { AppError } from '../core/errors';
import type {
  AcceptanceCheck,
  ChatMessage,
  ConfirmationResponseInput,
  CreateWebsiteProjectInput,
  RuntimeSettings,
  RuntimeStatus,
  SendWebsiteMessageInput,
  WebsiteBuilderEvent,
  WebsiteBuilderSnapshot,
  WebsiteBrief,
  WebsiteProject,
} from '../../shared/contracts/website-builder';
import {
  ALVAX_CONFIRMATION_PREFIX,
  ALVAX_KEY_INFO_PREFIX,
  WEBSITE_BRIEF_MESSAGE_PREFIX,
} from '../../shared/contracts/website-builder';
import { createEmptySnapshot, type WebsiteBuilderStore } from './store';
import { installBundledAgentResources, writeWebsiteStarter } from './starter';
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
  private readonly cancelledProjects = new Set<string>();
  private readonly continuationAttempts = new Map<string, number>();
  private readonly recoveringProjects = new Set<string>();

  constructor(
    private readonly store: WebsiteBuilderStore,
    private readonly pi: PiRpcRuntime,
    private readonly previews: PreviewManager,
    private readonly emit: EventSink,
    private readonly aiConfig: AiRuntimeConfigManager,
    private readonly logError: (scope: string, error: unknown, context?: unknown) => string = () => '',
  ) {}

  async listProjects(): Promise<WebsiteProject[]> {
    const projects = await this.store.listProjects();
    for (const project of projects) {
      if (project.status === 'previewing' && !this.previews.has(project.id)) {
        await this.store.patch(project.id, (draft) => {
          draft.project.status = 'ready';
          draft.preview = { status: 'stopped' };
        });
        project.status = 'ready';
      }
      if ((project.status === 'generating' || project.status === 'checking')
        && !this.pi.hasSession(project.id)
        && !this.recoveringProjects.has(project.id)) {
        this.recoveringProjects.add(project.id);
        void this.recoverInterruptedProject(project).finally(() => this.recoveringProjects.delete(project.id));
      }
    }
    return projects;
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
    this.continuationAttempts.delete(id);
    return { id };
  }

  async createProject(input: CreateWebsiteProjectInput): Promise<WebsiteBuilderSnapshot> {
    const now = new Date().toISOString();
    const brief = normalizeWebsiteBrief(input);
    const project: WebsiteProject = {
      id: crypto.randomUUID(),
      brief,
      status: 'generating',
      createdAt: now,
      updatedAt: now,
    };
    const artifacts = await writeWebsiteStarter(this.store.workspacePath(project.id), brief);
    const request = buildInitialRequest(brief);
    const snapshot = await this.store.save(createEmptySnapshot(project, artifacts, request));
    this.notify(project.id, 'snapshot');
    this.repairAttempts.set(project.id, 0);
    this.continuationAttempts.set(project.id, 0);
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
    this.continuationAttempts.set(input.projectId, 0);
    await this.startAgent(input.projectId, runtime, buildAgentPrompt(current, input.message));
    return snapshot!;
  }

  async cancel(projectId: string): Promise<WebsiteBuilderSnapshot> {
    this.cancelledProjects.add(projectId);
    try { await this.pi.abort(projectId); } catch (error) {
      this.logError('Cancel website generation', error, { projectId });
    }
    const snapshot = await this.store.patch(projectId, (draft) => {
      draft.project.status = 'ready';
      const message = draft.messages.find((entry) => entry.id === this.activeMessages.get(projectId));
      if (message) message.state = 'complete';
    });
    if (!snapshot) throw new AppError('NOT_FOUND', '网站项目不存在。');
    this.activeMessages.delete(projectId);
    this.continuationAttempts.delete(projectId);
    this.notify(projectId, 'agent');
    return snapshot;
  }

  async respondToConfirmation(input: ConfirmationResponseInput): Promise<{ accepted: true }> {
    await this.getProject(input.projectId);
    const directory = path.join(this.store.workspacePath(input.projectId), '.alvax', 'confirmations');
    await mkdir(directory, { recursive: true });
    await writeFile(path.join(directory, `${input.toolCallId}.response.json`), JSON.stringify({
      approved: input.approved,
      suggestion: input.suggestion,
    }), 'utf8');
    return { accepted: true };
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
    const env = runtimeEnvironment(this.aiConfig.environment(config), runtime.nodePath);
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
      const url = await this.previews.start(projectId, this.store.workspacePath(projectId), resolveNpm(runtime.nodePath), runtimeEnvironment(this.aiConfig.environment(config), runtime.nodePath));
      await this.store.patch(projectId, (draft) => {
        draft.project.status = 'previewing';
        draft.preview = { status: 'running', url };
      });
    } catch (error) {
      this.logError('Preview start', error, { projectId });
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
      const toolName = String(event.toolName ?? 'tool');
      const message = createMessage(projectId, 'tool', formatToolStart(toolName, event.args, toolCallId), 'streaming');
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
          if (!message.content.startsWith(ALVAX_KEY_INFO_PREFIX) && !message.content.startsWith(ALVAX_CONFIRMATION_PREFIX)) {
            message.content += event.isError === true ? ' · 失败' : ' · 完成';
          }
        }
      });
      this.activeTools.get(projectId)?.delete(toolCallId);
      this.notify(projectId, 'agent');
    } else if (event.type === 'agent_settled') {
      await this.completeActiveMessage(projectId);
      if (this.cancelledProjects.delete(projectId)) {
        this.activeMessages.delete(projectId);
        this.activeTools.delete(projectId);
        this.notify(projectId, 'agent');
        return;
      }
      const current = await this.getProject(projectId);
      if (!hasFinalDeliveryInLatestTurn(current.messages)) {
        const attempt = (this.continuationAttempts.get(projectId) ?? 0) + 1;
        if (attempt <= 3) {
          this.continuationAttempts.set(projectId, attempt);
          await this.store.patch(projectId, (draft) => {
            draft.project.status = 'generating';
            draft.messages.push(createMessage(projectId, 'system', `Agent 本轮尚未完成交付，正在自动继续（${attempt}/3）。`, 'complete'));
          });
          this.activeMessages.delete(projectId);
          this.activeTools.delete(projectId);
          this.notify(projectId, 'agent');
          await delay(300);
          const runtime = await this.store.getRuntime();
          await this.startAgent(projectId, runtime, '继续完成当前任务。不要重复已经完成的工作；检查现有文件，从中断处继续。完成全部源码与 pre-flight check 后，必须调用 alvax_key_info(type=final_delivery) 输出最终交付报告。');
          return;
        }
        await this.failGeneration(projectId, 'Agent 连续三轮未完成最终交付，请补充指令后继续。');
        return;
      }
      this.continuationAttempts.delete(projectId);
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

  private async recoverInterruptedProject(project: WebsiteProject): Promise<void> {
    await this.store.patch(project.id, (draft) => {
      for (const message of draft.messages) {
        if (message.state === 'streaming') message.state = 'complete';
      }
      draft.messages.push(createMessage(
        project.id,
        'system',
        project.status === 'checking'
          ? '检测到应用重启导致验收中断，正在自动重新验收。'
          : '检测到应用重启导致 Agent 中断，正在从现有文件自动继续。',
        'complete',
      ));
    });
    this.notify(project.id, project.status === 'checking' ? 'acceptance' : 'agent');
    if (project.status === 'checking') {
      try { await this.runAcceptance(project.id); } catch (error) {
        await this.failGeneration(project.id, error);
      }
      return;
    }
    const runtime = await this.store.getRuntime();
    await this.startAgent(project.id, runtime, '应用重启中断了上一轮执行。请检查当前工作区已有文件和最近的验收错误，从中断处继续修复并完成任务，不要重做已经完成的页面。完成后必须调用 alvax_key_info(type=final_delivery) 输出最终交付报告。');
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
      this.cancelledProjects.delete(projectId);
      await installBundledAgentResources(this.store.workspacePath(projectId));
      const config = await this.aiConfig.load();
      const effectiveRuntime = { ...runtime, provider: config.provider, model: config.model };
      const systemPrompt = this.aiConfig.agentContext(config);
      const workspace = this.store.workspacePath(projectId);
      await this.pi.prompt(projectId, workspace, effectiveRuntime, prompt, (event) => {
        void this.handlePiEvent(projectId, event);
      }, runtimeEnvironment({
        ...this.aiConfig.environment(config),
        ALVAX_CONFIRMATION_DIR: path.join(workspace, '.alvax', 'confirmations'),
      }, runtime.nodePath), systemPrompt);
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

function normalizeWebsiteBrief(input: CreateWebsiteProjectInput): WebsiteBrief {
  if (input.mode === 'create') return { ...input, referenceUrl: '', referenceRequest: '' };
  const host = new URL(input.referenceUrl).hostname.replace(/^www\./, '');
  return {
    mode: 'reference', name: host, industry: '参考网站项目', offering: input.referenceRequest,
    audience: '由 AI 根据参考网站与需求分析', purposes: ['brand', 'product', 'conversion', 'content'],
    notes: '', referenceUrl: input.referenceUrl, referenceRequest: input.referenceRequest,
  };
}

function buildInitialRequest(input: WebsiteBrief): string {
  if (input.mode === 'reference') {
    return [WEBSITE_BRIEF_MESSAGE_PREFIX, '创建模式：参考网站', `参考 URL：${input.referenceUrl}`,
      `需求：${input.referenceRequest}`, '说明：该 URL 是需要研究和参考的目标网站，不是本项目提供的产品或服务。'].join('\n');
  }
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

function hasFinalDeliveryInLatestTurn(messages: ChatMessage[]): boolean {
  const lastUserIndex = messages.findLastIndex((message) => message.role === 'user');
  return messages.slice(lastUserIndex + 1).some((message) =>
    message.role === 'tool'
      && message.content.startsWith(ALVAX_KEY_INFO_PREFIX)
      && message.content.includes('"type":"final_delivery"'),
  );
}

function buildAgentPrompt(snapshot: WebsiteBuilderSnapshot, message: string): string {
  const brief = snapshot.project.brief;
  const workflow = brief.mode === 'reference'
    ? '当前必须执行“参考网站”流程：研究参考 URL → 调用 AI 分析说明页面结构、视觉语言、内容与交互 → 调用优化建议说明具体实现方向 → 调用用户确认 → 确认后按用户需求生成参考网站的本地页面 → 调用最终交付报告。用户确认前不得修改源码。'
    : '当前必须执行“创建网站项目”流程：分析行业、产品、受众与页面要求 → 调用 AI 分析 → 直接生成或修改本地页面 → 调用最终交付报告。';
  const context = brief.mode === 'reference'
    ? `参考目标：${brief.referenceUrl}\n参考需求：${brief.referenceRequest}\n\n最高优先级语义约束：参考 URL 是要研究、参考或复刻的网站，不是用户经营的产品，也不是要制作一个“复刻其他网站的工具”。除非用户明确要求，否则最终交付必须是参考目标网站本身风格、结构和体验的本地实现。`
    : `产品信息：\n- 名称：${brief.name}\n- 行业：${brief.industry}\n- 产品或服务：${brief.offering}\n- 目标用户：${brief.audience}`;
  return `你是 Alvax Studio 的网站开发 Agent。当前工作目录就是网站源码目录。\n\n开始工作前必须依次读取并遵循两个项目内置技能：\n1. .pi/skills/design-taste-frontend/SKILL.md\n2. .pi/skills/shadcn/SKILL.md\n\n先根据 design-taste-frontend 完成 Design Read，推导 DESIGN_VARIANCE、MOTION_INTENSITY、VISUAL_DENSITY；再按 shadcn 技能核对项目上下文、组件组合、表单、图标与样式规范。\n\n你必须使用 Alvax 专用工具表达关键阶段，不要用普通 assistant 文本代替：\n- alvax_key_info(type=analysis)：完成需求或参考网站分析后立即调用，输出“AI 分析”。\n- alvax_key_info(type=suggestion)：在参考网站流程中调用，输出具体的复刻或优化实现建议。\n- alvax_request_confirmation：在参考网站流程中、修改代码之前调用。工具会暂停等待用户确认；确认后必须结合返回的用户建议开始工作，取消后立即停止。\n- alvax_key_info(type=final_delivery)：所有开发与 pre-flight check 完成后作为最后一个动作调用，输出最终交付报告。调用后不要再输出普通文本。\n\n${workflow}\n\n${context}\n\n用户本轮要求：${message}\n\n保持 Vite + React + TypeScript + Tailwind 技术栈；可创建首页、Use Cases、FAQ、Blog/Article 等页面。不要启动长期运行的服务，也不要执行 npm install、typecheck 或 build，宿主应用会统一验收。不要修改工作目录之外的文件。结束前执行 taste skill 的 pre-flight check。`;
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

function runtimeEnvironment(env: NodeJS.ProcessEnv, nodePath: string): NodeJS.ProcessEnv {
  const nodeDirectory = path.dirname(nodePath);
  const existingPath = env.PATH ?? env.Path ?? '';
  return {
    ...env,
    PATH: [nodeDirectory, existingPath].filter(Boolean).join(path.delimiter),
  };
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

function formatToolStart(name: string, args: unknown, toolCallId: string): string {
  if (name === 'alvax_key_info') return `${ALVAX_KEY_INFO_PREFIX}${JSON.stringify(args ?? {})}`;
  if (name === 'alvax_request_confirmation') {
    return `${ALVAX_CONFIRMATION_PREFIX}${JSON.stringify({ ...(args as object ?? {}), toolCallId })}`;
  }
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
