import { spawn } from 'node:child_process';

export interface CommandResult {
  code: number;
  output: string;
  durationMs: number;
}

export function runCommand(
  command: string,
  args: string[],
  cwd: string,
  timeoutMs = 180_000,
  env: NodeJS.ProcessEnv = process.env,
): Promise<CommandResult> {
  return new Promise((resolve) => {
    const started = Date.now();
    const child = spawn(command, args, { cwd, env, shell: false });
    let output = '';
    const append = (chunk: Buffer): void => {
      output = (output + chunk.toString()).slice(-60_000);
    };
    child.stdout.on('data', append);
    child.stderr.on('data', append);
    const timer = setTimeout(() => child.kill('SIGTERM'), timeoutMs);
    child.on('error', (error) => {
      clearTimeout(timer);
      resolve({ code: -1, output: `${output}\n${error.message}`.trim(), durationMs: Date.now() - started });
    });
    child.on('close', (code) => {
      clearTimeout(timer);
      resolve({ code: code ?? -1, output: output.trim(), durationMs: Date.now() - started });
    });
  });
}
