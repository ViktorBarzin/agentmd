import { describe, expect, it } from 'vitest';
import {
  analysisStatus,
  candidateId,
  contextMembers,
  contextsLoading,
  groupContexts,
  loadRefs,
} from './contexts';
import { ctx, ref, state } from './test-helpers';
import type { Job } from './types';

const org = '/etc/claude-code/managed-settings.json#claudeMd';
const userLink = '/home/alex/.claude/CLAUDE.md';
const hub = '/home/alex/.agents/AGENTS.md';
const core = '/home/alex/.agents/core.md';
const personal = '/home/alex/.agents/personal.md';
const origin = '/home/alex/code/infra/workstation/managed-settings.json#claudeMd';
const infraLink = '/home/alex/code/infra/CLAUDE.md';
const infra = '/home/alex/code/infra/AGENTS.md';
const skill = '/home/alex/.claude/skills/tdd/SKILL.md';
const skillTarget = '/home/alex/.agents/skills/tdd/SKILL.md';
const agent = '/home/alex/code/infra/.claude/agents/sev-triage.md';

const refs = [
  ref(userLink, hub, 'symlink', { sub: 'link' }),
  ref(infraLink, infra, 'symlink', { sub: 'link' }),
  ref(skill, skillTarget, 'symlink', { sub: 'dir' }),
  ref(hub, core, 'build', { sub: 'header' }),
  ref(hub, personal, 'build', { sub: 'header' }),
  ref(org, origin, 'build', { sub: 'origin' }),
  ref(infra, '/home/alex/code/infra/docs/agents/terraform.md', 'mention', { sub: 'path', line: 4 }),
];

const claudeInfra = ctx('claude:/home/alex/code/infra', [org, userLink, infraLink], {
  skills: [{ name: 'tdd', fileId: skill }, { name: 'builtin-thing' }],
  subagents: [{ name: 'sev-triage', fileId: agent }],
});

describe('loadRefs', () => {
  it('links each entry to the next one, numbered from 1', () => {
    const got = loadRefs([claudeInfra, ctx('agents-md:/x', ['/x/AGENTS.md'])]);
    expect(got).toEqual([
      { from: org, to: userLink, kind: 'load', context: claudeInfra.id, step: 1 },
      { from: userLink, to: infraLink, kind: 'load', context: claudeInfra.id, step: 2 },
    ]);
  });
});

describe('contextMembers', () => {
  const members = contextMembers(claudeInfra, refs);

  it('numbers entries by load position', () => {
    expect(members.get(org)).toEqual({ role: 'entry', position: 1 });
    expect(members.get(userLink)).toEqual({ role: 'entry', position: 2 });
    expect(members.get(infraLink)).toEqual({ role: 'entry', position: 3 });
  });

  it('follows symlinks to the file that is really read', () => {
    expect(members.get(hub)).toEqual({ role: 'link-target', position: 2, via: userLink });
    expect(members.get(infra)).toEqual({ role: 'link-target', position: 3, via: infraLink });
  });

  it('adds the parts of built files and the origins of installed copies', () => {
    expect(members.get(core)).toEqual({ role: 'source', position: 2, via: hub });
    expect(members.get(personal)).toEqual({ role: 'source', position: 2, via: hub });
    expect(members.get(origin)).toEqual({ role: 'source', position: 1, via: org });
  });

  it('adds skills and subagents that have a file, and their link targets', () => {
    expect(members.get(skill)).toEqual({ role: 'skill' });
    expect(members.get(skillTarget)).toEqual({ role: 'skill', via: skill });
    expect(members.get(agent)).toEqual({ role: 'subagent' });
  });

  it('does not follow text mentions', () => {
    expect(members.has('/home/alex/code/infra/docs/agents/terraform.md')).toBe(false);
    expect(members.size).toBe(11);
  });

  it('survives a symlink cycle', () => {
    const loop = [ref('/a', '/b', 'symlink'), ref('/b', '/a', 'symlink')];
    const m = contextMembers(ctx('claude:/', ['/a']), loop);
    expect([...m.keys()].sort()).toEqual(['/a', '/b']);
  });
});

