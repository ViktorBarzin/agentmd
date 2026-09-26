import { describe, expect, it } from 'vitest';
import { contextMembers } from './contexts';
import { buildGraph, DEFAULT_SHOW, nodeLabel, type GraphOptions } from './graph';
import { ctx, file, ref, state } from './test-helpers';
import type { State } from './types';

const org = '/etc/claude-code/managed-settings.json#claudeMd';
const userLink = '/home/alex/.claude/CLAUDE.md';
const hub = '/home/alex/.agents/AGENTS.md';
const core = '/home/alex/.agents/core.md';
const infraLink = '/home/alex/code/infra/CLAUDE.md';
const infra = '/home/alex/code/infra/AGENTS.md';
const doc = '/home/alex/code/infra/docs/agents/terraform.md';
const skill = '/home/alex/.agents/skills/publish-page/SKILL.md';
const missing = '/home/alex/code/infra/docs/agents/old-runbook.md';

const claudeInfra = ctx('claude:/home/alex/code/infra', [org, userLink, infraLink]);
const codexInfra = ctx('codex:/home/alex/code/infra', [org, infra]);

const s: State = state({
  files: [
    file(org, { scope: 'org', access: { writable: false, reason: 'owned by root' } }),
    file(userLink, { scope: 'user', isLink: true, linkTarget: hub }),
    file(hub, { scope: 'user', access: { writable: false, builtFrom: [core], builtBy: 'agents-sync' } }),
    file(core, { scope: 'user' }),
    file(infraLink, { isLink: true, linkTarget: infra }),
    file(infra),
    file(doc, { kind: 'doc' }),
    file(skill, { kind: 'skill', scope: 'user', name: 'publish-page' }),
  ],
  refs: [
    ref(userLink, hub, 'symlink', { sub: 'link' }),
    ref(infraLink, infra, 'symlink', { sub: 'link' }),
    ref(hub, core, 'build', { sub: 'header' }),
    ref(infra, doc, 'mention', { sub: 'path', line: 45 }),
    ref(infra, doc, 'mention', { sub: 'link', line: 46 }),
    ref(core, skill, 'mention', { sub: 'skill', line: 31 }),
    ref(infra, missing, 'mention', { sub: 'path', line: 88, dangling: true }),
  ],
  contexts: [claudeInfra, codexInfra],
});

function opts(extra: Partial<GraphOptions> = {}): GraphOptions {
  return {
    show: { ...DEFAULT_SHOW },
    collapseLinks: false,
    hideSkills: false,
    context: null,
    members: null,
    query: '',
    ...extra,
  };
}

const ids = (xs: { data: { id: string } }[]) => xs.map((x) => x.data.id).sort();
const edgeKeys = (g: ReturnType<typeof buildGraph>) =>
  g.edges.map((e) => `${e.data.kind} ${e.data.source} > ${e.data.target}`).sort();

