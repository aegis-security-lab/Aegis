import { spawn, type ChildProcess } from 'node:child_process';
import { createServer } from 'node:net';

interface PreviewProcess {
  child: ChildProcess;
  url: string;
}

export class PreviewManager {
  private readonly processes = new Map<string, PreviewProcess>();

  async start(projectId: string, workspace: string, npmPath: string): Promise<string> {
    this.stop(projectId);
    const port = await getFreePort();
    const url = `http://127.0.0.1:${port}`;
    const child = spawn(npmPath, ['run', 'preview', '--', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
      cwd: workspace,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    this.processes.set(projectId, { child, url });
    child.on('exit', () => {
      if (this.processes.get(projectId)?.child === child) this.processes.delete(projectId);
    });
    await waitForHealth(url, child);
    return url;
  }

  stop(projectId: string): void {
    this.processes.get(projectId)?.child.kill('SIGTERM');
    this.processes.delete(projectId);
  }

  shutdown(): void {
    for (const projectId of this.processes.keys()) this.stop(projectId);
  }
}

function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      server.close(() => resolve(port));
    });
  });
}

async function waitForHealth(url: string, child: ChildProcess): Promise<void> {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    if (child.exitCode !== null) throw new Error('预览服务启动失败。');
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch { /* waiting */ }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  child.kill('SIGTERM');
  throw new Error('预览服务健康检查超时。');
}