describe('contextsLoading', () => {
  it('lists every context that loads a file, with its position', () => {
    const codex = ctx('codex:/home/alex/code/infra', ['/etc/codex/requirements.toml#x', infra]);
    const got = contextsLoading(infra, [claudeInfra, codex]);
    expect(got.map((c) => [c.context.id, c.position])).toEqual([['codex:/home/alex/code/infra', 2]]);
  });
});

describe('groupContexts', () => {
  it('groups by harness in the order the state lists harnesses, sorted by directory', () => {
    const s = state({
      contexts: [
        ctx('codex:/home/alex/code/infra', []),
        ctx('claude:/home/alex/code/webapp', []),
        ctx('claude:/home/alex', []),
        ctx('pi:/home/alex', [], { source: 'static' }),
        ctx('agents-md:/home/alex/code/infra', [], { source: 'static' }),
      ],
    });
    const groups = groupContexts(s);
    expect(groups.map((g) => [g.harness, g.contexts.map((c) => c.dir)])).toEqual([
      ['claude', ['/home/alex', '/home/alex/code/webapp']],
      ['codex', ['/home/alex/code/infra']],
      ['agents-md', ['/home/alex/code/infra']],
      ['pi', ['/home/alex']],
    ]);
    expect(groups[0].label).toBe('Claude Code');
    expect(groups[3].label).toBe('pi');
    expect(groups.every((g) => g.unprobed.length === 0)).toBe(true);
  });

  it('lists directories that are not probed yet under their harness', () => {
    const s = state({
      contexts: [ctx('claude:/home/alex', [])],
      unprobed: [
        { harness: 'codex', dir: '/home/alex/code/webapp', display: '~/code/webapp' },
        { harness: 'claude', dir: '/home/alex/code/webapp/frontend', display: '~/code/webapp/frontend' },
        { harness: 'claude', dir: '/home/alex/code', display: '~/code' },
      ],
    });
    const groups = groupContexts(s);
    expect(groups.map((g) => [g.harness, g.contexts.length, g.unprobed.map((c) => c.dir)])).toEqual([
      ['claude', 1, ['/home/alex/code', '/home/alex/code/webapp/frontend']],
      ['codex', 0, ['/home/alex/code/webapp']],
    ]);
    expect(candidateId(groups[1].unprobed[0])).toBe('codex:/home/alex/code/webapp');
  });
});

describe('analysisStatus', () => {
  const job = (status: Job['status'], extra: Partial<Job> = {}): Job => ({
    id: 'j1',
    kind: 'analyse',
    status,
    context: claudeInfra.id,
    startedAt: '2026-09-26T11:00:00Z',
    ...extra,
  });

  it('is never when there is no analysis and no job', () => {
    expect(analysisStatus(claudeInfra)).toEqual({ state: 'never' });
  });

  it('reports the last analysis, and whether it is stale', () => {
    const done = { ...claudeInfra, analysis: { at: '2026-09-26T09:00:00Z', cli: 'claude', findings: 2 } };
    expect(analysisStatus(done)).toEqual({ state: 'done', at: '2026-09-26T09:00:00Z', cli: 'claude', findings: 2 });
    const stale = { ...done, analysis: { ...done.analysis, stale: true } };
    expect(analysisStatus(stale)).toMatchObject({ state: 'stale', findings: 2 });
  });

  it('prefers a running or failed job over the stored analysis', () => {
    const done = { ...claudeInfra, analysis: { at: '2026-09-26T09:00:00Z', cli: 'claude', findings: 2 } };
    expect(analysisStatus(done, job('running'))).toEqual({ state: 'running', since: '2026-09-26T11:00:00Z' });
    expect(analysisStatus(done, job('error', { error: 'claude exited 1' }))).toEqual({
      state: 'error',
      message: 'claude exited 1',
    });
  });

  it('falls back to the finished job when the state has not caught up yet', () => {
    const finished = job('done', { finishedAt: '2026-09-26T11:00:02Z', findings: [] });
    expect(analysisStatus(claudeInfra, finished)).toEqual({
      state: 'done',
      at: '2026-09-26T11:00:02Z',
      cli: '',
      findings: 0,
    });
  });
});
