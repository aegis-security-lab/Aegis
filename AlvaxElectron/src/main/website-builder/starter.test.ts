import { mkdtemp, readFile, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { afterEach, describe, expect, it } from 'vitest';
import { installBundledAgentResources, writeWebsiteStarter } from './starter';

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) =>
    rm(directory, { recursive: true, force: true }),
  ));
});

describe('website starter bundled skills', () => {
  it('installs design and shadcn skills into every new workspace', async () => {
    const workspace = await mkdtemp(path.join(tmpdir(), 'alvax-starter-'));
    temporaryDirectories.push(workspace);

    const artifacts = await writeWebsiteStarter(workspace, {
      mode: 'create',
      name: 'Nova',
      industry: '企业服务',
      offering: 'AI 客户支持',
      audience: '客户成功团队',
      purposes: ['brand'],
      notes: '',
      referenceUrl: '',
      referenceRequest: '',
    });
    const skillPath = '.pi/skills/design-taste-frontend/SKILL.md';
    const content = await readFile(path.join(workspace, skillPath), 'utf8');

    expect(content).toContain('name: design-taste-frontend');
    expect(artifacts.some((artifact) => artifact.path === skillPath)).toBe(true);
    const shadcnPath = '.pi/skills/shadcn/SKILL.md';
    await expect(readFile(path.join(workspace, shadcnPath), 'utf8')).resolves.toContain('name: shadcn');
    expect(artifacts.some((artifact) => artifact.path === shadcnPath)).toBe(true);
    await expect(readFile(path.join(workspace, '.pi/skills/shadcn/rules/styling.md'), 'utf8')).resolves.toContain('Tailwind');
    const extensionPath = '.pi/extensions/alvax-tools.ts';
    await expect(readFile(path.join(workspace, extensionPath), 'utf8')).resolves.toContain("name: 'alvax_key_info'");
    await expect(readFile(path.join(workspace, extensionPath), 'utf8')).resolves.toContain("Type.Literal('competitor_research')");
    await expect(readFile(path.join(workspace, extensionPath), 'utf8')).resolves.toContain("name: 'alvax_browser_inspect'");
    expect(artifacts.some((artifact) => artifact.path === extensionPath)).toBe(true);
  });

  it('repairs the bundled skill in an existing workspace', async () => {
    const workspace = await mkdtemp(path.join(tmpdir(), 'alvax-skill-'));
    temporaryDirectories.push(workspace);

    await installBundledAgentResources(workspace);

    await expect(readFile(
      path.join(workspace, '.pi/skills/design-taste-frontend/SKILL.md'),
      'utf8',
    )).resolves.toContain('# tasteskill: Anti-Slop Frontend Skill');
    await expect(readFile(
      path.join(workspace, '.pi/skills/shadcn/SKILL.md'),
      'utf8',
    )).resolves.toContain('name: shadcn');
    await expect(readFile(
      path.join(workspace, '.pi/extensions/alvax-tools.ts'),
      'utf8',
    )).resolves.toContain("name: 'alvax_request_confirmation'");
    await expect(readFile(
      path.join(workspace, '.pi/extensions/alvax-tools.ts'),
      'utf8',
    )).resolves.toContain("name: 'alvax_browser_inspect'");
  });
});
