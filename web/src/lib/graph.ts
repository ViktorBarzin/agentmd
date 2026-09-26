// Graph elements for cytoscape, built from the state and the view options.
// Kept free of cytoscape itself so it can be tested and reused.
import { loadRefs, type LoadRef, type Membership } from './contexts';
import type { FileCounts } from './findings';
import { basename, dirname } from './format';
import { matchesQuery } from './tree';
import type { AgentFile, Kind, Ref, RefKind, RuntimeContext, Scope, State } from './types';

export const REF_KINDS: RefKind[] = ['symlink', 'build', 'mention', 'load'];

export const REF_KIND_LABELS: Record<RefKind, string> = {
  symlink: 'Symlink',
  build: 'Build join',
  mention: 'Text mention',
  load: 'Load order',
};

export const DEFAULT_SHOW: Record<RefKind, boolean> = { symlink: true, build: true, mention: true, load: true };

export interface GraphOptions {
  show: Record<RefKind, boolean>;
  collapseLinks: boolean;
  hideSkills: boolean;
  /** Leave out files that no shown reference touches (a picked context's files stay). */
  hideIsolated: boolean;
  /** Only these files, from the header's harness filter; null for all. */
  only?: Set<string> | null;
  context: RuntimeContext | null;
  members: Map<string, Membership> | null;
  query: string;
}

export interface GraphNodeData {
  id: string;
  label: string;
  kind: Kind | 'missing';
  scope: Scope | 'missing';
  display: string;
  /** The file this node stands for; unset for a dangling target. */
  fileId?: string;
  /** Links collapsed into this node. */
  aliases: string[];
  /** Load positions in the picked context. */
  positions: number[];
  problems: number;
  hints: number;
}

export interface GraphEdgeData {
  id: string;
  source: string;
  target: string;
  kind: RefKind;
  label: string;
  count: number;
  lines: number[];
}

export interface GraphElement<T> {
  data: T;
  classes: string[];
}

export interface GraphModel {
  nodes: GraphElement<GraphNodeData>[];
  edges: GraphElement<GraphEdgeData>[];
}

const GENERIC_NAMES = new Set(['AGENTS.md', 'CLAUDE.md', 'AGENTS.override.md', 'CLAUDE.local.md', 'SKILL.md']);

/** A short label: the skill's name, or the folder for generic file names. */
export function nodeLabel(f: AgentFile): string {
  if ((f.kind === 'skill' || f.kind === 'subagent' || f.kind === 'command') && f.name) return f.name;
  const name = basename(f.path);
  const withField = f.field ? `${name}#${f.field}` : name;
  if (!GENERIC_NAMES.has(name) && !f.field) return withField;
  const parent = basename(dirname(f.path));
  return parent ? `${parent}/${withField}` : withField;
}

export const MISSING_PREFIX = 'missing:';

