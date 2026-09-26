<script lang="ts">
  import { analysisStatus, candidateId, groupContexts } from '../contexts';
  import { formatTime, plural, relativeTime } from '../format';
  import { store } from '../store.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    helpId: string;
  }
  let { helpId }: Props = $props();

  const groups = $derived(store.data ? groupContexts(store.data) : []);
  const cli = $derived(store.data?.analysis ?? '');
  const probingAll = $derived(store.probing.has('*'));
  const hasProbed = $derived(groups.some((g) => g.probe));
</script>

<section class="contexts" aria-labelledby="contexts-title">
  <header>
    <h2 id="contexts-title">Contexts</h2>
    {#if hasProbed}
      <button
        class="btn small"
        onclick={() => store.probe()}
        disabled={store.probing.size > 0}
        title="Start Claude Code and Codex in every directory to record what they load"
      >
        {#if probingAll}<span class="spinner"></span>{:else}<Icon name="probe" size={14} />{/if}
        Probe all
      </button>
    {/if}
  </header>
  {#if cli}
    <p class="help faint">Analysis runs {cli} on the files a context loads, to find contradictions and reworded duplicates.</p>
  {:else}
    <p class="banner" id={helpId}>
      Analysis and fix proposals need the claude or codex CLI, and agentmd found neither on this machine. The findings from rules
      still work.
    </p>
  {/if}

  {#each groups as g (g.harness)}
    <h3>{g.label}{g.version ? ` ${g.version}` : ''}</h3>
    <ul>
      {#each g.contexts as c (c.id)}
        {@const status = analysisStatus(c, store.analyseJobs[c.id])}
        <li class:current={store.context?.id === c.id}>
          <div class="line">
            <button
              class="ctx-link mono"
              onclick={() => store.setContext(store.context?.id === c.id ? undefined : c.id)}
              aria-pressed={store.context?.id === c.id}
              title="Show only what loads here"
            >
              {c.display}
            </button>
            <span class="tag" class:static={c.source === 'static'}>{c.source === 'probe' ? 'probed' : 'static'}</span>
            <button
              class="btn small action"
              disabled={!cli || status.state === 'running'}
              aria-describedby={cli ? undefined : helpId}
              aria-label={`Analyse ${g.label} in ${c.display}`}
              onclick={() => store.analyse(c.id)}
            >
              {#if status.state === 'running'}<span class="spinner"></span>{:else}<Icon name="play" size={13} />{/if}
              Analyse
            </button>
          </div>
          <div class="status">
            {#if c.error}
              <span class="error-text">The probe failed: {c.error}</span>
            {:else if status.state === 'never'}
              <span class="faint">Not analysed yet</span>
            {:else if status.state === 'running'}
              <span>Analysing, started {relativeTime(status.since, store.now)}</span>
            {:else if status.state === 'error'}
              <span class="error-text">Failed: {status.message}</span>
            {:else if status.state === 'stale'}
              <span class="stale" title={formatTime(status.at)}>
                Stale: files changed since the analysis {relativeTime(status.at, store.now)}
              </span>
            {:else}
              <span title={formatTime(status.at)}>
                Analysed {relativeTime(status.at, store.now)}{status.cli ? ` with ${status.cli}` : ''}, {plural(status.findings, 'finding')}
              </span>
            {/if}
          </div>
        </li>
      {/each}
      {#each g.unprobed as c (c.dir)}
        {@const id = candidateId(c)}
        {@const busy = store.isProbing(id)}
        <li class="unprobed">
          <div class="line">
            <span class="ctx-name mono">{c.display}</span>
            <span class="tag none">not probed</span>
            <button
              class="btn small action"
              disabled={busy}
              aria-label={`Probe ${g.label} in ${c.display}`}
              onclick={() => store.probe([id])}
            >
              {#if busy}<span class="spinner"></span>{:else}<Icon name="probe" size={13} />{/if}
              Probe
            </button>
          </div>
          <div class="status faint">
            {busy ? `Starting ${g.label} here to record what it loads` : 'Not probed yet, so what loads here is unknown'}
          </div>
        </li>
      {/each}
    </ul>
  {/each}
</section>

<style>
  .contexts {
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    align-self: start;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  h2 {
    margin: 0;
    font-size: 14px;
  }
  .help {
    margin: 6px 0 4px;
    font-size: 12px;
  }
  .banner {
    margin: 8px 0 4px;
  }
  h3 {
    margin: 12px 0 4px;
    font-size: 11.5px;
    font-weight: 650;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    padding: 6px 0;
    border-top: 1px solid var(--border);
  }
  li.current .ctx-link {
    color: var(--accent-text);
    font-weight: 700;
  }
  .line {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .ctx-link,
  .ctx-name {
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text);
    font-size: 12.5px;
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ctx-link {
    cursor: pointer;
  }
  .ctx-link:hover {
    text-decoration: underline;
  }
  .unprobed .ctx-name {
    color: var(--text-2);
  }
  .tag {
    flex: none;
    padding: 0 5px;
    border-radius: 3px;
    background: var(--ok-bg);
    color: var(--ok-text);
    font-size: 10.5px;
    line-height: 17px;
  }
  .tag.static {
    background: var(--surface-3);
    color: var(--text-2);
  }
  .tag.none {
    background: var(--hint-bg);
    color: var(--hint-text);
  }
  .action {
    margin-left: auto;
  }
  .status {
    margin-top: 2px;
    font-size: 12px;
    color: var(--text-2);
  }
  .status.faint {
    color: var(--text-3);
  }
  .stale {
    color: var(--hint-text);
  }
</style>
