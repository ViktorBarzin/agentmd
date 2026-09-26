<script lang="ts">
  import {
    EMPTY_FILTERS,
    KIND_LABELS,
    KIND_ORDER,
    countFindings,
    filterFindings,
    findingHarnesses,
    sortFindings,
    type FindingFilters,
  } from '../findings';
  import { plural } from '../format';
  import { store } from '../store.svelte';
  import type { AgentFile, Finding, RuntimeContext } from '../types';
  import ContextsPanel from './ContextsPanel.svelte';
  import FindingRow from './FindingRow.svelte';
  import Icon from './Icon.svelte';

  let filters = $state<FindingFilters>({ ...EMPTY_FILTERS });
  /** On narrow screens the chips fold away behind a toggle. */
  let filtersOpen = $state(false);
  const helpId = $props.id();

  const files = $derived(store.index?.files ?? new Map<string, AgentFile>());
  const contexts = $derived(store.index?.contexts ?? new Map<string, RuntimeContext>());
  const searching = $derived(store.query.trim() !== '');
  const scoped = $derived<Finding[]>(
    searching ? store.contextFindings.filter((f) => f.spans.some((s) => store.queryMatch(s.fileId))) : store.contextFindings,
  );
  const counts = $derived(countFindings(scoped));
  const shown = $derived(sortFindings(filterFindings(scoped, filters, { contexts, files }), files));
  const problems = $derived(shown.filter((f) => f.severity === 'problem'));
  const hints = $derived(shown.filter((f) => f.severity === 'hint'));
  const harnesses = $derived(store.data?.harnesses ?? []);
  const harnessCounts = $derived.by(() => {
    const out = new Map<string, number>();
    for (const f of scoped) for (const h of findingHarnesses(f, contexts, files)) out.set(h, (out.get(h) ?? 0) + 1);
    return out;
  });
  const sourceCounts = $derived({
    rule: scoped.filter((f) => f.source === 'rule').length,
    analysis: scoped.filter((f) => f.source === 'analysis').length,
  });
  const activeFilters = $derived(
    filters.severities.length + filters.kinds.length + filters.harnesses.length + filters.sources.length,
  );
  const filtering = $derived(activeFilters > 0 || filters.text.trim() !== '');
  const canFix = $derived(Boolean(store.data?.analysis));

  function toggle<K extends 'severities' | 'kinds' | 'harnesses' | 'sources'>(key: K, value: FindingFilters[K][number]) {
    const list = filters[key] as Array<typeof value>;
    const next = list.includes(value) ? list.filter((x) => x !== value) : [...list, value];
    filters = { ...filters, [key]: next };
  }
</script>

