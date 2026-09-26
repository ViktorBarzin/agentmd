// Sorting, filtering and counting findings.
import type { AgentFile, Finding, FindingKind, RuntimeContext, Severity, Span } from './types';

export const KIND_ORDER: FindingKind[] = ['contradiction', 'duplicate', 'reworded', 'budget', 'dangling', 'not-loaded'];

export const KIND_LABELS: Record<FindingKind, string> = {
  contradiction: 'Contradiction',
  duplicate: 'Duplicate',
  reworded: 'Reworded duplicate',
  budget: 'Over budget',
  dangling: 'Dangling reference',
  'not-loaded': 'Not loaded',
};

function kindRank(k: FindingKind): number {
  const i = KIND_ORDER.indexOf(k);
  return i === -1 ? KIND_ORDER.length : i;
}

function kindLabel(k: FindingKind): string {
  return KIND_LABELS[k] ?? k;
}

/** Problems first, then by kind, then by the first span's file and line. */
export function sortFindings(findings: Finding[], files: Map<string, AgentFile>): Finding[] {
  const key = (f: Finding) => {
    const s = f.spans[0];
    return { path: s ? (files.get(s.fileId)?.display ?? s.fileId) : '', line: s?.startLine ?? 0 };
  };
  return [...findings].sort((a, b) => {
    if (a.severity !== b.severity) return a.severity === 'problem' ? -1 : 1;
    const k = kindRank(a.kind) - kindRank(b.kind);
    if (k !== 0) return k;
    const ka = key(a);
    const kb = key(b);
    if (ka.path !== kb.path) return ka.path.localeCompare(kb.path);
    return ka.line - kb.line;
  });
}

export interface FindingCountSummary {
  problem: number;
  hint: number;
  byKind: Record<FindingKind, number>;
}

export function countFindings(findings: Finding[]): FindingCountSummary {
  const byKind = Object.fromEntries(KIND_ORDER.map((k) => [k, 0])) as Record<FindingKind, number>;
  let problem = 0;
  let hint = 0;
  for (const f of findings) {
    if (f.severity === 'problem') problem++;
    else hint++;
    byKind[f.kind] = (byKind[f.kind] ?? 0) + 1;
  }
  return { problem, hint, byKind };
}

/** The harnesses a finding concerns: from its contexts, or else from its files. */
export function findingHarnesses(
  f: Finding,
  contexts: Map<string, RuntimeContext>,
  files: Map<string, AgentFile>,
): string[] {
  const out: string[] = [];
  const add = (h: string) => {
    if (!out.includes(h)) out.push(h);
  };
  if (f.contexts && f.contexts.length > 0) {
    for (const id of f.contexts) {
      const c = contexts.get(id);
      add(c ? c.harness : id.slice(0, Math.max(0, id.indexOf(':'))));
    }
    return out.filter(Boolean);
  }
  for (const s of f.spans) for (const h of files.get(s.fileId)?.harnesses ?? []) add(h);
  return out;
}

export interface FindingFilters {
  severities: Severity[];
  kinds: FindingKind[];
  harnesses: string[];
  sources: Array<Finding['source']>;
  text: string;
}

export const EMPTY_FILTERS: FindingFilters = { severities: [], kinds: [], harnesses: [], sources: [], text: '' };

export interface FindingScope {
  /** Keep findings that name this context... */
  contextId?: string;
  /** ...or that touch one of these files. */
  members?: Set<string>;
  /** Keep findings that touch a file matching the header search. */
  fileMatch?: (fileId: string) => boolean;
}

export interface FindingLookup {
  contexts: Map<string, RuntimeContext>;
  files: Map<string, AgentFile>;
}

function haystack(f: Finding, files: Map<string, AgentFile>): string {
  const parts = [f.summary, f.detail ?? '', kindLabel(f.kind), f.kind, f.source];
  for (const s of f.spans) {
    parts.push(files.get(s.fileId)?.display ?? s.fileId, s.quote ?? '');
  }
  return parts.join('\n').toLowerCase();
}

export function filterFindings(
  findings: Finding[],
  filters: FindingFilters,
  lookup: FindingLookup,
  scope: FindingScope = {},
): Finding[] {
  const terms = filters.text.toLowerCase().split(/\s+/).filter(Boolean);
  return findings.filter((f) => {
    if (filters.severities.length && !filters.severities.includes(f.severity)) return false;
    if (filters.kinds.length && !filters.kinds.includes(f.kind)) return false;
    if (filters.sources.length && !filters.sources.includes(f.source)) return false;
    if (filters.harnesses.length) {
      const hs = findingHarnesses(f, lookup.contexts, lookup.files);
      if (!filters.harnesses.some((h) => hs.includes(h))) return false;
    }
    if (scope.contextId !== undefined || scope.members !== undefined) {
      const named = scope.contextId !== undefined && (f.contexts ?? []).includes(scope.contextId);
      const touches = scope.members !== undefined && f.spans.some((s) => scope.members?.has(s.fileId));
      if (!named && !touches) return false;
    }
    if (scope.fileMatch && !f.spans.some((s) => scope.fileMatch?.(s.fileId))) return false;
    if (terms.length) {
      const h = haystack(f, lookup.files);
      if (!terms.every((t) => h.includes(t))) return false;
    }
    return true;
  });
}

export interface FileCounts {
  problem: number;
  hint: number;
}

/** Finding counts per file. A finding counts once per file it touches. */
export function fileFindingCounts(findings: Finding[]): Map<string, FileCounts> {
  const out = new Map<string, FileCounts>();
  for (const f of findings) {
    for (const id of new Set(f.spans.map((s) => s.fileId))) {
      const c = out.get(id) ?? { problem: 0, hint: 0 };
      c[f.severity === 'problem' ? 'problem' : 'hint']++;
      out.set(id, c);
    }
  }
  return out;
}

export function findingsForFile(findings: Finding[], fileId: string): Finding[] {
  return findings.filter((f) => f.spans.some((s) => s.fileId === fileId));
}

/** The first finding span in a file that holds the given line. */
export function spanForLine(findings: Finding[], fileId: string, line: number): Span | undefined {
  for (const f of findings) {
    for (const s of f.spans) {
      if (s.fileId === fileId && s.startLine <= line && line <= s.endLine) return s;
    }
  }
  return undefined;
}

/** Two different spans open side by side; one span opens the editor. */
export function isCompare(f: Finding): boolean {
  if (f.spans.length < 2) return false;
  const [a, b] = f.spans;
  return a.fileId !== b.fileId || a.startLine !== b.startLine || a.endLine !== b.endLine;
}
