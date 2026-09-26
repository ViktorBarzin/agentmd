import { describe, expect, it } from 'vitest';
import {
  EMPTY_FILTERS,
  countFindings,
  fileFindingCounts,
  filterFindings,
  findingHarnesses,
  findingsForFile,
  isCompare,
  sortFindings,
  spanForLine,
  type FindingFilters,
} from './findings';
import { ctx, file, finding } from './test-helpers';
import type { AgentFile, Finding, RuntimeContext } from './types';

const core = file('/home/alex/.agents/core.md', { display: '~/.agents/core.md', harnesses: [] });
const infra = file('/home/alex/code/infra/AGENTS.md', {
  display: '~/code/infra/AGENTS.md',
  harnesses: ['codex', 'agents-md'],
});
const webapp = file('/home/alex/code/webapp/AGENTS.md', {
  display: '~/code/webapp/AGENTS.md',
  harnesses: ['codex'],
});
const files = new Map<string, AgentFile>([core, infra, webapp].map((f) => [f.id, f]));
const contexts = new Map<string, RuntimeContext>(
  [ctx('claude:/home/alex/code/infra', []), ctx('codex:/home/alex/code/infra', [])].map((c) => [c.id, c]),
);

const dup = finding('dup', {
  severity: 'problem',
  kind: 'duplicate',
  summary: 'Five lines repeat',
  spans: [
    { fileId: infra.id, startLine: 120, endLine: 124 },
    { fileId: core.id, startLine: 10, endLine: 14, quote: 'Commit on a branch' },
  ],
  contexts: ['claude:/home/alex/code/infra', 'codex:/home/alex/code/infra'],
});
const contra = finding('contra', {
  severity: 'problem',
  kind: 'contradiction',
  source: 'analysis',
  summary: 'Ask before committing, or not',
  spans: [
    { fileId: core.id, startLine: 22, endLine: 22 },
    { fileId: infra.id, startLine: 8, endLine: 8 },
  ],
  contexts: ['claude:/home/alex/code/infra'],
});
const dangling = finding('dangling', {
  kind: 'dangling',
  summary: 'Mentions a missing runbook',
  spans: [{ fileId: infra.id, startLine: 88, endLine: 88 }],
});
const hintDup = finding('hint-dup', {
  kind: 'duplicate',
  summary: 'Same pull request rules',
  spans: [
    { fileId: webapp.id, startLine: 5, endLine: 9 },
    { fileId: webapp.id, startLine: 30, endLine: 34 },
  ],
});
const all: Finding[] = [hintDup, dangling, dup, contra];

describe('sortFindings', () => {
  it('puts problems first, then orders by kind, file and line', () => {
    expect(sortFindings(all, files).map((f) => f.id)).toEqual(['contra', 'dup', 'hint-dup', 'dangling']);
  });

  it('does not change its input', () => {
    const copy = [...all];
    sortFindings(all, files);
    expect(all).toEqual(copy);
  });
});

describe('countFindings', () => {
  it('counts by severity and by kind', () => {
    const c = countFindings(all);
    expect(c.problem).toBe(2);
    expect(c.hint).toBe(2);
    expect(c.byKind.duplicate).toBe(2);
    expect(c.byKind.contradiction).toBe(1);
    expect(c.byKind.budget).toBe(0);
  });
});

describe('findingHarnesses', () => {
  it('uses the contexts when a finding names them', () => {
    expect(findingHarnesses(dup, contexts, files)).toEqual(['claude', 'codex']);
  });

  it('falls back to the harnesses that load its files', () => {
    expect(findingHarnesses(dangling, contexts, files)).toEqual(['codex', 'agents-md']);
  });
});

describe('filterFindings', () => {
  const lookup = { contexts, files };

  it('keeps everything with empty filters', () => {
    expect(filterFindings(all, EMPTY_FILTERS, lookup)).toHaveLength(4);
  });

  it('ORs choices inside a group and ANDs across groups', () => {
    const f: FindingFilters = { ...EMPTY_FILTERS, kinds: ['duplicate', 'dangling'], severities: ['hint'] };
    expect(filterFindings(all, f, lookup).map((x) => x.id)).toEqual(['hint-dup', 'dangling']);
  });

  it('filters by harness and by source', () => {
    expect(filterFindings(all, { ...EMPTY_FILTERS, harnesses: ['claude'] }, lookup).map((x) => x.id)).toEqual([
      'dup',
      'contra',
    ]);
    expect(filterFindings(all, { ...EMPTY_FILTERS, sources: ['analysis'] }, lookup).map((x) => x.id)).toEqual([
      'contra',
    ]);
  });

  it('searches summaries, kinds, paths and quotes, case-insensitively', () => {
    const q = (text: string) => filterFindings(all, { ...EMPTY_FILTERS, text }, lookup).map((x) => x.id);
    expect(q('RUNBOOK')).toEqual(['dangling']);
    expect(q('webapp')).toEqual(['hint-dup']);
    expect(q('contradiction')).toEqual(['contra']);
    expect(q('commit on a branch')).toEqual(['dup']);
    expect(q('repeat core.md')).toEqual(['dup']);
  });

  it('limits to a context by its id or by the files that belong to it', () => {
    const scope = { contextId: 'codex:/home/alex/code/infra', members: new Set([infra.id]) };
    expect(filterFindings(all, EMPTY_FILTERS, lookup, scope).map((x) => x.id)).toEqual(['dangling', 'dup', 'contra']);
  });

  it('limits to files that match the header search', () => {
    const scope = { fileMatch: (id: string) => id.includes('webapp') };
    expect(filterFindings(all, EMPTY_FILTERS, lookup, scope).map((x) => x.id)).toEqual(['hint-dup']);
  });
});

describe('per-file helpers', () => {
  it('counts each finding once per file it touches', () => {
    const counts = fileFindingCounts(all);
    expect(counts.get(infra.id)).toEqual({ problem: 2, hint: 1 });
    expect(counts.get(core.id)).toEqual({ problem: 2, hint: 0 });
    expect(counts.get(webapp.id)).toEqual({ problem: 0, hint: 1 });
  });

  it('lists the findings for one file', () => {
    expect(findingsForFile(all, core.id).map((f) => f.id)).toEqual(['dup', 'contra']);
  });

  it('finds the span that holds a line', () => {
    expect(spanForLine(all, infra.id, 122)).toEqual({ fileId: infra.id, startLine: 120, endLine: 124 });
    expect(spanForLine(all, infra.id, 125)).toBeUndefined();
    expect(spanForLine(all, webapp.id, 31)).toEqual({ fileId: webapp.id, startLine: 30, endLine: 34 });
  });

  it('opens compare mode for two spans, and the editor for one', () => {
    expect(isCompare(dup)).toBe(true);
    expect(isCompare(hintDup)).toBe(true);
    expect(isCompare(dangling)).toBe(false);
    const sameSpanTwice = finding('x', {
      spans: [
        { fileId: 'a', startLine: 1, endLine: 2 },
        { fileId: 'a', startLine: 1, endLine: 2 },
      ],
    });
    expect(isCompare(sameSpanTwice)).toBe(false);
  });
});
