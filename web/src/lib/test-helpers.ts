// Builders for small hand-made states in unit tests.
import type { AgentFile, Finding, Ref, RuntimeContext, State } from './types';

export function file(id: string, extra: Partial<AgentFile> = {}): AgentFile {
  const path = id.includes('#') ? id.slice(0, id.indexOf('#')) : id;
  return {
    id,
    path,
    realPath: path,
    display: id.replace(/^\/home\/alex/, '~'),
    kind: 'instruction',
    scope: 'project',
    harnesses: [],
    size: 100,
    lines: 10,
    hash: 'h-' + id,
    access: { writable: true },
    ...extra,
  };
}

export function ctx(
  id: string,
  entries: string[],
  extra: Partial<RuntimeContext> = {},
): RuntimeContext {
  const colon = id.indexOf(':');
  const dir = id.slice(colon + 1);
  return {
    id,
    harness: id.slice(0, colon),
    dir,
    display: dir.replace(/^\/home\/alex/, '~'),
    source: 'probe',
    entries: entries.map((fileId) => ({ fileId, bytes: 100 })),
    skills: [],
    subagents: [],
    bytes: entries.length * 100,
    ...extra,
  };
}

export function finding(id: string, extra: Partial<Finding> = {}): Finding {
  return {
    id,
    kind: 'duplicate',
    severity: 'hint',
    summary: 'summary of ' + id,
    spans: [],
    source: 'rule',
    ...extra,
  };
}

export function ref(from: string, to: string, kind: Ref['kind'], extra: Partial<Ref> = {}): Ref {
  return { from, to, kind, ...extra };
}

export function state(extra: Partial<State> = {}): State {
  return {
    owner: 'alex',
    home: '/home/alex',
    roots: ['/home/alex/code'],
    scannedAt: '2026-09-26T12:00:00Z',
    harnesses: [
      { name: 'claude', label: 'Claude Code', available: true, version: '2.1.283', probe: true },
      { name: 'codex', label: 'Codex', available: true, version: '0.157.1', probe: true },
      { name: 'agents-md', label: 'AGENTS.md', available: true, probe: false },
    ],
    files: [],
    refs: [],
    contexts: [],
    findings: [],
    ...extra,
  };
}
