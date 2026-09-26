<script lang="ts">
  import { onMount } from 'svelte';
  import { api, errorMessage } from '../api';
  import { KIND_LABELS } from '../findings';
  import { plural } from '../format';
  import { store, waitForJob } from '../store.svelte';
  import type { Finding, Job } from '../types';
  import Dialog from './Dialog.svelte';
  import Icon from './Icon.svelte';
  import InlineText from './InlineText.svelte';
  import MergeView from './MergeView.svelte';

  interface Props {
    finding: Finding;
    onclose: () => void;
  }
  let { finding, onclose }: Props = $props();

  let job = $state.raw<Job | null>(null);
  let startError = $state<string | null>(null);
  let applying = $state(false);
  let applyError = $state<string | null>(null);
  let started = $state(Date.now());
  let now = $state(Date.now());
  let cancelled = false;

  const cli = $derived(store.data?.analysis || 'the agent CLI');
  const proposal = $derived(job?.status === 'done' ? (job.proposal ?? null) : null);
  const running = $derived(!startError && (!job || job.status === 'running'));

  async function start() {
    startError = null;
    job = null;
    started = Date.now();
    try {
      const first = await api.fix(finding.id);
      if (cancelled) return;
      job = first;
      const last = await waitForJob(first, (j) => {
        if (!cancelled) job = j;
      }, () => cancelled);
      if (!cancelled) job = last;
    } catch (e) {
      if (!cancelled) startError = errorMessage(e);
    }
  }

  onMount(() => {
    void start();
    const t = setInterval(() => {
      now = Date.now();
    }, 1000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  });

  async function apply() {
    if (!proposal || applying) return;
    applying = true;
    applyError = null;
    try {
      const res = await api.apply({
        edits: proposal.edits.map((e) => ({ id: e.fileId, content: e.after, baseHash: e.baseHash })),
      });
      const ids = res.results.map((r) => r.file.id);
      for (const id of ids) store.setDraft(id, null);
      store.requestReload(ids);
      await store.refresh();
      store.toast(`Applied the fix to ${plural(res.results.length, 'file')}.`, 'ok');
      onclose();
    } catch (e) {
      applyError = errorMessage(e);
    } finally {
      applying = false;
    }
  }
</script>

<Dialog title="Fix proposal" wide {onclose}>
  <div class="finding">
    <span class={`badge ${finding.severity}`}>{finding.severity === 'problem' ? 'Problem' : 'Hint'}</span>
    <span class="kind">{KIND_LABELS[finding.kind]}</span>
    <span class="summary"><InlineText text={finding.summary} /></span>
  </div>

  {#if startError}
    <div class="banner error" role="alert">
      <div class="banner-body">
        Could not ask for a fix: {startError}
        <div class="banner-actions"><button class="btn small" onclick={start}>Try again</button></div>
      </div>
    </div>
  {:else if running}
    <div class="waiting" role="status">
      <span class="spinner"></span>
      Asking {cli} for an edit. This usually takes a few seconds ({Math.max(0, Math.round((now - started) / 1000))} s so far).
    </div>
  {:else if job?.status === 'error'}
    <div class="banner warn" role="alert">
      <Icon name="info" />
      <div class="banner-body">
        <InlineText text={job.error || 'The agent did not propose a fix.'} />
        <div class="banner-actions"><button class="btn small" onclick={start}>Ask again</button></div>
      </div>
    </div>
  {:else if proposal}
    <p class="rationale"><InlineText text={proposal.rationale} /></p>
    {#if proposal.edits.length === 0}
      <p class="muted">The proposal changes no files.</p>
    {/if}
    {#each proposal.edits as e (e.fileId)}
      <section class="edit">
        <h3 class="mono">{e.display}</h3>
        <MergeView a={e.before} b={e.after} labelA="Before" labelB="After" />
      </section>
    {/each}
    {#if applyError}
      <div class="banner error" role="alert">The fix was not applied: {applyError}</div>
    {/if}
  {/if}

  {#snippet footer()}
    <span class="foot-note faint">
      {#if proposal}Apply saves {plural(proposal.edits.length, 'file')}. Nothing is committed.{/if}
    </span>
    <button class="btn" onclick={onclose}>{proposal ? 'Discard' : 'Close'}</button>
    <button class="btn primary" onclick={apply} disabled={!proposal || applying || proposal.edits.length === 0}>
      {#if applying}<span class="spinner"></span>{:else}<Icon name="check" size={14} />{/if}
      Apply
    </button>
  {/snippet}
</Dialog>

<style>
  .finding {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    margin-bottom: 12px;
  }
  .kind {
    color: var(--text-2);
    font-size: 12.5px;
    font-weight: 600;
  }
  .summary {
    flex: 1 1 100%;
    font-weight: 550;
  }
  .waiting {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 24px 4px;
    color: var(--text-2);
  }
  .rationale {
    margin: 0 0 12px;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: var(--surface-2);
    line-height: 1.5;
  }
  .edit {
    margin-top: 12px;
  }
  h3 {
    margin: 0 0 6px;
    font-size: 12.5px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .banner {
    margin-top: 10px;
  }
  .foot-note {
    margin-right: auto;
    align-self: center;
    font-size: 12px;
  }
</style>
