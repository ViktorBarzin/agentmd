<script lang="ts" module>
  import cytoscape from 'cytoscape';
  import fcose, { type FcoseLayoutOptions } from 'cytoscape-fcose';

  cytoscape.use(fcose);
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { plural } from '../format';
  import {
    buildGraph,
    DEFAULT_SHOW,
    MISSING_PREFIX,
    REF_KIND_LABELS,
    REF_KINDS,
    type GraphModel,
  } from '../graph';
  import { readStored, writeStored } from '../storage';
  import { store } from '../store.svelte';
  import { theme } from '../theme.svelte';
  import type { RefKind } from '../types';
  import FileBadges from './FileBadges.svelte';
  import Icon from './Icon.svelte';

  const OPTS_KEY = 'agentmd.graph';

  interface SavedOptions {
    show: Record<RefKind, boolean>;
    collapseLinks: boolean;
    hideSkills: boolean;
    hideIsolated: boolean;
  }

  function readOptions(): SavedOptions {
    const fallback: SavedOptions = { show: { ...DEFAULT_SHOW }, collapseLinks: false, hideSkills: false, hideIsolated: true };
    try {
      const raw = readStored(OPTS_KEY);
      if (!raw) return fallback;
      const v: unknown = JSON.parse(raw);
      if (typeof v !== 'object' || v === null) return fallback;
      const o = v as Partial<SavedOptions>;
      const show = { ...DEFAULT_SHOW };
      for (const k of REF_KINDS) if (typeof o.show?.[k] === 'boolean') show[k] = o.show[k];
      return {
        show,
        collapseLinks: o.collapseLinks === true,
        hideSkills: o.hideSkills === true,
        hideIsolated: o.hideIsolated !== false,
      };
    } catch {
      return fallback;
    }
  }

  const saved = readOptions();
  let show = $state<Record<RefKind, boolean>>(saved.show);
  let collapseLinks = $state(saved.collapseLinks);
  let hideSkills = $state(saved.hideSkills);
  let hideIsolated = $state(saved.hideIsolated);
  let legendOpen = $state(false);
  let selectedId = $state<string | null>(null);
  let container: HTMLDivElement | undefined = $state();
  let cy = $state.raw<cytoscape.Core | null>(null);
  let layingOut = $state(false);

  $effect(() => {
    writeStored(OPTS_KEY, JSON.stringify({ show, collapseLinks, hideSkills, hideIsolated }));
  });

  const model = $derived<GraphModel | null>(
    store.data
      ? buildGraph(
          store.data,
          {
            show,
            collapseLinks,
            hideSkills,
            hideIsolated,
            only: store.harnessSet,
            context: store.context,
            members: store.members,
            query: store.query,
          },
          store.index?.counts,
        )
      : null,
  );

  function cssVar(name: string): string {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#888';
  }

  function stylesheet(): cytoscape.StylesheetJson {
    const c = {
      text: cssVar('--text'),
      text2: cssVar('--text-2'),
      bg: cssVar('--bg'),
      accent: cssVar('--accent'),
      problem: cssVar('--problem'),
      hint: cssVar('--hint'),
      instruction: cssVar('--kind-instruction'),
      skill: cssVar('--kind-skill'),
      subagent: cssVar('--kind-subagent'),
      command: cssVar('--kind-command'),
      doc: cssVar('--kind-doc'),
      symlink: cssVar('--edge-symlink'),
      build: cssVar('--edge-build'),
      mention: cssVar('--edge-mention'),
      load: cssVar('--edge-load'),
    };
    const kinds = ['instruction', 'skill', 'subagent', 'command', 'doc'] as const;
    return [
      {
        selector: 'node',
        style: {
          'background-color': c.doc,
          width: 18,
          height: 18,
          label: 'data(label)',
          'font-size': 10,
          // cytoscape accepts double quotes in a font list, not single ones.
          'font-family': cssVar('--font-ui').replace(/'/g, '"'),
          color: c.text,
          'text-valign': 'bottom',
          'text-halign': 'center',
          'text-margin-y': 4,
          'text-max-width': '150px',
          'text-wrap': 'ellipsis',
          'text-background-color': c.bg,
          'text-background-opacity': 0.75,
          'text-background-padding': '1px',
          'text-background-shape': 'roundrectangle',
          'min-zoomed-font-size': 7,
          'border-width': 0,
        },
      },
      ...kinds.map((k) => ({ selector: `node.kind-${k}`, style: { 'background-color': c[k] } })),
      { selector: 'node.scope-org', style: { shape: 'diamond', width: 24, height: 24 } },
      { selector: 'node.scope-user', style: { shape: 'round-rectangle' } },
      { selector: 'node.scope-project', style: { shape: 'ellipse' } },
      { selector: 'node.scope-plugin', style: { shape: 'hexagon', width: 20, height: 20 } },
      { selector: 'node.scope-other', style: { shape: 'rectangle' } },
      { selector: 'node.built', style: { 'border-width': 4, 'border-style': 'double', 'border-color': c.text2 } },
      { selector: 'node.link', style: { 'background-opacity': 0.12, 'border-width': 2, 'border-style': 'dashed' } },
      ...kinds.map((k) => ({ selector: `node.link.kind-${k}`, style: { 'border-color': c[k] } })),
      {
        selector: 'node.missing',
        style: {
          shape: 'ellipse',
          'background-opacity': 0,
          'border-width': 2,
          'border-style': 'solid',
          'border-color': c.problem,
          color: c.problem,
        },
      },
      { selector: 'node.has-problem', style: { 'underlay-color': c.problem, 'underlay-opacity': 0.28, 'underlay-padding': 5 } },
      { selector: 'node.has-hint', style: { 'underlay-color': c.hint, 'underlay-opacity': 0.28, 'underlay-padding': 5 } },
      { selector: 'node.match', style: { 'border-width': 3, 'border-style': 'solid', 'border-color': c.hint } },
      { selector: 'node:selected', style: { 'border-width': 3, 'border-style': 'solid', 'border-color': c.text } },
      {
        selector: 'edge',
        style: {
          width: 1.3,
          'line-color': c.mention,
          'target-arrow-color': c.mention,
          'target-arrow-shape': 'triangle',
          'arrow-scale': 0.8,
          'curve-style': 'bezier',
        },
      },
      { selector: 'edge.ref-symlink', style: { 'line-style': 'dashed', 'line-color': c.symlink, 'target-arrow-color': c.symlink } },
      { selector: 'edge.ref-build', style: { width: 2.2, 'line-color': c.build, 'target-arrow-color': c.build } },
      { selector: 'edge.ref-load', style: { width: 1.6, 'line-color': c.load, 'target-arrow-color': c.load } },
      {
        selector: 'edge.ctx-load',
        style: {
          width: 2.6,
          label: 'data(label)',
          'font-size': 10,
          'font-weight': 'bold',
          color: c.load,
          'text-background-color': c.bg,
          'text-background-opacity': 1,
          'text-background-padding': '2px',
          'text-background-shape': 'roundrectangle',
        },
      },
      { selector: '.dim', style: { opacity: 0.14, 'underlay-opacity': 0.04 } },
      { selector: '.faded', style: { opacity: 0.2, 'underlay-opacity': 0.05 } },
    ];
  }

  function runLayout() {
    if (!cy || cy.nodes().length === 0) return;
    layingOut = true;
    const opts: FcoseLayoutOptions = {
      name: 'fcose',
      // 'proof' makes room for labels but costs about 1.5 s at 300 files, so
      // larger graphs use the faster default.
      quality: cy.nodes().length <= 150 ? 'proof' : 'default',
      randomize: true,
      animate: false,
      fit: true,
      padding: 36,
      nodeDimensionsIncludeLabels: true,
      nodeRepulsion: () => 12000,
      idealEdgeLength: () => 90,
      nodeSeparation: 110,
      tile: true,
      tilingPaddingVertical: 24,
      tilingPaddingHorizontal: 24,
    };
    const layout = cy.layout(opts);
    layout.one('layoutstop', () => {
      layingOut = false;
    });
    layout.run();
  }

  function sync(core: cytoscape.Core, m: GraphModel) {
    const nextNodes = new Set(m.nodes.map((n) => n.data.id));
    const currentNodes = new Set(core.nodes().map((n) => n.id()));
    const added = m.nodes.some((n) => !currentNodes.has(n.data.id));
    core.batch(() => {
      core.nodes().forEach((n) => {
        if (!nextNodes.has(n.id())) n.remove();
      });
      const nextEdges = new Set(m.edges.map((e) => e.data.id));
      core.edges().forEach((e) => {
        if (!nextEdges.has(e.id())) e.remove();
      });
      for (const n of m.nodes) {
        const el = core.getElementById(n.data.id);
        if (el.nonempty()) {
          el.data(n.data);
          el.classes(n.classes);
        } else {
          core.add({ group: 'nodes', data: { ...n.data }, classes: n.classes });
        }
      }
      for (const e of m.edges) {
        const el = core.getElementById(e.data.id);
        if (el.nonempty()) {
          el.data(e.data);
          el.classes(e.classes);
        } else {
          core.add({ group: 'edges', data: { ...e.data }, classes: e.classes });
        }
      }
    });
    if (added) runLayout();
    if (selectedId && !nextNodes.has(selectedId)) selectedId = null;
  }

  onMount(() => {
    if (!container) return;
    const core = cytoscape({
      container,
      elements: [],
      style: stylesheet(),
      minZoom: 0.15,
      maxZoom: 3,
      boxSelectionEnabled: false,
      selectionType: 'single',
    });
    core.on('select', 'node', (e) => {
      selectedId = e.target.id();
    });
    core.on('unselect', 'node', () => {
      if (core.nodes(':selected').empty()) selectedId = null;
    });
    core.on('dbltap', 'node', (e) => openNode(e.target.id()));
    cy = core;
    let frame = 0;
    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => core.resize());
    });
    ro.observe(container);
    return () => {
      ro.disconnect();
      cancelAnimationFrame(frame);
      core.destroy();
      cy = null;
    };
  });

  $effect(() => {
    if (cy && model) sync(cy, model);
  });

  $effect(() => {
    // Colours come from CSS variables, so restyle when the theme changes.
    void theme.effective;
    cy?.style(stylesheet());
  });

  function fit() {
    cy?.animate({ fit: { eles: cy.elements(), padding: 36 } }, { duration: 250 });
  }

  const selected = $derived(selectedId && model ? (model.nodes.find((n) => n.data.id === selectedId) ?? null) : null);
  const selectedFile = $derived(selected?.data.fileId ? (store.index?.files.get(selected.data.fileId) ?? null) : null);
  const mentionsOfMissing = $derived(
    selected && !selected.data.fileId && store.data
      ? store.data.refs.filter((r) => MISSING_PREFIX + r.to === selected.data.id)
      : [],
  );

  function openNode(id: string) {
    if (id.startsWith(MISSING_PREFIX)) {
      const r = store.data?.refs.find((x) => MISSING_PREFIX + x.to === id);
      if (r) store.openFile(r.from, r.line);
      return;
    }
    store.openFile(id);
  }

  const counts = $derived(model ? { nodes: model.nodes.length, edges: model.edges.length } : { nodes: 0, edges: 0 });
