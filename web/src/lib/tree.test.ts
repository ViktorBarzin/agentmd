import { describe, expect, it } from 'vitest';
import { groupFiles, matchesQuery, rowLabel } from './tree';
import { file } from './test-helpers';

const home = '/home/alex';

const files = [
  file('/home/alex/code/infra/docs/agents/terraform.md', { kind: 'doc', repo: '/home/alex/code/infra' }),
  file('/home/alex/code/infra/AGENTS.md', { repo: '/home/alex/code/infra' }),
  file('/home/alex/code/infra/CLAUDE.md', { repo: '/home/alex/code/infra', isLink: true }),
  file('/home/alex/code/AGENTS.md', { repo: '/home/alex/code' }),
  file('/etc/claude-code/managed-settings.json#claudeMd', { scope: 'org', display: '/etc/claude-code/managed-settings.json#claudeMd' }),
  file('/home/alex/.claude/CLAUDE.md', { scope: 'user' }),
  file('/home/alex/.agents/skills/tdd/SKILL.md', {
    kind: 'skill',
    scope: 'user',
    name: 'tdd',
    access: { writable: true, managed: { tool: 'skills', source: 'github.com/example/skills', warn: true } },
  }),
  file('/home/alex/.agents/skills/publish-page/SKILL.md', { kind: 'skill', scope: 'user', name: 'publish-page' }),
  file('/home/alex/.claude/plugins/cache/docs-tools/skills/docs-lookup/SKILL.md', {
    kind: 'skill',
    scope: 'plugin',
    name: 'docs-lookup',
  }),
  file('/home/alex/code/tripit/.agents/skills/import/SKILL.md', {
    kind: 'skill',
    scope: 'project',
    name: 'import',
    repo: '/home/alex/code/tripit',
  }),
  file('/opt/elsewhere/notes.md', { kind: 'doc', scope: 'other', display: '/opt/elsewhere/notes.md' }),
];

describe('groupFiles', () => {
  const groups = groupFiles(files, home);

  it('orders org policy, user, skills by source, repositories, then the rest', () => {
    expect(groups.map((g) => g.label)).toEqual([
      'Org policy',
      'User',
      'Your skills',
      'Skills from github.com/example/skills',
      'Plugin skills',
      '~/code',
      '~/code/infra',
      '~/code/tripit',
      'Other',
    ]);
  });

  it('keeps project skills with their repository', () => {
    const tripit = groups.find((g) => g.id === 'repo:/home/alex/code/tripit');
    expect(tripit?.files.map((f) => f.name)).toEqual(['import']);
  });

  it('sorts a repository by depth, then path', () => {
    const infra = groups.find((g) => g.id === 'repo:/home/alex/code/infra');
    expect(infra?.files.map((f) => rowLabel(f, infra, home))).toEqual([
      'AGENTS.md',
      'CLAUDE.md',
      'docs/agents/terraform.md',
    ]);
  });

  it('can order members of a context by load position first', () => {
    const order = new Map([
      ['/home/alex/code/infra/CLAUDE.md', 1],
      ['/home/alex/code/infra/AGENTS.md', 2],
    ]);
    const infra = groupFiles(files, home, order).find((g) => g.id === 'repo:/home/alex/code/infra');
    expect(infra?.files.map((f) => f.id)).toEqual([
      '/home/alex/code/infra/CLAUDE.md',
      '/home/alex/code/infra/AGENTS.md',
      '/home/alex/code/infra/docs/agents/terraform.md',
    ]);
  });
});

describe('rowLabel', () => {
  it('shows a path relative to the repository, or the home-shortened path', () => {
    const groups = groupFiles(files, home);
    const user = groups.find((g) => g.id === 'user');
    expect(rowLabel(files[5], user, home)).toBe('~/.claude/CLAUDE.md');
    const org = groups.find((g) => g.id === 'org');
    expect(rowLabel(files[4], org, home)).toBe('/etc/claude-code/managed-settings.json#claudeMd');
  });
});

describe('matchesQuery', () => {
  it('matches every word against the path, name and description', () => {
    const tdd = files[6];
    expect(matchesQuery(tdd, '')).toBe(true);
    expect(matchesQuery(tdd, 'tdd')).toBe(true);
    expect(matchesQuery(tdd, 'SKILLS tdd')).toBe(true);
    expect(matchesQuery(tdd, 'tdd infra')).toBe(false);
    expect(matchesQuery(files[1], 'infra agents')).toBe(true);
  });
});
