import { describe, expect, it } from 'vitest';
import { contextMembers } from '../contexts';
import { buildFixture, CODEX_BUDGET, CTX, P } from './fixture';
import { countLines, hashText } from './util';

const fx = buildFixture(Date.parse('2026-09-26T12:00:00Z'));
const { state } = fx;
const files = new Map(state.files.map((f) => [f.id, f]));
const text = (id: string) => fx.contents.get(fx.links.get(id) ?? id) ?? '';
const lines = (id: string) => text(id).split('\n');

describe('fixture', () => {
  it('has about 25 files with unique ids and content that matches their metadata', () => {
    expect(state.files.length).toBeGreaterThanOrEqual(24);
    expect(files.size).toBe(state.files.length);
    for (const f of state.files) {
      const t = text(f.id);
      expect(t, f.id).not.toBe('');
      expect(f.hash, f.id).toBe(hashText(t));
      expect(f.lines, f.id).toBe(countLines(t));
    }
  });

  it('points every span at lines that exist', () => {
    for (const finding of state.findings) {
      for (const s of finding.spans) {
        const f = files.get(s.fileId);
        expect(f, `${finding.id} ${s.fileId}`).toBeDefined();
        expect(s.startLine).toBeGreaterThanOrEqual(1);
        expect(s.endLine).toBeGreaterThanOrEqual(s.startLine);
        expect(s.endLine, finding.id).toBeLessThanOrEqual(f?.lines ?? 0);
        if (s.quote) expect(lines(s.fileId).slice(s.startLine - 1, s.endLine).join('\n')).toContain(s.quote);
      }
    }
  });

  it('points every reference at known files, except the dangling one', () => {
    for (const r of state.refs) {
      expect(files.has(r.from), r.from).toBe(true);
      expect(files.has(r.to), r.to).toBe(!r.dangling);
      if (r.line !== undefined && r.text) {
        const line = lines(r.from)[r.line - 1] ?? '';
        const needle = r.text.replace(/^\[[^\]]*\]\(|\)$/g, '').replace(/`/g, '').replace(/ skill$/, '');
        expect(line, `${r.from}:${r.line}`).toContain(needle);
      }
    }
    expect(state.refs.filter((r) => r.dangling).map((r) => [r.to, r.line])).toEqual([[P.oldRunbook, 88]]);
    expect(state.refs.some((r) => r.kind === 'load')).toBe(false);
  });

  it('refers only to files and contexts that exist', () => {
    const ctxIds = new Set(state.contexts.map((c) => c.id));
    for (const c of [...state.contexts, ...fx.probeable.values()]) {
      for (const e of [...c.entries, ...(c.skipped ?? [])]) expect(files.has(e.fileId), `${c.id} ${e.fileId}`).toBe(true);
      for (const e of c.skipped ?? []) expect(e.reason, e.fileId).toBeTruthy();
      for (const i of [...c.skills, ...c.subagents]) if (i.fileId) expect(files.has(i.fileId), i.fileId).toBe(true);
    }
    for (const f of state.findings) for (const c of f.contexts ?? []) expect(ctxIds.has(c), c).toBe(true);
  });

  it('makes infra/AGENTS.md exactly 3,291 bytes over the Codex budget', () => {
    const infra = files.get(P.infraAgents);
    expect(infra?.size).toBe(CODEX_BUDGET + 3291);
    const entry = state.contexts.find((c) => c.id === CTX.codexInfra)?.entries.find((e) => e.fileId === P.infraAgents);
    expect(entry).toMatchObject({ truncated: true, lostBytes: 3291, bytes: CODEX_BUDGET });
  });

  it('repeats core.md lines 10-14 at infra/AGENTS.md lines 120-124', () => {
    expect(lines(P.core).slice(9, 14)).toEqual(lines(P.infraAgents).slice(119, 124));
    expect(lines(P.webappAgents).slice(4, 9)).toEqual(lines(P.tripitAgents).slice(6, 11));
  });

  it('builds ~/.agents/AGENTS.md from its parts, and links both user files to it', () => {
    const hub = files.get(P.hub);
    expect(hub?.access).toMatchObject({ writable: false, builtFrom: [P.core, P.personal], builtBy: 'agents-sync' });
    expect(text(P.hub)).toContain(text(P.core));
    expect(text(P.hub)).toContain(text(P.personal));
    for (const id of [P.userClaude, P.userCodex]) {
      expect(files.get(id)).toMatchObject({ isLink: true, realPath: P.hub, hash: hub?.hash });
    }
  });

  it('marks access the way the plan describes', () => {
    expect(files.get(P.orgClaude)?.access).toMatchObject({ writable: false, owner: 'root', origin: { fileId: P.origin } });
    expect(files.get(P.origin)?.access.writable).toBe(true);
    expect(files.get(P.core)?.access.managed).toMatchObject({ tool: 'chezmoi' });
    expect(files.get(P.core)?.access.managed?.warn).toBeUndefined();
    expect(files.get(P.tdd)?.access.managed).toMatchObject({ tool: 'skills', warn: true });
    expect(files.get(P.docsLookup)?.access.writable).toBe(false);
  });

  it('derives which harnesses load each file from the contexts', () => {
    expect(files.get(P.infraAgents)?.harnesses).toEqual(['claude', 'codex', 'agents-md']);
    expect(files.get(P.webappAgents)?.harnesses).toEqual(['codex', 'agents-md']);
    expect(files.get(P.hub)?.harnesses).toEqual(['claude', 'codex']);
    expect(files.get(P.terraform)?.harnesses).toEqual([]);
  });

  it('leaves webapp/AGENTS.md out of the Claude Code context in webapp, saying why', () => {
    const c = state.contexts.find((x) => x.id === CTX.claudeWebapp);
    expect(c?.entries.map((e) => e.fileId)).toEqual([P.orgClaude, P.userClaude, P.codeClaude]);
    expect(c?.skipped?.map((e) => e.fileId)).toEqual([P.webappAgents]);
    expect(contextMembers(c ?? state.contexts[0], state.refs).get(P.webappAgents)?.role).toBe('skipped');
  });

  it('has four problems and three hints, two of them from analysis', () => {
    const problems = state.findings.filter((f) => f.severity === 'problem');
    expect(problems.map((f) => f.kind).sort()).toEqual(['budget', 'contradiction', 'duplicate', 'reworded']);
    expect(state.findings.filter((f) => f.severity === 'hint').map((f) => f.kind).sort()).toEqual([
      'dangling',
      'duplicate',
      'not-loaded',
    ]);
    expect(state.findings.filter((f) => f.source === 'analysis')).toHaveLength(2);
    expect(state.contexts.find((c) => c.id === CTX.claudeInfra)?.analysis).toMatchObject({ cli: 'claude', findings: 2 });
  });
});