describe('buildGraph', () => {
  it('makes one node per file plus one per dangling target', () => {
    const g = buildGraph(s, opts());
    expect(ids(g.nodes)).toEqual([...s.files.map((f) => f.id), 'missing:' + missing].sort());
    const m = g.nodes.find((n) => n.data.id === 'missing:' + missing);
    expect(m?.classes).toContain('missing');
    expect(m?.data.label).toBe('old-runbook.md');
  });

  it('merges repeated references into one edge with a count', () => {
    const g = buildGraph(s, opts());
    const e = g.edges.filter((x) => x.data.kind === 'mention' && x.data.target === doc);
    expect(e).toHaveLength(1);
    expect(e[0].data.count).toBe(2);
    expect(e[0].data.lines).toEqual([45, 46]);
  });

  it('derives load order from every context when none is picked, once per pair', () => {
    const g = buildGraph(s, opts());
    const load = g.edges.filter((e) => e.data.kind === 'load');
    expect(load.map((e) => `${e.data.source} > ${e.data.target}`).sort()).toEqual(
      [`${org} > ${userLink}`, `${userLink} > ${infraLink}`, `${org} > ${infra}`].sort(),
    );
    expect(load.every((e) => e.data.label === '')).toBe(true);
  });

  it('drops a reference kind when its toggle is off', () => {
    const g = buildGraph(s, opts({ show: { ...DEFAULT_SHOW, mention: false, load: false } }));
    expect(g.edges.some((e) => e.data.kind === 'mention' || e.data.kind === 'load')).toBe(false);
    expect(g.edges.some((e) => e.data.kind === 'symlink')).toBe(true);
  });

  it('collapses symlinks into their targets and re-points their references', () => {
    const g = buildGraph(s, opts({ collapseLinks: true }));
    expect(ids(g.nodes)).not.toContain(userLink);
    expect(ids(g.nodes)).not.toContain(infraLink);
    expect(g.edges.some((e) => e.data.kind === 'symlink')).toBe(false);
    const hubNode = g.nodes.find((n) => n.data.id === hub);
    expect(hubNode?.data.aliases).toEqual(['~/.claude/CLAUDE.md']);
    expect(edgeKeys(g)).toContain(`load ${org} > ${hub}`);
    expect(edgeKeys(g)).toContain(`load ${hub} > ${infra}`);
  });

  it('hides skills and the references that touch them', () => {
    const g = buildGraph(s, opts({ hideSkills: true }));
    expect(ids(g.nodes)).not.toContain(skill);
    expect(g.edges.some((e) => e.data.target === skill)).toBe(false);
  });

  it('numbers the load order of a picked context and dims everything else', () => {
    const members = contextMembers(claudeInfra, s.refs);
    const g = buildGraph(s, opts({ context: claudeInfra, members }));
    const load = g.edges.filter((e) => e.data.kind === 'load');
    expect(load.map((e) => [e.data.source, e.data.label])).toEqual([
      [org, '1'],
      [userLink, '2'],
    ]);
    expect(load.every((e) => e.classes.includes('ctx-load'))).toBe(true);
    const cls = (id: string) => g.nodes.find((n) => n.data.id === id)?.classes ?? [];
    expect(cls(org)).toContain('member');
    expect(cls(hub)).toContain('member');
    expect(cls(core)).toContain('member');
    expect(cls(doc)).toContain('dim');
    expect(cls(skill)).toContain('dim');
    expect(g.nodes.find((n) => n.data.id === userLink)?.data.label).toBe('2 · .claude/CLAUDE.md');
    const docEdge = g.edges.find((e) => e.data.target === doc);
    expect(docEdge?.classes).toContain('dim');
  });

  it('carries load positions onto the target when links are collapsed', () => {
    const members = contextMembers(claudeInfra, s.refs);
    const g = buildGraph(s, opts({ context: claudeInfra, members, collapseLinks: true }));
    expect(g.nodes.find((n) => n.data.id === hub)?.data.positions).toEqual([2]);
    expect(g.nodes.find((n) => n.data.id === infra)?.data.positions).toEqual([3]);
  });

  it('fades nodes that do not match the search', () => {
    const g = buildGraph(s, opts({ query: 'terraform' }));
    expect(g.nodes.find((n) => n.data.id === doc)?.classes).toContain('match');
    expect(g.nodes.find((n) => n.data.id === infra)?.classes).toContain('faded');
  });

  it('marks links, built files and read-only files', () => {
    const g = buildGraph(s, opts());
    const cls = (id: string) => g.nodes.find((n) => n.data.id === id)?.classes ?? [];
    expect(cls(userLink)).toContain('link');
    expect(cls(hub)).toContain('built');
    expect(cls(org)).toContain('readonly');
    expect(cls(org)).toContain('scope-org');
    expect(cls(doc)).toContain('kind-doc');
  });

  it('stays fast with 300 files and 500 references', () => {
    const files = Array.from({ length: 300 }, (_, i) => file(`/home/alex/code/r${i % 20}/f${i}.md`));
    const refs = Array.from({ length: 500 }, (_, i) =>
      ref(files[i % 300].id, files[(i * 7 + 3) % 300].id, i % 3 === 0 ? 'mention' : 'build'),
    );
    const contexts = Array.from({ length: 40 }, (_, i) =>
      ctx(`claude:/home/alex/code/r${i}`, [files[i].id, files[i + 1].id, files[i + 2].id]),
    );
    const big = state({ files, refs, contexts });
    const t = performance.now();
    const g = buildGraph(big, opts({ collapseLinks: true }));
    expect(performance.now() - t).toBeLessThan(200);
    expect(g.nodes).toHaveLength(300);
  });
});

describe('nodeLabel', () => {
  it('names skills by their name and generic file names by their folder', () => {
    expect(nodeLabel(file(skill, { kind: 'skill', name: 'publish-page' }))).toBe('publish-page');
    expect(nodeLabel(file(infra))).toBe('infra/AGENTS.md');
    expect(nodeLabel(file(doc, { kind: 'doc' }))).toBe('terraform.md');
    expect(nodeLabel(file(org, { field: 'claudeMd' }))).toBe('claude-code/managed-settings.json#claudeMd');
  });
});
