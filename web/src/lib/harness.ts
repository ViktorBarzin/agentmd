// Which files belong to one harness, for the header's harness filter.
import type { State } from './types';

/**
 * The files a harness reads, plus what they reach one hop out: a built file's
 * parts, a copy's origin, a symlink's target and the docs they mention. So
 * picking Claude Code keeps ~/.agents/core.md, which it reads through the
 * built ~/.agents/AGENTS.md.
 */
export function harnessFiles(s: Pick<State, 'files' | 'refs'>, harness: string): Set<string> {
  const out = new Set<string>();
  for (const f of s.files) if (f.harnesses.includes(harness)) out.add(f.id);
  const direct = new Set(out);
  const known = new Set(s.files.map((f) => f.id));
  for (const r of s.refs) {
    if (r.dangling || !direct.has(r.from) || !known.has(r.to)) continue;
    if (r.kind === 'build' || r.kind === 'symlink' || r.kind === 'mention') out.add(r.to);
  }
  return out;
}
