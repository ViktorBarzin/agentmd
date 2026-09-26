<script lang="ts">
  import { probeBacklog } from '../contexts';
  import { plural } from '../format';
  import { readStored, writeStored } from '../storage';
  import { store } from '../store.svelte';
  import { groupFiles, rowLabel } from '../tree';
  import ContextSummary from './ContextSummary.svelte';
  import FileRow from './FileRow.svelte';
  import Icon from './Icon.svelte';

  const KEY = 'agentmd.collapsed';

  function readCollapsed(): Set<string> {
    try {
      const raw = readStored(KEY);
      const parsed: unknown = raw ? JSON.parse(raw) : [];
      return new Set(Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : []);
    } catch {
      return new Set();
    }
  }

  let collapsed = $state<Set<string>>(readCollapsed());

  function toggle(id: string) {
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
    writeStored(KEY, JSON.stringify([...next]));
  }

  const home = $derived(store.data?.home ?? '');
  const order = $derived.by(() => {
    const m = store.members;
    if (!m) return undefined;
    const out = new Map<string, number>();
    for (const [id, x] of m) if (x.position !== undefined) out.set(id, x.position);
    return out;
  });
  const matching = $derived(store.visibleFiles.filter((f) => store.queryMatch(f.id)));
  const groups = $derived(groupFiles(matching, home, order));
  const currentId = $derived(store.route.name === 'file' ? store.route.id : null);
  const searching = $derived(store.query.trim() !== '');
  const backlog = $derived(store.data ? probeBacklog(store.data) : { unprobed: 0, stale: 0 });
</script>

<div class="files-view">
  {#if store.context}
    <ContextSummary context={store.context} />
  {:else if (backlog.unprobed > 0 || backlog.stale > 0 || store.probingAll) && !searching}
    <div class="banner probe-note" role="status">
      <Icon name="probe" />
      <div class="banner-body">
        {#if store.probingAll}
          {store.probeStatus}. Claude Code and Codex start in each directory to record what they load.
        {:else if backlog.unprobed && backlog.stale}
          {plural(backlog.unprobed, 'directory', 'directories')} not probed yet and {plural(backlog.stale, 'context')} out of
          date, so what loads there is unknown or may have changed.
        {:else if backlog.unprobed}
          Claude Code or Codex has not been probed in {plural(backlog.unprobed, 'directory', 'directories')} yet, so what loads
          there is unknown.
        {:else}
          {plural(backlog.stale, 'probed context')} out of date: a file {backlog.stale === 1 ? 'it loads' : 'they load'} changed
          since the probe.
        {/if}
        <div class="banner-actions">
          <button class="btn small" disabled={store.probingAll} onclick={() => store.probe()}>
            {#if store.probingAll}<span class="spinner"></span>{:else}<Icon name="probe" size={13} />{/if}
            Probe all
          </button>
        </div>
      </div>
    </div>
  {/if}

  <div class="status" aria-live="polite">
    {#if searching}
      {plural(matching.length, 'file')} match "{store.query.trim()}"
      <button class="btn small ghost" onclick={() => (store.query = '')}>Clear search</button>
    {:else if store.context}
      {plural(matching.length, 'file')} belong to this context
    {:else}
      {plural(matching.length, 'agent file')}
    {/if}
  </div>

  {#each groups as g (g.id)}
    {@const isOpen = searching || !collapsed.has(g.id)}
    <section class="group">
      <h2>
        <button type="button" aria-expanded={isOpen} onclick={() => toggle(g.id)} disabled={searching}>
          <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} size={14} />
          <span class="glabel" class:mono={g.root !== undefined}>{g.label}</span>
          <span class="count">{g.files.length}</span>
        </button>
      </h2>
      {#if isOpen}
        <ul>
          {#each g.files as f (f.id)}
            <li>
              <FileRow
                file={f}
                label={rowLabel(f, g, home)}
                href={store.hrefFor(store.fileRoute(f.id))}
                current={currentId === f.id}
                member={store.members?.get(f.id) ?? null}
                counts={store.index?.counts.get(f.id)}
                dirty={store.dirty.has(f.id)}
              />
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {:else}
    <div class="empty">
      {#if searching}
        <strong>No file matches "{store.query.trim()}".</strong>
        Try part of a path, a skill name or a kind such as "skill".
      {:else if store.context}
        <strong>Nothing loads in this context.</strong>
        Pick another context, or All files.
      {:else}
        <strong>No agent files found.</strong>
        agentmd looked in {store.data?.roots.join(', ') || 'no discovery roots'}. Add a root in ~/.config/agentmd/config.toml.
      {/if}
    </div>
  {/each}
</div>

<style>
  .files-view {
    height: 100%;
    overflow: auto;
    padding-bottom: 24px;
  }
  .probe-note {
    margin: 12px 12px 0;
  }
  .probe-note :global(svg) {
    margin-top: 2px;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 36px;
    padding: 6px 16px 2px;
    color: var(--text-2);
    font-size: 12.5px;
  }
  .group {
    margin-top: 6px;
  }
  h2 {
    margin: 0;
    font-size: 12px;
  }
  h2 button {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 6px 12px;
    border: 0;
    background: transparent;
    color: var(--text-2);
    font-weight: 600;
    text-align: left;
    cursor: pointer;
    letter-spacing: 0.02em;
  }
  h2 button:disabled {
    cursor: default;
  }
  h2 button:hover:not(:disabled) {
    color: var(--text);
  }
  .glabel {
    text-transform: uppercase;
  }
  .glabel.mono {
    text-transform: none;
    font-size: 12.5px;
  }
  .count {
    color: var(--text-3);
    font-weight: 500;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
