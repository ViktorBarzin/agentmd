import { beforeEach, describe, expect, it } from 'vitest';
import type { FileResponse, GitResponse, Job, SaveResponse, State } from '../types';
import { CTX, P } from './fixture';
import { MockServer } from './server';

let t = Date.parse('2026-09-26T12:00:00Z');
let server: MockServer;

beforeEach(() => {
  t = Date.parse('2026-09-26T12:00:00Z');
  server = new MockServer(() => t);
});

function call<T>(method: string, path: string, body?: unknown, query = ''): { status: number; body: T } {
  const res = server.handle(method, path, new URLSearchParams(query), body);
  return { status: res.status, body: res.body as T };
}

const getFile = (id: string) => call<FileResponse>('GET', '/file', undefined, `id=${encodeURIComponent(id)}`).body;
const getState = () => call<State>('GET', '/state').body;

describe('MockServer files', () => {
  it('serves a file and its content', () => {
    const res = getFile(P.codeAgents);
    expect(res.file.id).toBe(P.codeAgents);
    expect(res.content.startsWith('# ~/code')).toBe(true);
  });

  it('answers 404 for an unknown file', () => {
    expect(call('GET', '/file', undefined, 'id=/nope').status).toBe(404);
  });

  it('saves, updates the hash and returns the git diff', () => {
    const before = getFile(P.webappAgents);
    const content = before.content.replace('ten minutes', 'fifteen minutes');
    const res = call<SaveResponse>('PUT', '/file', { id: P.webappAgents, content, baseHash: before.file.hash });
    expect(res.status).toBe(200);
    expect(res.body.file.hash).not.toBe(before.file.hash);
    expect(res.body.repo).toBe('/home/alex/code/webapp');
    expect(res.body.diff).toContain('-- Keep each pull request to one change that someone can review in ten minutes.');
    expect(res.body.diff).toContain('+- Keep each pull request to one change that someone can review in fifteen minutes.');
    expect(getFile(P.webappAgents).content).toBe(content);
    const git = call<GitResponse>('GET', '/git', undefined, `id=${P.webappAgents}`).body;
    expect(git.dirty).toBe(true);
  });

  it('answers 409 with the current content when the base hash is stale', () => {
    const before = getFile(P.tripitAgents);
    server.touch(P.tripitAgents);
    const res = call<{ error: string; current: string; hash: string }>('PUT', '/file', {
      id: P.tripitAgents,
      content: 'mine',
      baseHash: before.file.hash,
    });
    expect(res.status).toBe(409);
    expect(res.body.current).toContain('Added in another editor');
    expect(res.body.hash).toBe(getFile(P.tripitAgents).file.hash);
  });

  it('refuses to save a read-only file', () => {
    const f = getFile(P.orgClaude);
    const res = call<{ error: string }>('PUT', '/file', { id: P.orgClaude, content: 'x', baseHash: f.file.hash });
    expect(res.status).toBe(403);
    expect(res.body.error).toContain('read-only');
  });

  it('writes through a link to its target', () => {
    const link = getFile(P.infraClaude);
    const content = link.content.replace('# infra', '# infra repository');
    const res = call<SaveResponse>('PUT', '/file', { id: P.infraClaude, content, baseHash: link.file.hash });
    expect(res.status).toBe(200);
    expect(getFile(P.infraAgents).content.startsWith('# infra repository')).toBe(true);
    expect(getFile(P.infraAgents).file.hash).toBe(getFile(P.infraClaude).file.hash);
  });

  it('moves findings with their text and drops them when the text is gone', () => {
    const infra = getFile(P.infraAgents);
    const content = '# A new first line\n' + infra.content;
    call('PUT', '/file', { id: P.infraAgents, content, baseHash: infra.file.hash });
    const dup = getState().findings.find((f) => f.id === 'dup-core-infra');
    expect(dup?.spans[1]).toMatchObject({ startLine: 121, endLine: 125 });

    const now = getFile(P.infraAgents);
    const without = now.content.replace('For the pre-2025 incident flow, see docs/agents/old-runbook.md.\n', '');
    call('PUT', '/file', { id: P.infraAgents, content: without, baseHash: now.file.hash });
    expect(getState().findings.some((f) => f.id === 'dangling-old-runbook')).toBe(false);
  });

  it('marks the analysis stale when a loaded file changes', () => {
    const core = getFile(P.core);
    call('PUT', '/file', { id: P.core, content: core.content + '\n- One more rule.\n', baseHash: core.file.hash });
    const ctx = getState().contexts.find((c) => c.id === CTX.claudeInfra);
    expect(ctx?.analysis?.stale).toBe(true);
  });
});

