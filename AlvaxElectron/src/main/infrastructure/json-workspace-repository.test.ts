import { mkdtemp, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { JsonWorkspaceRepository } from './json-workspace-repository';

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(
    temporaryDirectories.splice(0).map((directory) =>
      rm(directory, { recursive: true, force: true }),
    ),
  );
});

describe('JsonWorkspaceRepository', () => {
  it('seeds a useful research workspace and persists agent changes', async () => {
    const directory = await mkdtemp(path.join(os.tmpdir(), 'alvax-test-'));
    temporaryDirectories.push(directory);
    const statePath = path.join(directory, 'workspace.json');
    const repository = new JsonWorkspaceRepository(statePath);

    expect(await repository.listAgents()).toHaveLength(3);
    const saved = await repository.saveAgent({
      name: 'Planner',
      role: 'Planning',
      description: 'Plans the work',
      capabilities: ['planning'],
      instructions: 'Create a plan.',
      runtime: { kind: 'mock', model: 'research-simulator' },
      status: 'active',
    });

    const reloaded = new JsonWorkspaceRepository(statePath);
    expect((await reloaded.listAgents()).some((agent) => agent.id === saved.id)).toBe(true);
  });
});
