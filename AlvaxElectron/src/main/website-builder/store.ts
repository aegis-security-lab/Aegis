import { mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { z } from 'zod';
import {
  AcceptanceCheckSchema,
  ChatMessageSchema,
  DeliveryArtifactSchema,
  PreviewStateSchema,
  RuntimeSettingsSchema,
  WebsiteProjectSchema,
  type AcceptanceCheck,
  type ChatMessage,
  type DeliveryArtifact,
  type PreviewState,
  type RuntimeSettings,
  type WebsiteBuilderSnapshot,
  type WebsiteProject,
} from '../../shared/contracts/website-builder';

const StoredProjectSchema = z.object({
  project: WebsiteProjectSchema,
  messages: z.array(ChatMessageSchema),
  checks: z.array(AcceptanceCheckSchema),
  artifacts: z.array(DeliveryArtifactSchema),
  preview: PreviewStateSchema,
});

const StoreSchema = z.object({
  version: z.literal(1),
  projects: z.record(z.string(), StoredProjectSchema),
  runtime: RuntimeSettingsSchema,
});

type StoredProject = z.infer<typeof StoredProjectSchema>;
type Store = z.infer<typeof StoreSchema>;

export class WebsiteBuilderStore {
  private queue: Promise<unknown> = Promise.resolve();

  constructor(
    private readonly filePath: string,
    private readonly projectsRoot: string,
    private readonly defaultRuntime: RuntimeSettings,
  ) {}

  workspacePath(projectId: string): string {
    return path.join(this.projectsRoot, projectId, 'workspace');
  }

  async listProjects(): Promise<WebsiteProject[]> {
    const store = await this.read();
    return Object.values(store.projects)
      .map((entry) => entry.project)
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  }

  async get(projectId: string): Promise<WebsiteBuilderSnapshot | undefined> {
    const entry = (await this.read()).projects[projectId];
    return entry ? structuredClone(entry) : undefined;
  }

  async save(snapshot: WebsiteBuilderSnapshot): Promise<WebsiteBuilderSnapshot> {
    return this.mutate((store) => {
      store.projects[snapshot.project.id] = structuredClone(snapshot);
      return structuredClone(snapshot);
    });
  }

  async patch(
    projectId: string,
    change: (snapshot: StoredProject) => void,
  ): Promise<WebsiteBuilderSnapshot | undefined> {
    return this.mutate((store) => {
      const snapshot = store.projects[projectId];
      if (!snapshot) return undefined;
      change(snapshot);
      snapshot.project.updatedAt = new Date().toISOString();
      return structuredClone(snapshot);
    });
  }

  async getRuntime(): Promise<RuntimeSettings> {
    return structuredClone((await this.read()).runtime);
  }

  async saveRuntime(runtime: RuntimeSettings): Promise<RuntimeSettings> {
    return this.mutate((store) => {
      store.runtime = runtime;
      return structuredClone(runtime);
    });
  }

  private async read(): Promise<Store> {
    try {
      return StoreSchema.parse(JSON.parse(await readFile(this.filePath, 'utf8')));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') console.warn('[Website store]', error);
      return { version: 1, projects: {}, runtime: this.defaultRuntime };
    }
  }

  private async mutate<T>(operation: (store: Store) => T): Promise<T> {
    const work = this.queue.then(async () => {
      const store = await this.read();
      const result = operation(store);
      await mkdir(path.dirname(this.filePath), { recursive: true });
      const temporary = `${this.filePath}.${process.pid}.tmp`;
      await writeFile(temporary, JSON.stringify(store, null, 2), 'utf8');
      await rename(temporary, this.filePath);
      return result;
    });
    this.queue = work.catch(() => undefined);
    return work;
  }
}

export function createEmptySnapshot(
  project: WebsiteProject,
  artifacts: DeliveryArtifact[],
): WebsiteBuilderSnapshot {
  const messages: ChatMessage[] = [{
    id: crypto.randomUUID(),
    projectId: project.id,
    role: 'assistant',
    content: `已为「${project.brief.name}」创建网站工作区。我先生成了一套可运行的现代化首页。你可以继续告诉我品牌调性、页面结构或文案调整，我会直接修改并重新验收。`,
    state: 'complete',
    createdAt: new Date().toISOString(),
  }];
  const checks: AcceptanceCheck[] = [];
  const preview: PreviewState = { status: 'stopped' };
  return { project, messages, checks, artifacts, preview };
}