describe('MockServer commits', () => {
  it('commits a changed file, then reports it clean', () => {
    const f = getFile(P.webappAgents);
    call('PUT', '/file', { id: P.webappAgents, content: f.content + '\nMore.\n', baseHash: f.file.hash });
    const res = call<{ repo: string; commit: string; output: string }>('POST', '/commit', {
      ids: [P.webappAgents],
      message: 'Add a closing line',
    });
    expect(res.status).toBe(200);
    expect(res.body.commit).toMatch(/^[0-9a-f]{40}$/);
    expect(res.body.output).toContain('Add a closing line');
    expect(call<GitResponse>('GET', '/git', undefined, `id=${P.webappAgents}`).body.dirty).toBe(false);
  });

  it('reports hook failures, empty messages and clean files as errors', () => {
    const f = getFile(P.webappAgents);
    call('PUT', '/file', { id: P.webappAgents, content: f.content + 'x\n', baseHash: f.file.hash });
    expect(call<{ error: string }>('POST', '/commit', { ids: [P.webappAgents], message: 'wip stuff' }).body.error).toContain(
      'commit-msg hook',
    );
    expect(call('POST', '/commit', { ids: [P.webappAgents], message: '  ' }).status).toBe(400);
    expect(call('POST', '/commit', { ids: [P.tripitAgents], message: 'Nothing' }).status).toBe(400);
    expect(call('POST', '/commit', { ids: [P.core], message: 'Not in git' }).status).toBe(400);
  });
});

describe('MockServer pushes', () => {
  type Git = GitResponse;
  const gitOf = (id: string) => call<Git>('GET', '/git', undefined, `id=${encodeURIComponent(id)}`).body;
  const editAndCommit = (id: string) => {
    const f = getFile(id);
    call('PUT', '/file', { id, content: f.content + '\nOne more line.\n', baseHash: f.file.hash });
    return call<{ commit: string; output: string }>('POST', '/commit', { ids: [id], message: 'Add a line' });
  };

  it('reports the branch, its upstream and how far apart they are', () => {
    expect(gitOf(P.infraAgents)).toMatchObject({ branch: 'main', upstream: 'origin/main', ahead: 0, behind: 0 });
    expect(gitOf(P.webappAgents)).toMatchObject({ branch: 'pr-rules', upstream: '' });
    expect(gitOf(P.core)).toMatchObject({ repo: '', branch: '', upstream: '', ahead: 0, behind: 0 });
  });

  it('fast-forwards the upstream after a commit', () => {
    const c = editAndCommit(P.infraAgents);
    expect(c.body.output.startsWith('[main ')).toBe(true);
    expect(gitOf(P.infraAgents).ahead).toBe(1);
    const res = call<{ branch: string; upstream: string; output: string }>('POST', '/push', { id: P.infraClaude });
    expect(res.status).toBe(200);
    expect(res.body).toMatchObject({ branch: 'main', upstream: 'origin/main' });
    expect(res.body.output).toContain(`${c.body.commit.slice(0, 7)}  main -> main`);
    expect(gitOf(P.infraAgents).ahead).toBe(0);
  });

  it('passes on git refusing a push that is not a fast-forward', () => {
    editAndCommit(P.tripitAgents);
    const res = call<{ error: string }>('POST', '/push', { id: P.tripitAgents });
    expect(res.status).toBe(409);
    expect(res.body.error).toContain('[rejected]        main -> main (non-fast-forward)');
  });

  it('refuses when there is no upstream or nothing to push', () => {
    editAndCommit(P.webappAgents);
    expect(call('POST', '/push', { id: P.webappAgents }).status).toBe(409);
    expect(call('POST', '/push', { id: P.codeAgents }).status).toBe(409);
    expect(call('POST', '/push', { id: P.core }).status).toBe(400);
  });
});

