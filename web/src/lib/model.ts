// Lookup tables built once per state, so the views stay fast with hundreds
// of files and references.
import type { Loading } from './contexts';
import { fileFindingCounts, type FileCounts } from './findings';
import type { AgentFile, Finding, Ref, RuntimeContext, State } from './types';

export interface StateIndex {
  files: Map<string, AgentFile>;
  contexts: Map<string, RuntimeContext>;
  findings: Map<string, Finding>;
  /** Symlink, build and mention references by their source file. */
  refsFrom: Map<string, Ref[]>;
  /** The same references by their target. */
  refsTo: Map<string, Ref[]>;
  /** The links that point at a file. */
  linksTo: Map<string, string[]>;
  counts: Map<string, FileCounts>;
  loadedIn: Map<string, Loading[]>;
}

function push<K, V>(m: Map<K, V[]>, k: K, v: V) {
  const list = m.get(k);
  if (list) list.push(v);
  else m.set(k, [v]);
}

export function indexState(s: State): StateIndex {
  const refsFrom = new Map<string, Ref[]>();
  const refsTo = new Map<string, Ref[]>();
  const linksTo = new Map<string, string[]>();
  for (const r of s.refs) {
    if (r.kind === 'load') continue;
    push(refsFrom, r.from, r);
    push(refsTo, r.to, r);
    if (r.kind === 'symlink') push(linksTo, r.to, r.from);
  }
  const loadedIn = new Map<string, Loading[]>();
  for (const c of s.contexts) {
    c.entries.forEach((e, i) => push(loadedIn, e.fileId, { context: c, position: i + 1 }));
  }
  return {
    files: new Map(s.files.map((f) => [f.id, f])),
    contexts: new Map(s.contexts.map((c) => [c.id, c])),
    findings: new Map(s.findings.map((f) => [f.id, f])),
    refsFrom,
    refsTo,
    linksTo,
    counts: fileFindingCounts(s.findings),
    loadedIn,
  };
}
