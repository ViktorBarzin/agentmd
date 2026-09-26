<script lang="ts">
  import { KIND_LABELS, isCompare } from '../findings';
  import { lineRange } from '../format';
  import { store } from '../store.svelte';
  import type { Finding } from '../types';
  import Icon from './Icon.svelte';
  import InlineText from './InlineText.svelte';

  interface Props {
    finding: Finding;
    canFix: boolean;
    fixHelpId: string;
  }
  let { finding, canFix, fixHelpId }: Props = $props();

  const files = $derived(store.index?.files);
  const contexts = $derived(store.index?.contexts);
  const harnessLabel = (name: string) => store.data?.harnesses.find((h) => h.name === name)?.label ?? name;
</script>

<article class="finding" class:problem={finding.severity === 'problem'}>
  <a class="main" href={store.hrefFor(store.findingRoute(finding))}>
    <span class="top">
      <span class={`badge ${finding.severity}`}>{finding.severity === 'problem' ? 'Problem' : 'Hint'}</span>
      <span class="kind">{KIND_LABELS[finding.kind]}</span>
      <span class="source faint">{finding.source === 'analysis' ? 'from analysis' : 'from rules'}</span>
      {#if isCompare(finding)}
        <span class="faint mode"><Icon name="compare" size={13} />side by side</span>
      {/if}
    </span>
    <span class="summary"><InlineText text={finding.summary} /></span>
    <span class="spans">
      {#each finding.spans as s, i (i)}
        <span class="span mono">{files?.get(s.fileId)?.display ?? s.fileId}:{lineRange(s.startLine, s.endLine)}</span>
      {/each}
    </span>
    {#if finding.detail}
      <span class="detail"><InlineText text={finding.detail} /></span>
    {/if}
    {#if finding.contexts?.length}
      <span class="contexts faint">
        Seen in
        {#each finding.contexts as id, i (id)}
          {@const c = contexts?.get(id)}
          {i > 0 ? ', ' : ''}{c ? `${harnessLabel(c.harness)} ${c.display}` : id}
        {/each}
      </span>
    {/if}
  </a>
  <div class="actions">
    <button
      class="btn small"
      disabled={!canFix}
      aria-describedby={canFix ? undefined : fixHelpId}
      onclick={() => (store.proposalFor = finding.id)}
    >
      <Icon name="wand" size={14} />Propose fix
    </button>
  </div>
</article>

<style>
  .finding {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-left: 3px solid var(--hint);
    border-radius: var(--radius);
    background: var(--surface);
  }
  .finding.problem {
    border-left-color: var(--problem);
  }
  .main {
    display: grid;
    gap: 4px;
    flex: 1;
    min-width: 0;
    color: var(--text);
    text-decoration: none;
  }
  .main:hover .summary {
    text-decoration: underline;
  }
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    font-size: 12px;
  }
  .kind {
    font-weight: 600;
    color: var(--text-2);
  }
  .mode {
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }
  .summary {
    font-weight: 550;
  }
  .spans {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 10px;
  }
  .span {
    font-size: 12px;
    color: var(--accent-text);
    overflow-wrap: anywhere;
  }
  .detail {
    color: var(--text-2);
    font-size: 12.5px;
  }
  .contexts {
    font-size: 12px;
  }
  .actions {
    flex: none;
  }
  @media (max-width: 599px) {
    .finding {
      flex-direction: column;
    }
  }
</style>
