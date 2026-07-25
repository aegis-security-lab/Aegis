import { mkdtemp, readFile, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { afterEach, describe, expect, it } from 'vitest';
import { installBundledSkills, writeWebsiteStarter } from './starter';

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
      name: 'Nova',
      industry: '企业服务',
      offering: 'AI 客户支持',
      audience: '客户成功团队',
      purposes: ['brand'],
      notes: '',
    });
    const skillPath = '.pi/skills/design-taste-frontend/SKILL.md';
    const content = await readFile(path.join(workspace, skillPath), 'utf8');

    expect(content).toContain('name: design-taste-frontend');
    expect(artifacts.some((artifact) => artifact.path === skillPath)).toBe(true);
    const shadcnPath = '.pi/skills/shadcn/SKILL.md';
    await expect(readFile(path.join(workspace, shadcnPath), 'utf8')).resolves.toContain('name: shadcn');
    expect(artifacts.some((artifact) => artifact.path === shadcnPath)).toBe(true);
    await expect(readFile(path.join(workspace, '.pi/skills/shadcn/rules/styling.md'), 'utf8')).resolves.toContain('Tailwind');
  });

  it('repairs the bundled skill in an existing workspace', async () => {
    const workspace = await mkdtemp(path.join(tmpdir(), 'alvax-skill-'));
    temporaryDirectories.push(workspace);

    await installBundledSkills(workspace);

    await expect(readFile(
      path.join(workspace, '.pi/skills/design-taste-frontend/SKILL.md'),
      'utf8',
    )).resolves.toContain('# tasteskill: Anti-Slop Frontend Skill');
    await expect(readFile(
      path.join(workspace, '.pi/skills/shadcn/SKILL.md'),
      'utf8',
    )).resolves.toContain('name: shadcn');
  });
});