</script>

<div class="graph-view">
  <div class="toolbar" role="toolbar" aria-label="Graph options">
    <div class="chips">
      {#each REF_KINDS as k (k)}
        <button
          type="button"
          class="chip"
          aria-pressed={show[k]}
          onclick={() => (show = { ...show, [k]: !show[k] })}
        >
          <span class={`swatch ${k}`} aria-hidden="true"></span>{REF_KIND_LABELS[k]}
        </button>
      {/each}
      <span class="sep" aria-hidden="true"></span>
      <button type="button" class="chip" aria-pressed={collapseLinks} onclick={() => (collapseLinks = !collapseLinks)}>
        Collapse symlinks into targets
      </button>
      <button type="button" class="chip" aria-pressed={hideSkills} onclick={() => (hideSkills = !hideSkills)}>
        Hide skills
      </button>
      <button type="button" class="chip" aria-pressed={hideIsolated} onclick={() => (hideIsolated = !hideIsolated)}>
        Hide unconnected files
      </button>
    </div>
    <div class="actions">
      <span class="faint count">{plural(counts.nodes, 'node')}, {plural(counts.edges, 'edge')}</span>
      <button type="button" class="btn small" onclick={fit}><Icon name="fit" size={14} />Fit</button>
      <button type="button" class="btn small" onclick={runLayout} disabled={layingOut}>
        <Icon name="layout" size={14} />Re-layout
      </button>
      <button type="button" class="btn small" aria-expanded={legendOpen} onclick={() => (legendOpen = !legendOpen)}>
        Legend
      </button>
    </div>
  </div>

  <div class="stage">
    <div
      class="canvas"
      bind:this={container}
      role="img"
      aria-label={`Reference graph with ${plural(counts.nodes, 'file')} and ${plural(counts.edges, 'reference')}. The Files view lists the same files for keyboard use.`}
    ></div>

    {#if store.context}
      <div class="ctx-note">
        Numbered edges show the load order in <span class="mono">{store.context.display}</span>. Other files are dimmed.
      </div>
    {/if}

    {#if legendOpen}
      <aside class="legend" aria-label="Legend">
        <div class="legend-title">Nodes are files</div>
        <ul>
          <li><span class="swatch-dot" style="background: var(--kind-instruction)"></span>Instruction file</li>
          <li><span class="swatch-dot" style="background: var(--kind-skill)"></span>Skill</li>
          <li><span class="swatch-dot" style="background: var(--kind-subagent)"></span>Subagent</li>
          <li><span class="swatch-dot" style="background: var(--kind-command)"></span>Command</li>
          <li><span class="swatch-dot" style="background: var(--kind-doc)"></span>Referenced doc</li>
        </ul>
        <div class="legend-title">Shape is scope</div>
        <ul class="shapes">
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.5 14.5 8 8 14.5 1.5 8z" /></svg>Org</li>
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><rect x="2" y="2" width="12" height="12" rx="3" /></svg>User</li>
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6" /></svg>Project</li>
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4.5 2h7L15 8l-3.5 6h-7L1 8z" /></svg>Plugin</li>
        </ul>
        <ul class="shapes">
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6" class="dashed" /></svg>Link</li>
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6" class="double" /><circle cx="8" cy="8" r="3.6" class="double" /></svg>Built</li>
          <li><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6" class="missing" /></svg>Missing</li>
        </ul>
        <div class="legend-title">Edges are references</div>
        <ul>
          {#each REF_KINDS as k (k)}
            <li><span class={`swatch ${k}`}></span>{REF_KIND_LABELS[k]}</li>
          {/each}
        </ul>
        <p class="faint">Click a node for details, double-click to open it.</p>
      </aside>
    {/if}

    {#if selected}
      <aside class="card" aria-label="Selected file">
        <div class="card-head">
          <span class="mono path">{selected.data.display}</span>
          <button class="icon-btn" aria-label="Clear selection" onclick={() => cy?.nodes().unselect()}>
            <Icon name="close" size={14} />
          </button>
        </div>
        {#if selectedFile}
          <div class="card-meta">{selectedFile.kind}, {selectedFile.scope} scope</div>
          <FileBadges file={selectedFile} counts={store.index?.counts.get(selectedFile.id)} member={store.members?.get(selectedFile.id) ?? null} />
          {#if selected.data.aliases.length}
            <div class="card-meta">Also reached as {selected.data.aliases.join(', ')}</div>
          {/if}
          <div class="card-meta">
            {plural(store.index?.refsFrom.get(selectedFile.id)?.length ?? 0, 'reference')} out,
            {store.index?.refsTo.get(selectedFile.id)?.length ?? 0} in,
            loads in {plural(store.index?.loadedIn.get(selectedFile.id)?.length ?? 0, 'context')}
          </div>
          <div class="card-actions">
            <button class="btn small primary" onclick={() => openNode(selectedFile.id)}>
              <Icon name="edit" size={14} />Open in editor
            </button>
          </div>
        {:else}
          <div class="card-meta error-text">This file does not exist.</div>
          {#each mentionsOfMissing as r (r.from + r.line)}
            <div class="card-meta">
              Mentioned by <span class="mono">{store.index?.files.get(r.from)?.display ?? r.from}</span>{r.line ? ` on line ${r.line}` : ''}
            </div>
          {/each}
          <div class="card-actions">
            <button class="btn small" onclick={() => openNode(selected.data.id)}>Open the mention</button>
          </div>
        {/if}
      </aside>
    {/if}
  </div>
</div>

<style>
  .graph-view {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .chips,
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .sep {
    width: 1px;
    height: 18px;
    background: var(--border);
    margin: 0 2px;
  }
  .count {
    font-size: 12px;
    margin-right: 4px;
  }
  .swatch {
    display: inline-block;
    width: 16px;
    height: 0;
    border-top: 2px solid var(--edge-mention);
    flex: none;
  }
  .swatch.symlink {
    border-top: 2px dashed var(--edge-symlink);
  }
  .swatch.build {
    border-top: 3px solid var(--edge-build);
  }
  .swatch.load {
    border-top: 2px solid var(--edge-load);
  }
  .stage {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .canvas {
    position: absolute;
    inset: 0;
  }
  .ctx-note {
    position: absolute;
    left: 12px;
    top: 10px;
    max-width: calc(100% - 24px);
    padding: 4px 8px;
    border-radius: 5px;
    background: var(--surface);
    border: 1px solid var(--border);
    color: var(--text-2);
    font-size: 12px;
  }
  .legend,
  .card {
    position: absolute;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    box-shadow: var(--shadow);
    font-size: 12.5px;
  }
  .legend {
    left: 12px;
    bottom: 12px;
    width: 230px;
    max-height: calc(100% - 24px);
    overflow: auto;
    padding: 10px 12px;
  }
  .legend-title {
    margin: 8px 0 4px;
    color: var(--text-3);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .legend-title:first-child {
    margin-top: 0;
  }
  .legend ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 3px;
  }
  .legend ul.shapes {
    grid-template-columns: 1fr 1fr;
    margin-bottom: 4px;
  }
  .legend li {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .legend svg {
    width: 14px;
    height: 14px;
    fill: var(--text-3);
    stroke: none;
  }
  .legend svg .dashed {
    fill: none;
    stroke: var(--kind-instruction);
    stroke-width: 1.6;
    stroke-dasharray: 2.5 2;
  }
  .legend svg .double {
    fill: none;
    stroke: var(--text-2);
    stroke-width: 1.2;
  }
  .legend svg .missing {
    fill: none;
    stroke: var(--problem);
    stroke-width: 1.8;
  }
  .swatch-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
  }
  .legend p {
    margin: 8px 0 0;
  }
  .card {
    right: 12px;
    bottom: 12px;
    width: min(340px, calc(100% - 24px));
    padding: 10px 12px;
    display: grid;
    gap: 6px;
  }
  .card-head {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  .path {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
    font-size: 12.5px;
    font-weight: 600;
  }
  .card-meta {
    color: var(--text-2);
  }
  .card-actions {
    display: flex;
    gap: 6px;
  }
  @media (max-width: 799px) {
    .legend {
      right: 12px;
      width: auto;
    }
  }
</style>
