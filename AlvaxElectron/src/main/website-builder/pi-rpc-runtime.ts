import { access } from 'node:fs/promises';
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { createHash } from 'node:crypto';
import path from 'node:path';
import type { RuntimeSettings } from '../../shared/contracts/website-builder';

type EventSink = (event: Record<string, unknown>) => void;

interface Session {
  child: ChildProcessWithoutNullStreams;
  buffer: string;
  requests: Map<string, { resolve: () => void; reject: (error: Error) => void }>;
  sink: EventSink;
  fingerprint: string;
}

export class PiRpcRuntime {
  private readonly sessions = new Map<string, Session>();

  async probe(settings: RuntimeSettings): Promise<{ ready: boolean; message: string }> {
    try {
      await Promise.all([access(settings.nodePath), access(settings.piPath)]);
      return { ready: true, message: 'Pi Agent RPC 已就绪' };
    } catch {
      return { ready: false, message: '未找到 Node 或 Pi CLI，请在设置中配置路径' };
    }
  }

  async prompt(
    projectId: string,
    workspace: string,
    settings: RuntimeSettings,
    message: string,
    sink: EventSink,
    env: NodeJS.ProcessEnv = process.env,
    systemPrompt = '',
  ): Promise<void> {
    let session = this.sessions.get(projectId);
    const fingerprint = runtimeFingerprint(settings, env, systemPrompt);
    if (session && session.fingerprint !== fingerprint) {
      session.child.kill('SIGTERM');
      this.sessions.delete(projectId);
      session = undefined;
    }
    if (!session) {
      session = this.start(projectId, workspace, settings, sink, env, fingerprint, systemPrompt);
      await new Promise((resolve) => setTimeout(resolve, 180));
      if (session.child.exitCode !== null) throw new Error('Pi Agent 启动失败。');
    } else {
      session.sink = sink;
    }
    await this.send(session, { type: 'prompt', message });
  }

  async abort(projectId: string): Promise<void> {
    const session = this.sessions.get(projectId);
    if (session) await this.send(session, { type: 'abort' });
  }

  shutdown(): void {
    for (const session of this.sessions.values()) session.child.kill('SIGTERM');
    this.sessions.clear();
  }

  private start(
    projectId: string,
    workspace: string,
    settings: RuntimeSettings,
    sink: EventSink,
    env: NodeJS.ProcessEnv,
    fingerprint: string,
    systemPrompt: string,
  ): Session {
    const args = [
      settings.piPath,
      '--mode', 'rpc',
      '--no-session',
      '--extension', path.join(workspace, '.pi/extensions/alvax-tools.ts'),
    ];
    if (settings.provider) args.push('--provider', settings.provider);
    if (settings.model) args.push('--model', settings.model);
    if (systemPrompt) args.push('--append-system-prompt', systemPrompt);
    const child = spawn(settings.nodePath, args, {
      cwd: workspace,
      env,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    const session: Session = { child, buffer: '', requests: new Map(), sink, fingerprint };
    child.stdout.on('data', (chunk: Buffer) => this.read(session!, chunk.toString()));
    child.stderr.on('data', (chunk: Buffer) => sink({ type: 'runtime_stderr', message: chunk.toString() }));
    child.on('exit', () => {
      if (this.sessions.get(projectId) === session) this.sessions.delete(projectId);
      for (const request of session.requests.values()) request.reject(new Error('Pi Agent 已退出。'));
    });
    this.sessions.set(projectId, session);
    return session;
  }

  private read(session: Session, chunk: string): void {
    session.buffer += chunk;
    for (;;) {
      const newline = session.buffer.indexOf('\n');
      if (newline < 0) break;
      const line = session.buffer.slice(0, newline).replace(/\r$/, '');
      session.buffer = session.buffer.slice(newline + 1);
      if (!line) continue;
      try {
        const payload = JSON.parse(line) as Record<string, unknown>;
        if (payload.type === 'response' && typeof payload.id === 'string') {
          const request = session.requests.get(payload.id);
          if (request) {
            session.requests.delete(payload.id);
            if (payload.success === true) request.resolve();
            else request.reject(new Error(String(payload.error ?? 'Pi RPC 请求失败')));
          }
        } else {
          session.sink(payload);
        }
      } catch (error) {
        session.sink({ type: 'runtime_error', message: `无法解析 Pi 输出：${String(error)}` });
      }
    }
  }

  private send(session: Session, command: Record<string, unknown>): Promise<void> {
    const id = crypto.randomUUID();
    return new Promise((resolve, reject) => {
      session.requests.set(id, { resolve, reject });
      session.child.stdin.write(`${JSON.stringify({ id, ...command })}\n`, (error) => {
        if (error) {
          session.requests.delete(id);
          reject(error);
        }
      });
    });
  }
}

function runtimeFingerprint(settings: RuntimeSettings, env: NodeJS.ProcessEnv, systemPrompt: string): string {
  return createHash('sha256').update(JSON.stringify({
    nodePath: settings.nodePath,
    piPath: settings.piPath,
    provider: settings.provider,
    model: settings.model,
    baseUrl: env.ALVAX_AI_BASE_URL,
    apiKey: env.ALVAX_AI_API_KEY,
    systemPrompt,
  })).digest('hex');
}
