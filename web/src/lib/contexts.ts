// Runtime contexts: grouping them for the picker, deriving load order, and
// working out which files belong to a context.
import type { Candidate, ContextEntry, HarnessName, Job, Ref, RuntimeContext, State } from './types';

/** A load-order reference, derived on the client from a context's entries. */
export interface LoadRef extends Ref {
  kind: 'load';
  context: string;
  /** 1 for the first hop (entry 1 to entry 2), and so on. */
  step: number;
}

export function loadRefs(contexts: RuntimeContext[]): LoadRef[] {
  const out: LoadRef[] = [];
  for (const c of contexts) {
    for (let i = 0; i + 1 < c.entries.length; i++) {
      out.push({ from: c.entries[i].fileId, to: c.entries[i + 1].fileId, kind: 'load', context: c.id, step: i + 1 });
    }
  }
  return out;
}

export type MemberRole = 'entry' | 'link-target' | 'source' | 'skill' | 'subagent' | 'skipped';

export interface Membership {
  role: MemberRole;
  /** Load position of the entry this file belongs to. */
  position?: number;
  /** The file that brought this one in: the link, or the built file. */
  via?: string;
  /** Why the harness skipped the file, for role "skipped". */
  reason?: string;
}

/**
 * The files that make up a context: its entries in load order, the files
 * those entries link to, the parts and origins behind them, and the skills
 * and subagents it offers.
 */
export function contextMembers(ctx: RuntimeContext, refs: Ref[]): Map<string, Membership> {
  const links = new Map<string, string>();
  const sources = new Map<string, string[]>();
  for (const r of refs) {
    if (r.dangling) continue;
    if (r.kind === 'symlink') links.set(r.from, r.to);
    else if (r.kind === 'build') {
      const list = sources.get(r.from) ?? [];
      list.push(r.to);
      sources.set(r.from, list);
    }
  }

  const out = new Map<string, Membership>();
  const add = (id: string, m: Membership) => {
    if (!out.has(id)) out.set(id, m);
  };

  // Follows links from a file, then the build sources of whatever it reaches.
  const expand = (id: string, base: Omit<Membership, 'role' | 'via'>, linkRole: MemberRole) => {
    let current = id;
    const seen = new Set<string>([id]);
    const chain = [id];
    for (let next = links.get(current); next !== undefined && !seen.has(next); next = links.get(current)) {
      add(next, { ...base, role: linkRole, via: current });
      seen.add(next);
      chain.push(next);
      current = next;
    }
    if (linkRole !== 'link-target') return;
    for (const holder of chain) {
      for (const part of sources.get(holder) ?? []) add(part, { ...base, role: 'source', via: holder });
    }
  };

  ctx.entries.forEach((e, i) => add(e.fileId, { role: 'entry', position: i + 1 }));
  ctx.entries.forEach((e, i) => expand(e.fileId, { position: i + 1 }, 'link-target'));
  for (const s of ctx.skills) {
    if (!s.fileId) continue;
    add(s.fileId, { role: 'skill' });
    expand(s.fileId, {}, 'skill');
  }
  for (const s of ctx.subagents) {
    if (!s.fileId) continue;
    add(s.fileId, { role: 'subagent' });
    expand(s.fileId, {}, 'subagent');
  }
  // Files the harness passed over belong to the picture too, marked as such.
  for (const e of ctx.skipped ?? []) add(e.fileId, e.reason ? { role: 'skipped', reason: e.reason } : { role: 'skipped' });
  return out;
}

export interface Loading {
  context: RuntimeContext;
  position: number;
}

/** The contexts that list a file as an entry, with its load position. */
export function contextsLoading(fileId: string, contexts: RuntimeContext[]): Loading[] {
  const out: Loading[] = [];
  for (const c of contexts) {
    const i = c.entries.findIndex((e) => e.fileId === fileId);
    if (i !== -1) out.push({ context: c, position: i + 1 });
  }
  return out;
}

export interface ContextGroup {
  harness: HarnessName;
  label: string;
  version?: string;
  probe: boolean;
  contexts: RuntimeContext[];
  /** Directories this harness could be probed in but has not been yet. */
  unprobed: Candidate[];
}

/** The id a candidate's context gets once it is probed. */
export function candidateId(c: Candidate): string {
  return `${c.harness}:${c.dir}`;
}

export function groupContexts(s: Pick<State, 'harnesses' | 'contexts' | 'unprobed'>): ContextGroup[] {
  const groups = new Map<string, ContextGroup>();
  const group = (harness: string, probe: boolean): ContextGroup => {
    let g = groups.get(harness);
    if (!g) {
      g = { harness, label: harness, probe, contexts: [], unprobed: [] };
      groups.set(harness, g);
    }
    return g;
  };
  for (const h of s.harnesses) {
    groups.set(h.name, { harness: h.name, label: h.label, version: h.version, probe: h.probe, contexts: [], unprobed: [] });
  }
  for (const c of s.contexts) group(c.harness, c.source === 'probe').contexts.push(c);
  for (const c of s.unprobed ?? []) group(c.harness, true).unprobed.push(c);
  const out = [...groups.values()].filter((g) => g.contexts.length + g.unprobed.length > 0);
  for (const g of out) {
    g.contexts.sort((a, b) => a.dir.localeCompare(b.dir));
    g.unprobed.sort((a, b) => a.dir.localeCompare(b.dir));
  }
  return out;
}

export interface Skip {
  context: RuntimeContext;
  entry: ContextEntry;
}

/** The contexts whose harness could have loaded a file but skipped it, with the reason. */
export function contextsSkipping(fileId: string, contexts: RuntimeContext[]): Skip[] {
  const out: Skip[] = [];
  for (const c of contexts) {
    for (const e of c.skipped ?? []) if (e.fileId === fileId) out.push({ context: c, entry: e });
  }
  return out;
}

/** What "Probe all" would probe: directories never probed, and stale contexts. */
export function probeBacklog(s: Pick<State, 'contexts' | 'unprobed'>): { unprobed: number; stale: number } {
  return {
    unprobed: s.unprobed?.length ?? 0,
    stale: s.contexts.filter((c) => c.source === 'probe' && c.stale).length,
  };
}

/** "Probing 37 of 142" while a probe job runs. */
export function probeProgress(job: Job): string {
  if (job.status !== 'running') return '';
  if (!job.total) return 'Probing';
  return `Probing ${Math.min(job.done ?? 0, job.total)} of ${job.total}`;
}

export type AnalysisStatus =
  | { state: 'never' }
  | { state: 'running'; since: string }
  | { state: 'error'; message: string }
  | { state: 'done'; at: string; cli: string; findings: number }
  | { state: 'stale'; at: string; cli: string; findings: number };

function time(iso: string | undefined): number {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isNaN(t) ? 0 : t;
}

/** What to show for a context's analysis, given the last job started for it. */
export function analysisStatus(ctx: RuntimeContext, job?: Job): AnalysisStatus {
  const a = ctx.analysis;
  if (job?.status === 'running') return { state: 'running', since: job.startedAt };
  if (job?.status === 'error' && (!a || time(a.at) < time(job.startedAt))) {
    return { state: 'error', message: job.error || 'The analysis failed.' };
  }
  if (job?.status === 'done' && (!a || time(a.at) < time(job.finishedAt))) {
    return { state: 'done', at: job.finishedAt ?? job.startedAt, cli: a?.cli ?? '', findings: job.findings?.length ?? 0 };
  }
  if (!a) return { state: 'never' };
  return { state: a.stale ? 'stale' : 'done', at: a.at, cli: a.cli, findings: a.findings };
}