<div class="findings-view">
  <div class="layout">
    <div class="list-col">
      <div class="filters" class:collapsed={!filtersOpen}>
        <button
          type="button"
          class="btn small filters-toggle"
          aria-expanded={filtersOpen}
          onclick={() => (filtersOpen = !filtersOpen)}
        >
          <Icon name={filtersOpen ? 'chevron-down' : 'chevron-right'} size={14} />
          Filters{activeFilters ? ` (${activeFilters} on)` : ''}
        </button>
        <div class="chips" role="group" aria-label="Kind">
          {#each KIND_ORDER as k (k)}
            {#if counts.byKind[k] > 0 || filters.kinds.includes(k)}
              <button class="chip" aria-pressed={filters.kinds.includes(k)} onclick={() => toggle('kinds', k)}>
                {KIND_LABELS[k]} <span class="n">{counts.byKind[k]}</span>
              </button>
            {/if}
          {/each}
        </div>
        <div class="chips">
          <span class="group" role="group" aria-label="Severity">
            <button class="chip" aria-pressed={filters.severities.includes('problem')} onclick={() => toggle('severities', 'problem')}>
              <span class="dot problem" aria-hidden="true"></span>Problems <span class="n">{counts.problem}</span>
            </button>
            <button class="chip" aria-pressed={filters.severities.includes('hint')} onclick={() => toggle('severities', 'hint')}>
              <span class="dot hint" aria-hidden="true"></span>Hints <span class="n">{counts.hint}</span>
            </button>
          </span>
          <span class="group" role="group" aria-label="Harness">
            {#each harnesses as h (h.name)}
              <button class="chip" aria-pressed={filters.harnesses.includes(h.name)} onclick={() => toggle('harnesses', h.name)}>
                {h.label} <span class="n">{harnessCounts.get(h.name) ?? 0}</span>
              </button>
            {/each}
          </span>
          <span class="group" role="group" aria-label="Source">
            <button class="chip" aria-pressed={filters.sources.includes('rule')} onclick={() => toggle('sources', 'rule')}>
              Rules <span class="n">{sourceCounts.rule}</span>
            </button>
            <button class="chip" aria-pressed={filters.sources.includes('analysis')} onclick={() => toggle('sources', 'analysis')}>
              Analysis <span class="n">{sourceCounts.analysis}</span>
            </button>
          </span>
        </div>
        <div class="search-row">
          <label class="search">
            <Icon name="search" size={14} />
            <span class="sr-only">Search findings</span>
            <input type="search" placeholder="Search findings" bind:value={filters.text} />
          </label>
          {#if filtering}
            <button class="btn small ghost" onclick={() => (filters = { ...EMPTY_FILTERS })}>Clear filters</button>
          {/if}
        </div>
        {#if store.context || searching}
          <p class="scope faint">
            {#if store.context}Showing findings for <span class="mono">{store.context.display}</span>.{/if}
            {#if searching}Only findings in files that match the search.{/if}
          </p>
        {/if}
      </div>

      {#if scoped.length === 0}
        <div class="empty">
          <strong>No findings{store.context ? ' in this context' : ''}.</strong>
          {store.context || searching ? 'Nothing to clean up here.' : 'The agent files look tidy.'}
        </div>
      {:else if shown.length === 0}
        <div class="empty">
          <strong>No findings match these filters.</strong>
          <button class="btn small" onclick={() => (filters = { ...EMPTY_FILTERS })}>Clear filters</button>
        </div>
      {:else}
        {#if problems.length}
          <h2>Problems <span class="n">{problems.length}</span></h2>
          <p class="explain faint">An agent reads both sides of these in the same session.</p>
          <ul>
            {#each problems as f (f.id)}
              <li><FindingRow finding={f} {canFix} fixHelpId={helpId} /></li>
            {/each}
          </ul>
        {/if}
        {#if hints.length}
          <h2>Hints <span class="n">{hints.length}</span></h2>
          <p class="explain faint">Worth a look, though no single session reads both sides.</p>
          <ul>
            {#each hints as f (f.id)}
              <li><FindingRow finding={f} {canFix} fixHelpId={helpId} /></li>
            {/each}
          </ul>
        {/if}
        <p class="faint total">{plural(shown.length, 'finding')} shown</p>
      {/if}
    </div>
    <ContextsPanel {helpId} />
  </div>
</div>

<style>
  .findings-view {
    height: 100%;
    overflow: auto;
    container-type: inline-size;
  }
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
    align-items: start;
    padding: 12px 12px 32px;
  }
  @container (min-width: 880px) {
    .layout {
      grid-template-columns: minmax(0, 1fr) 320px;
    }
  }
  .list-col {
    min-width: 0;
  }
  .filters {
    display: grid;
    gap: 8px;
    margin-bottom: 4px;
  }
  .filters-toggle {
    display: none;
    justify-self: start;
  }
  @container (max-width: 599px) {
    .filters-toggle {
      display: inline-flex;
    }
    .filters.collapsed .chips {
      display: none;
    }
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 12px;
  }
  .chips[aria-label='Kind'] {
    gap: 6px;
  }
  .group {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .search-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    max-width: 420px;
    padding-left: 8px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text-3);
  }
  .search input {
    flex: 1;
    border: 0;
    background: transparent;
    padding-left: 0;
  }
  .search input:focus-visible {
    outline: none;
  }
  .search:focus-within {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .scope {
    margin: 0;
    font-size: 12.5px;
  }
  h2 {
    margin: 16px 0 2px;
    font-size: 14px;
  }
  h2 .n {
    color: var(--text-3);
    font-weight: 500;
  }
  .explain {
    margin: 0 0 8px;
    font-size: 12.5px;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 8px;
  }
  .total {
    margin: 12px 0 0;
    font-size: 12px;
  }
</style>
