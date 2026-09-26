<script lang="ts">
  import { formatBytes, formatCount, relativeTime } from '../format';
  import { store } from '../store.svelte';
  import type { RuntimeContext } from '../types';

  interface Props {
    context: RuntimeContext;
  }
  let { context }: Props = $props();

  const harness = $derived(store.data?.harnesses.find((h) => h.name === context.harness));
  const files = $derived(store.index?.files);
</script>

<section class="summary" aria-label="Load order">
  <header>
    <div>
      <strong>{harness?.label ?? context.harness}</strong>
      <span class="mono">{context.display}</span>
    </div>
    <div class="meta">
      {#if context.source === 'probe'}
        probed {relativeTime(context.probedAt, store.now)}{context.version ? `, version ${context.version}` : ''}
      {:else}
        static: worked out from the documented loading rules
      {/if}
    </div>
  </header>
  {#if context.error}
    <p class="banner error">The probe failed: {context.error}</p>
  {/if}
  {#if context.entries.length === 0}
    <p class="muted">Nothing loads here at session start.</p>
  {:else}
    <ol>
      {#each context.entries as e, i (e.fileId + i)}
        {@const f = files?.get(e.fileId)}
        <li>
          <span class="pos">{i + 1}</span>
          <a class="mono" href={store.hrefFor(store.fileRoute(e.fileId))}>{f?.display ?? e.fileId}</a>
          {#if e.label}<span class="faint label">{e.label}</span>{/if}
          <span class="bytes" title={`${formatCount(e.bytes)} bytes loaded`}>{formatBytes(e.bytes)}</span>
          {#if e.truncated}
            <span class="badge problem" title={`${formatCount(e.lostBytes ?? 0)} bytes cut by the budget`}>
              cut {formatBytes(e.lostBytes ?? 0)}
            </span>
          {/if}
        </li>
      {/each}
    </ol>
    <div class="total faint">{formatBytes(context.bytes)} in total</div>
  {/if}
  {#if context.skills.length || context.subagents.length}
    <div class="offers">
      {#if context.skills.length}
        <span class="faint">Skills</span>
        {#each context.skills as s (s.name)}
          {#if s.fileId}
            <a href={store.hrefFor(store.fileRoute(s.fileId))}>{s.name}</a>
          {:else}
            <span>{s.name}</span>
          {/if}
        {/each}
      {/if}
      {#if context.subagents.length}
        <span class="faint">Subagents</span>
        {#each context.subagents as s (s.name)}
          {#if s.fileId}
            <a href={store.hrefFor(store.fileRoute(s.fileId))}>{s.name}</a>
          {:else}
            <span>{s.name}</span>
          {/if}
        {/each}
      {/if}
    </div>
  {/if}
</section>

<style>
  .summary {
    margin: 12px 12px 4px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  header {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 2px 12px;
    margin-bottom: 6px;
  }
  header .mono {
    margin-left: 6px;
    font-size: 12.5px;
  }
  .meta {
    color: var(--text-3);
    font-size: 12px;
  }
  ol {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 2px 8px;
    padding: 3px 0;
    font-size: 13px;
    min-width: 0;
  }
  .pos {
    display: inline-grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--accent-bg);
    color: var(--accent-text);
    font-size: 11px;
    font-weight: 700;
    flex: none;
  }
  li a {
    min-width: 0;
    overflow-wrap: anywhere;
    font-size: 12.5px;
    color: var(--text);
  }
  .label {
    font-size: 12px;
  }
  .bytes {
    margin-left: auto;
    color: var(--text-2);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }
  .total {
    margin-top: 2px;
    font-size: 12px;
    text-align: right;
  }
  .offers {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 10px;
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
  }
  .offers a {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  .offers span:not(.faint) {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  p {
    margin: 4px 0;
  }
</style>