export function buildGraph(state: State, opts: GraphOptions, counts?: Map<string, FileCounts>): GraphModel {
  const files = new Map(state.files.map((f) => [f.id, f]));

  // Symlinks to collapse: link id -> target id.
  const alias = new Map<string, string>();
  if (opts.collapseLinks) {
    for (const r of state.refs) {
      if (r.kind === 'symlink' && !r.dangling && files.has(r.to) && r.from !== r.to) alias.set(r.from, r.to);
    }
  }
  const resolve = (id: string): string => {
    let cur = id;
    const seen = new Set<string>();
    while (alias.has(cur) && !seen.has(cur)) {
      seen.add(cur);
      cur = alias.get(cur) ?? cur;
    }
    return cur;
  };

  const hidden = (id: string) =>
    (opts.hideSkills && files.get(id)?.kind === 'skill') || (opts.only ? !opts.only.has(id) : false);

  // Context membership, carried through collapsed links.
  const memberNodes = new Set<string>();
  const positions = new Map<string, number[]>();
  if (opts.context && opts.members) {
    for (const [id, m] of opts.members) {
      if (m.role === 'skipped') continue;
      const node = resolve(id);
      memberNodes.add(node);
      if (m.role === 'entry' && m.position !== undefined) {
        const list = positions.get(node) ?? [];
        if (!list.includes(m.position)) list.push(m.position);
        positions.set(node, list);
      }
    }
  }

  const nodes = new Map<string, GraphElement<GraphNodeData>>();
  const aliases = new Map<string, string[]>();
  for (const f of state.files) {
    const r = resolve(f.id);
    if (r !== f.id) {
      const list = aliases.get(r) ?? [];
      list.push(f.display);
      aliases.set(r, list);
      continue;
    }
    if (hidden(f.id)) continue;
    const c = counts?.get(f.id);
    const pos = (positions.get(f.id) ?? []).sort((a, b) => a - b);
    const base = nodeLabel(f);
    const classes = [`kind-${f.kind}`, `scope-${f.scope}`];
    if (f.isLink) classes.push('link');
    if (f.access.builtFrom?.length) classes.push('built');
    if (!f.access.writable) classes.push('readonly');
    if (f.missing) classes.push('missing');
    if (c?.problem) classes.push('has-problem');
    else if (c?.hint) classes.push('has-hint');
    nodes.set(f.id, {
      data: {
        id: f.id,
        label: pos.length ? `${pos.join(',')} · ${base}` : base,
        kind: f.kind,
        scope: f.scope,
        display: f.display,
        fileId: f.id,
        aliases: [],
        positions: pos,
        problems: c?.problem ?? 0,
        hints: c?.hint ?? 0,
      },
      classes,
    });
  }
  for (const [id, list] of aliases) {
    const n = nodes.get(id);
    if (n) n.data.aliases = list.sort();
  }

  const refs: Array<Ref | LoadRef> = state.refs.filter((r) => r.kind !== 'load');
  refs.push(...loadRefs(opts.context ? [opts.context] : state.contexts));

  const edges = new Map<string, GraphElement<GraphEdgeData>>();
  const missingSources = new Map<string, string[]>();
  for (const r of refs) {
    if (!opts.show[r.kind]) continue;
    const source = resolve(r.from);
    if (!nodes.has(source)) continue;
    let target: string;
    if (r.dangling || !files.has(r.to)) {
      target = MISSING_PREFIX + r.to;
      if (!nodes.has(target)) {
        nodes.set(target, {
          data: {
            id: target,
            label: basename(r.to),
            kind: 'missing',
            scope: 'missing',
            display: r.to,
            aliases: [],
            positions: [],
            problems: 0,
            hints: 0,
          },
          classes: ['missing'],
        });
      }
      const srcs = missingSources.get(target) ?? [];
      srcs.push(source);
      missingSources.set(target, srcs);
    } else {
      target = resolve(r.to);
      if (!nodes.has(target)) continue;
    }
    if (source === target) continue;
    const key = `${r.kind}|${source}|${target}`;
    const existing = edges.get(key);
    const step = 'step' in r && opts.context ? String(r.step) : '';
    if (existing) {
      existing.data.count++;
      if (r.line !== undefined) existing.data.lines.push(r.line);
      if (step) existing.data.label = existing.data.label ? `${existing.data.label},${step}` : step;
      continue;
    }
    edges.set(key, {
      data: {
        id: 'e:' + key,
        source,
        target,
        kind: r.kind,
        label: step,
        count: 1,
        lines: r.line !== undefined ? [r.line] : [],
      },
      classes: [`ref-${r.kind}`],
    });
  }

  if (opts.hideIsolated) {
    const touched = new Set<string>();
    for (const e of edges.values()) {
      touched.add(e.data.source);
      touched.add(e.data.target);
    }
    for (const id of [...nodes.keys()]) {
      if (!touched.has(id) && !memberNodes.has(id)) nodes.delete(id);
    }
  }

  if (opts.context) {
    for (const [id, n] of nodes) {
      const isMember = n.data.kind === 'missing'
        ? (missingSources.get(id) ?? []).every((s) => memberNodes.has(s))
        : memberNodes.has(id);
      n.classes.push(isMember ? 'member' : 'dim');
    }
    for (const e of edges.values()) {
      if (e.data.kind === 'load') e.classes.push('ctx-load');
      else if (!(memberNodes.has(e.data.source) && nodes.get(e.data.target)?.classes.includes('member'))) {
        e.classes.push('dim');
      }
    }
  }

  const q = opts.query.trim();
  if (q) {
    const lower = q.toLowerCase();
    for (const [id, n] of nodes) {
      const f = files.get(id);
      const hit = f ? matchesQuery(f, q) : n.data.display.toLowerCase().includes(lower);
      n.classes.push(hit ? 'match' : 'faded');
    }
    for (const e of edges.values()) {
      const s = nodes.get(e.data.source);
      const t = nodes.get(e.data.target);
      if (s?.classes.includes('faded') || t?.classes.includes('faded')) e.classes.push('faded');
    }
  }

  return { nodes: [...nodes.values()], edges: [...edges.values()] };
}