describe('MockServer at scale', () => {
  it('grows to about 300 files and 500 references with consistent ids', () => {
    const big = new MockServer(() => t, 70);
    const s = big.handle('GET', '/state', new URLSearchParams(), undefined).body as State;
    const ids = new Set(s.files.map((f) => f.id));
    expect(s.files.length).toBeGreaterThanOrEqual(300);
    const loadRefs = s.contexts.reduce((n, c) => n + Math.max(0, c.entries.length - 1), 0);
    expect(s.refs.length + loadRefs).toBeGreaterThanOrEqual(500);
    expect(ids.size).toBe(s.files.length);
    for (const r of s.refs) expect(ids.has(r.from) && (r.dangling || ids.has(r.to)), `${r.from} -> ${r.to}`).toBe(true);
    for (const c of s.contexts) for (const e of c.entries) expect(ids.has(e.fileId), e.fileId).toBe(true);
    const file = big.handle('GET', '/file', new URLSearchParams({ id: '/home/alex/code/lab/p010/CLAUDE.md' }), undefined);
    expect((file.body as FileResponse).content).toContain('# p010');
  });
});

describe('MockServer probes', () => {
  it('lists unprobed directories apart from the contexts', () => {
    const s = getState();
    expect(s.unprobed?.map((c) => `${c.harness}:${c.dir}`).sort()).toEqual([CTX.claudeFrontend, CTX.codexWebapp].sort());
    expect(s.contexts.some((c) => c.id === CTX.claudeFrontend || c.id === CTX.codexWebapp)).toBe(false);
  });

  const runProbe = (contexts: string[]) => {
    const job = call<Job>('POST', '/probe', { contexts }).body;
    expect(job).toMatchObject({ kind: 'probe', status: 'running', done: 0 });
    t += 1600;
    const mid = call<Job>('GET', `/jobs/${job.id}`).body;
    t += 1500;
    const end = call<Job>('GET', `/jobs/${job.id}`).body;
    return { job, mid, end, state: getState() };
  };

  it('turns one candidate into a probed context through a job', () => {
    const { job, mid, end, state: s } = runProbe([CTX.claudeFrontend]);
    expect(job.total).toBe(1);
    expect(mid.status).toBe('running');
    expect(end).toMatchObject({ status: 'done', done: 1, total: 1 });
    const c = s.contexts.find((x) => x.id === CTX.claudeFrontend);
    expect(c).toMatchObject({ source: 'probe', harness: 'claude', probedAt: new Date(t).toISOString() });
    expect(c?.entries.map((e) => e.fileId)).toEqual([P.orgClaude, P.userClaude, P.codeClaude]);
    expect(c?.skipped?.map((e) => e.fileId)).toEqual([P.webappAgents, P.webappFrontend]);
    expect(c?.skipped?.[0].reason).toContain('CLAUDE.md');
    expect(s.unprobed?.map((x) => x.dir)).toEqual(['/home/alex/code/webapp']);
  });

  it('probes every unprobed and stale context when the list is empty, counting up', () => {
    const { job, mid, end, state: s } = runProbe([]);
    expect(job.total).toBe(3);
    expect(mid.done).toBe(1);
    expect(end).toMatchObject({ status: 'done', done: 3 });
    expect(s.unprobed).toEqual([]);
    expect(s.contexts.find((c) => c.id === CTX.codexTripit)?.stale).toBeUndefined();
    const failed = s.contexts.find((c) => c.id === CTX.codexWebapp);
    expect(failed?.error).toContain('exited with status 1');
    expect(failed?.entries).toEqual([]);
  });

  it('finishes at once when nothing needs probing, and refuses unknown ids', () => {
    runProbe([]);
    const job = call<Job>('POST', '/probe', { contexts: [] }).body;
    expect(job.total).toBe(0);
    expect(call<Job>('GET', `/jobs/${job.id}`).body.status).toBe('done');
    expect(call('POST', '/probe', { contexts: ['claude:/nowhere'] }).status).toBe(404);
  });

  it('marks a probed context stale when a file it loaded changes', () => {
    const f = getFile(P.tripitAgents);
    call('PUT', '/file', { id: P.tripitAgents, content: f.content + 'x\n', baseHash: f.file.hash });
    const s = getState();
    expect(s.contexts.find((c) => c.id === CTX.claudeTripit)?.stale).toBe(true);
    expect(s.contexts.find((c) => c.id === CTX.claudeInfra)?.stale).toBeUndefined();
  });
});

