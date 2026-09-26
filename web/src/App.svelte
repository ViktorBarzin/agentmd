<script lang="ts" module>
  // The graph (cytoscape) and the editors (CodeMirror) load on first use, so
  // the Files and Findings views start without them.
  const lazy = <T,>(load: () => Promise<T>) => {
    let p: Promise<T> | undefined;
    return () => (p ??= load());
  };
  const loadGraph = lazy(() => import('./lib/components/GraphView.svelte'));
  const loadEditor = lazy(() => import('./lib/components/EditorPanel.svelte'));
  const loadCompare = lazy(() => import('./lib/components/CompareView.svelte'));
  const loadProposal = lazy(() => import('./lib/components/ProposalDialog.svelte'));
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import ContextPicker from './lib/components/ContextPicker.svelte';
  import FilesView from './lib/components/FilesView.svelte';
  import FindingsView from './lib/components/FindingsView.svelte';
  import Icon, { type IconName } from './lib/components/Icon.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import { errorMessage } from './lib/api';
  import { candidateId } from './lib/contexts';
  import { countFindings } from './lib/findings';
  import { formatTime, relativeTime } from './lib/format';
  import { backgroundView, type View } from './lib/route';
  import { readStored, writeStored } from './lib/storage';
  import { store } from './lib/store.svelte';
  import { theme } from './lib/theme.svelte';
  import { groupFiles } from './lib/tree';

  onMount(() => store.init());

  const WIDTH_KEY = 'agentmd.editorWidth';
  let editorWidth = $state(Number(readStored(WIDTH_KEY)) || 620);
  let searchInput: HTMLInputElement | undefined = $state();

  const route = $derived(store.route);
  const view = $derived(backgroundView(route));
  const editorOpen = $derived(route.name === 'file');
  const compareOpen = $derived(route.name === 'compare');
  const counts = $derived(countFindings(store.contextFindings));
  const proposalFinding = $derived(store.proposalFor ? (store.index?.findings.get(store.proposalFor) ?? null) : null);

  const views: Array<{ id: View; label: string; icon: IconName }> = [
    { id: 'files', label: 'Files', icon: 'files' },
    { id: 'graph', label: 'Graph', icon: 'graph' },
    { id: 'findings', label: 'Findings', icon: 'findings' },
  ];

  function viewHref(v: View): string {
    return store.hrefFor(route.ctx ? { name: v, ctx: route.ctx } : { name: v });
  }

  const themeIcon = $derived<IconName>(theme.mode === 'system' ? 'monitor' : theme.mode === 'light' ? 'sun' : 'moon');
  const themeLabel = $derived(
    theme.mode === 'system' ? 'Theme follows the system. Switch to light' : theme.mode === 'light' ? 'Light theme. Switch to dark' : 'Dark theme. Follow the system',
  );

  function onKeydown(e: KeyboardEvent) {
    const mod = e.metaKey || e.ctrlKey;
    if (!mod || e.altKey) return;
    const key = e.key.toLowerCase();
    if (key === 'k') {
      e.preventDefault();
      searchInput?.focus();
      searchInput?.select();
    } else if (key === 's' && (editorOpen || compareOpen)) {
      if (e.defaultPrevented) return;
      e.preventDefault();
      store.activeSave?.();
    }
  }

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      store.query = '';
      searchInput?.blur();
    } else if (e.key === 'Enter') {
      const home = store.data?.home ?? '';
      const matches = store.visibleFiles.filter((f) => store.queryMatch(f.id));
      const first = groupFiles(matches, home)[0]?.files[0];
      if (first) store.openFile(first.id);
    }
  }

  function startResize(e: PointerEvent) {
    const handle = e.currentTarget;
    if (!(handle instanceof HTMLElement)) return;
    handle.setPointerCapture(e.pointerId);
    const move = (ev: PointerEvent) => setWidth(window.innerWidth - ev.clientX);
    const up = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', up);
      handle.removeEventListener('pointercancel', up);
      writeStored(WIDTH_KEY, String(Math.round(editorWidth)));
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
    handle.addEventListener('pointercancel', up);
  }

  function setWidth(w: number) {
    const max = Math.max(380, window.innerWidth - 176 - 360);
    editorWidth = Math.min(max, Math.max(360, w));
  }

  function onSplitterKey(e: KeyboardEvent) {
    if (e.key === 'ArrowLeft') setWidth(editorWidth + 40);
    else if (e.key === 'ArrowRight') setWidth(editorWidth - 40);
    else return;
    e.preventDefault();
    writeStored(WIDTH_KEY, String(Math.round(editorWidth)));
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="app" class:with-editor={editorOpen} style={`--editor-w: ${editorWidth}px`}>
  <header class="top">
    <div class="brand">
      <span class="logo" aria-hidden="true">md</span>
      <span class="name">agentmd</span>
      {#if store.data}
        <span class="owner" title="The OS user whose agent files these are">{store.data.owner}</span>
      {/if}
    </div>
    <div class="scan">
      {#if store.data}
        <span class="when" title={`Last scan: ${formatTime(store.data.scannedAt)}`}>
          Scanned {relativeTime(store.data.scannedAt, store.now)}
        </span>
      {/if}
      <button class="icon-btn" aria-label="Rescan the agent files" title="Rescan" onclick={() => store.rescan()} disabled={store.scanning || !store.data}>
        <Icon name="refresh" spin={store.scanning} />
      </button>
    </div>
    <label class="search">
      <Icon name="search" size={15} />
      <span class="sr-only">Search files</span>
      <input
        bind:this={searchInput}
        bind:value={store.query}
        type="search"
        placeholder="Search files"
        autocomplete="off"
        spellcheck="false"
        onkeydown={onSearchKey}
      />
      <kbd aria-hidden="true">Ctrl K</kbd>
    </label>
    <label class="harness">
      <span class="sr-only">Harness</span>
      <select value={store.harness} onchange={(e) => store.setHarness(e.currentTarget.value)} title="Show one harness's files">
        <option value="">All harnesses</option>
        {#each store.data?.harnesses ?? [] as h (h.name)}
          <option value={h.name}>{h.label}</option>
        {/each}
      </select>
    </label>
    <div class="picker"><ContextPicker /></div>
    <button class="icon-btn theme" aria-label={themeLabel} title={themeLabel} onclick={() => theme.cycle()}>
      <Icon name={themeIcon} />
    </button>
  </header>

  <nav class="rail" aria-label="Views">
    {#each views as v (v.id)}
      <a href={viewHref(v.id)} aria-current={view === v.id && !compareOpen ? 'page' : undefined}>
        <Icon name={v.icon} />
        <span class="label">{v.label}</span>
        {#if v.id === 'files'}
          <span class="n">{store.visibleFiles.length}</span>
        {:else if v.id === 'findings'}
          <span class="counts">
            {#if counts.problem}<span class="badge problem" title={`${counts.problem} problems`}>{counts.problem}</span>{/if}
            {#if counts.hint}<span class="badge hint" title={`${counts.hint} hints`}>{counts.hint}</span>{/if}
          </span>
        {/if}
      </a>
    {/each}
  </nav>

  <main class="main" id="main">
    {#if route.ctx && store.data && !store.context}
      {@const candidate = store.data.unprobed?.find((c) => candidateId(c) === route.ctx)}
      <div class="ctx-missing banner warn" role="status">
        <Icon name="probe" />
        <div class="banner-body">
          {#if candidate}
            {store.contextLabel(route.ctx)} has not been probed yet, so what loads there is unknown.
          {:else}
            The last scan has no context <span class="mono">{route.ctx}</span>.
          {/if}
          <div class="banner-actions">
            {#if candidate}
              {@const ctxId = route.ctx}
              <button class="btn small" disabled={store.isProbing(ctxId)} onclick={() => store.probeAndPick(ctxId)}>
                {#if store.isProbing(ctxId)}<span class="spinner"></span>{:else}<Icon name="probe" size={13} />{/if}
                Probe it
              </button>
            {/if}
            <button class="btn small" onclick={() => store.setContext(undefined)}>Show all files</button>
          </div>
        </div>
      </div>
    {/if}
    <div class="view-host">
      {#if !store.data && store.loading}
        <div class="state"><span class="spinner"></span> Loading agent files</div>
      {:else if !store.data}
        <div class="state">
          <div class="banner error" role="alert">
            <div class="banner-body">
              Could not load the agent files: {store.loadError}
              <div class="banner-actions"><button class="btn small" onclick={() => store.load()}>Try again</button></div>
            </div>
          </div>
        </div>
      {:else if route.name === 'compare'}
        {#await loadCompare()}
          {@render loadingPart('Loading the editors')}
        {:then { default: CompareView }}
          <CompareView findingId={route.finding} />
        {:catch e}
          {@render loadFailed(e)}
        {/await}
      {:else if view === 'files'}
        <FilesView />
      {:else if view === 'graph'}
        {#await loadGraph()}
          {@render loadingPart('Loading the graph')}
        {:then { default: GraphView }}
          <GraphView />
        {:catch e}
          {@render loadFailed(e)}
        {/await}
      {:else}
        <FindingsView />
      {/if}
    </div>
  </main>

  {#if route.name === 'file' && store.data}
    <aside class="editor-pane">
      <button
        type="button"
        class="splitter"
        aria-label={`Resize the editor, now ${Math.round(editorWidth)} pixels wide. Use the left and right arrow keys.`}
        onpointerdown={startResize}
        onkeydown={onSplitterKey}
      ></button>
      {#await loadEditor()}
        {@render loadingPart('Loading the editor')}
      {:then { default: EditorPanel }}
        <EditorPanel fileId={route.id} line={route.line} />
      {:catch e}
        {@render loadFailed(e)}
      {/await}
    </aside>
  {/if}

  <nav class="tabs" aria-label="Views">
    {#each views as v (v.id)}
      <a href={viewHref(v.id)} aria-current={view === v.id && !compareOpen ? 'page' : undefined}>
        <span class="tab-icon">
          <Icon name={v.icon} size={18} />
          {#if v.id === 'findings' && counts.problem}<span class="pip">{counts.problem}</span>{/if}
        </span>
        <span>{v.label}</span>
      </a>
    {/each}
  </nav>
</div>

{#if proposalFinding}
  {#await loadProposal() then { default: ProposalDialog }}
    <ProposalDialog finding={proposalFinding} onclose={() => (store.proposalFor = null)} />
  {:catch e}
    {@render loadFailed(e)}
  {/await}
{/if}

<Toasts />

{#snippet loadingPart(text: string)}
  <div class="state"><span class="spinner"></span> {text}</div>
{/snippet}

{#snippet loadFailed(e: unknown)}
  <div class="state">
    <div class="banner error" role="alert">
      Part of the page failed to load: {errorMessage(e)}. Reload the page to try again.
    </div>
  </div>
{/snippet}

<style>
  .app {
    display: grid;
    height: 100dvh;
    grid-template-rows: auto minmax(0, 1fr);
    grid-template-columns: 176px minmax(0, 1fr);
    grid-template-areas:
      'top top'
      'rail main';
  }
  .app.with-editor {
    grid-template-columns: 176px minmax(0, 1fr) var(--editor-w);
    grid-template-areas:
      'top top top'
      'rail main editor';
  }

  .top {
    grid-area: top;
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr) auto minmax(0, 300px) auto;
    grid-template-areas: 'brand scan search harness picker theme';
    align-items: center;
    gap: 8px 12px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .brand {
    grid-area: brand;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .logo {
    display: inline-grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 6px;
    background: var(--accent);
    color: var(--surface);
    font: 700 11px/1 var(--font-mono);
  }
  .name {
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .owner {
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 12px;
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .scan {
    grid-area: scan;
    display: flex;
    align-items: center;
    gap: 2px;
    color: var(--text-3);
    font-size: 12px;
    white-space: nowrap;
  }
  .search {
    grid-area: search;
    justify-self: end;
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    max-width: 380px;
    height: 32px;
    padding: 0 6px 0 9px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-3);
  }
  .search:focus-within {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .search input {
    flex: 1;
    min-width: 0;
    height: 100%;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text);
  }
  .search input:focus-visible {
    outline: none;
  }
  .harness {
    grid-area: harness;
  }
  .harness select {
    height: 32px;
    max-width: 150px;
    padding: 0 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 13px;
  }
  .harness select:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .picker {
    grid-area: picker;
    min-width: 0;
    display: flex;
    justify-content: flex-end;
  }
  .theme {
    grid-area: theme;
  }

  .rail {
    grid-area: rail;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 8px;
    border-right: 1px solid var(--border);
    background: var(--surface);
    overflow: auto;
  }
  .rail a {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 10px;
    border-radius: var(--radius);
    color: var(--text-2);
    text-decoration: none;
    font-weight: 550;
  }
  .rail a:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .rail a[aria-current='page'] {
    background: var(--accent-bg);
    color: var(--accent-text);
  }
  .rail .label {
    flex: 1;
  }
  .rail .n {
    color: var(--text-3);
    font-size: 12px;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }
  .counts {
    display: inline-flex;
    gap: 3px;
  }

  .main {
    grid-area: main;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    background: var(--bg);
  }
  .view-host {
    flex: 1;
    min-height: 0;
    position: relative;
  }
  .ctx-missing {
    margin: 10px 12px 0;
  }
  .state {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    height: 100%;
    padding: 16px;
    color: var(--text-2);
  }

  .editor-pane {
    grid-area: editor;
    position: relative;
    min-width: 0;
    min-height: 0;
    border-left: 1px solid var(--border);
    background: var(--surface);
  }
  .splitter {
    position: absolute;
    left: -4px;
    top: 0;
    bottom: 0;
    width: 8px;
    z-index: 3;
    padding: 0;
    border: 0;
    background: transparent;
    cursor: col-resize;
    touch-action: none;
  }
  .splitter:hover,
  .splitter:focus-visible {
    background: linear-gradient(to right, transparent 3px, var(--accent) 3px, var(--accent) 5px, transparent 5px);
    outline: none;
  }

  .tabs {
    display: none;
  }

  @media (max-width: 1099px) {
    .app.with-editor {
      grid-template-columns: 176px minmax(0, 1fr);
      grid-template-areas:
        'top top'
        'rail main';
    }
    .editor-pane {
      grid-area: main;
      z-index: 5;
      border-left: 0;
    }
    .splitter {
      display: none;
    }
    .top {
      grid-template-columns: auto auto minmax(0, 1fr) auto minmax(0, 240px) auto;
    }
  }

  @media (max-width: 799px) {
    .app,
    .app.with-editor {
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto minmax(0, 1fr) auto;
      grid-template-areas:
        'top'
        'main'
        'tabs';
    }
    .top {
      grid-template-columns: minmax(0, 1fr) auto auto;
      grid-template-areas:
        'brand scan theme'
        'search search search'
        'picker harness harness';
      padding: 6px 12px 8px;
      gap: 6px 8px;
    }
    .search {
      max-width: none;
      justify-self: stretch;
    }
    .search kbd {
      display: none;
    }
    .picker {
      justify-content: stretch;
    }
    .picker :global(.picker),
    .picker :global(.trigger) {
      width: 100%;
    }
    .rail {
      display: none;
    }
    .editor-pane {
      position: fixed;
      inset: 0;
      z-index: 30;
    }
    .tabs {
      grid-area: tabs;
      display: flex;
      border-top: 1px solid var(--border);
      background: var(--surface);
      padding-bottom: env(safe-area-inset-bottom);
    }
    .tabs a {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 2px;
      padding: 7px 4px 6px;
      color: var(--text-2);
      font-size: 11.5px;
      text-decoration: none;
    }
    .tabs a[aria-current='page'] {
      color: var(--accent-text);
      font-weight: 600;
    }
    .tab-icon {
      position: relative;
      display: inline-flex;
    }
    .pip {
      position: absolute;
      top: -5px;
      right: -12px;
      min-width: 16px;
      height: 16px;
      padding: 0 4px;
      border-radius: 8px;
      background: var(--problem);
      color: #fff;
      font-size: 10px;
      font-weight: 700;
      line-height: 16px;
      text-align: center;
    }
  }
</style>
