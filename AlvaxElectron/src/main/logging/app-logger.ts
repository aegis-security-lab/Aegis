import { appendFileSync, mkdirSync } from 'node:fs';
import path from 'node:path';

export class AppLogger {
  readonly filePath: string;

  constructor(logDirectory: string) {
    mkdirSync(logDirectory, { recursive: true });
    this.filePath = path.join(logDirectory, 'alvax-studio.log');
  }

  info(scope: string, message: string): void {
    this.write('INFO', scope, message);
  }

  error(scope: string, error: unknown, context?: unknown): string {
    const errorId = crypto.randomUUID().slice(0, 8);
    const detail = error instanceof Error ? `${error.name}: ${error.message}\n${error.stack ?? ''}` : String(error);
    this.write('ERROR', scope, `[${errorId}] ${detail}${context === undefined ? '' : `\nContext: ${safeJson(context)}`}`);
    return errorId;
  }

  private write(level: string, scope: string, message: string): void {
    try {
      appendFileSync(this.filePath, `${new Date().toISOString()} [${level}] [${scope}] ${redact(message)}\n`, 'utf8');
    } catch (error) {
      console.error('[Logger]', error);
    }
  }
}

function redact(value: string): string {
  return value
    .replace(/\bsk-[A-Za-z0-9_-]{12,}\b/g, 'sk-[REDACTED]')
    .replace(/(authorization["']?\s*[:=]\s*["']?bearer\s+)[^\s"']+/gi, '$1[REDACTED]');
}

function safeJson(value: unknown): string {
  try { return JSON.stringify(value); } catch { return String(value); }
}