describe('MockServer jobs', () => {
  it('runs an analysis for about two seconds, then records it', () => {
    const job = call<Job>('POST', '/analyse', { context: CTX.claudeInfra }).body;
    expect(job.status).toBe('running');
    t += 1000;
    expect(call<Job>('GET', `/jobs/${job.id}`).body.status).toBe('running');
    t += 1500;
    const done = call<Job>('GET', `/jobs/${job.id}`).body;
    expect(done.status).toBe('done');
    expect(done.findings).toHaveLength(2);
    const ctx = getState().contexts.find((c) => c.id === CTX.claudeInfra);
    expect(ctx?.analysis?.at).toBe(new Date(t).toISOString());
    expect(getState().findings.filter((f) => f.source === 'analysis')).toHaveLength(2);
  });

  it('fails the analysis of one context, to show the error state', () => {
    const job = call<Job>('POST', '/analyse', { context: CTX.codexTripit }).body;
    t += 2500;
    const done = call<Job>('GET', `/jobs/${job.id}`).body;
    expect(done.status).toBe('error');
    expect(done.error).toContain('exited with status 1');
  });

  it('proposes removing the duplicate from infra/AGENTS.md, and applies it', () => {
    const job = call<Job>('POST', '/fix', { findingId: 'dup-core-infra' }).body;
    t += 2500;
    const done = call<Job>('GET', `/jobs/${job.id}`).body;
    expect(done.status).toBe('done');
    const edit = done.proposal?.edits[0];
    expect(edit?.fileId).toBe(P.infraAgents);
    expect(edit?.before).toContain('## Git\n\n- Commit on a branch');
    expect(edit?.after).not.toContain('## Git\n\n- Commit on a branch');
    expect((edit?.before.length ?? 0) - (edit?.after.length ?? 0)).toBeGreaterThan(200);

    const res = call<{ results: SaveResponse[] }>('POST', '/apply', {
      edits: [{ id: edit?.fileId, content: edit?.after, baseHash: edit?.baseHash }],
    });
    expect(res.status).toBe(200);
    const s = getState();
    expect(s.findings.some((f) => f.id === 'dup-core-infra')).toBe(false);
    const budget = s.findings.find((f) => f.id === 'budget-infra-codex');
    expect(budget?.summary).not.toContain('36,059');
  });

  it('refuses to apply over a file that changed after the proposal', () => {
    const job = call<Job>('POST', '/fix', { findingId: 'contra-commit' }).body;
    t += 2500;
    const edit = call<Job>('GET', `/jobs/${job.id}`).body.proposal?.edits[0];
    server.touch(P.personal);
    const res = call('POST', '/apply', { edits: [{ id: edit?.fileId, content: edit?.after, baseHash: edit?.baseHash }] });
    expect(res.status).toBe(409);
  });

  it('explains when it has no fix to propose', () => {
    const job = call<Job>('POST', '/fix', { findingId: 'budget-infra-codex' }).body;
    t += 2500;
    const done = call<Job>('GET', `/jobs/${job.id}`).body;
    expect(done.status).toBe('error');
    expect(done.error).toContain('No edit proposed');
  });

  it('turns analysis and fixes off when no CLI is installed', () => {
    server.setAnalysis('');
    expect(getState().analysis).toBeUndefined();
    expect(call('POST', '/analyse', { context: CTX.claudeInfra }).status).toBe(400);
    expect(call('POST', '/fix', { findingId: 'dup-core-infra' }).status).toBe(400);
  });

  it('records a probe time for the probed context only', () => {
    const job = call<Job>('POST', '/probe', { contexts: [CTX.claudeInfra] }).body;
    t += 3100;
    expect(call<Job>('GET', `/jobs/${job.id}`).body.status).toBe('done');
    for (const c of getState().contexts) {
      if (c.id === CTX.claudeInfra) expect(c.probedAt).toBe(new Date(t).toISOString());
      else if (c.source === 'probe') expect(c.probedAt).not.toBe(new Date(t).toISOString());
      else expect(c.probedAt).toBeUndefined();
    }
  });
});
