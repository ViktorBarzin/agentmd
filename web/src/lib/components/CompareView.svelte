<script lang="ts">
  import { KIND_LABELS } from '../findings';
  import { lineRange } from '../format';
  import { store } from '../store.svelte';
  import FileEditor from './FileEditor.svelte';
  import Icon from './Icon.svelte';
  import InlineText from './InlineText.svelte';

  interface Props {
    findingId: string;
  }
  let { findingId }: Props = $props();

  const finding = $derived(store.index?.findings.get(findingId) ?? null);
  const spans = $derived(finding?.spans.slice(0, 2) ?? []);
  const more = $derived(finding ? finding.spans.slice(2) : []);
  const canFix = $derived(Boolean(store.data?.analysis));
</script>

<div class="compare">
  <header>
    <button class="btn small ghost" onclick={() => store.closeEditor()}>
      <Icon name="back" size={14} />Back
    </button>
    {#if finding}
      <span class={`badge ${finding.severity}`}>{finding.severity === 'problem' ? 'Problem' : 'Hint'}</span>
      <span class="kind">{KIND_LABELS[finding.kind]}</span>
      <h2><InlineText text={finding.summary} /></h2>
      <button
        class="btn small"
        disabled={!canFix}
        title={canFix ? 'Ask the agent CLI for an edit that resolves this finding' : 'No agent CLI was found, so fix proposals are off'}
        onclick={() => (store.proposalFor = finding.id)}
      >
        <Icon name="wand" size={14} />Propose fix
      </button>
    {/if}
  </header>

  {#if !finding}
    <div class="empty">
      <strong>This finding is gone.</strong>
      It may have been fixed since the last scan.
    </div>
  {:else}
    {#if finding.detail}
      <p class="detail"><InlineText text={finding.detail} /></p>
    {/if}
    <div class="panes">
      {#each spans as s, i (i + s.fileId)}
        {@const f = store.index?.files.get(s.fileId)}
        <section class="pane" aria-label={`${i === 0 ? 'First' : 'Second'} side: ${f?.display ?? s.fileId}`}>
          <div class="pane-head">
            <span class="mono path">{f?.display ?? s.fileId}</span>
            <span class="faint">lines {lineRange(s.startLine, s.endLine)}</span>
            <a class="open" href={store.hrefFor({ ...store.fileRoute(s.fileId, s.startLine), view: 'findings' })}>Open in the editor</a>
          </div>
          <FileEditor
            fileId={s.fileId}
            span={{ start: s.startLine, end: s.endLine }}
            scrollKey={`${findingId}:${i}:${s.startLine}`}
            compact
          />
        </section>
      {/each}
    </div>
    {#if more.length}
      <p class="more faint">
        Also in:
        {#each more as s, i (i)}
          <a href={store.hrefFor(store.fileRoute(s.fileId, s.startLine))}>
            {store.index?.files.get(s.fileId)?.display ?? s.fileId}:{lineRange(s.startLine, s.endLine)}
          </a>
        {/each}
      </p>
    {/if}
  {/if}
</div>

<style>
  .compare {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .kind {
    color: var(--text-2);
    font-size: 12.5px;
  }
  h2 {
    flex: 1 1 18rem;
    margin: 0;
    font-size: 14px;
    font-weight: 600;
  }
  .detail {
    margin: 0;
    padding: 8px 12px;
    color: var(--text-2);
    font-size: 13px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .panes {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 1fr 1fr;
  }
  .pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    border-right: 1px solid var(--border);
  }
  .pane:last-child {
    border-right: 0;
  }
  .pane-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 10px;
    padding: 6px 12px;
    background: var(--surface-2);
    border-bottom: 1px solid var(--border);
    font-size: 12.5px;
  }
  .path {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .open {
    margin-left: auto;
    font-size: 12px;
  }
  .more {
    margin: 0;
    padding: 6px 12px;
    font-size: 12.5px;
  }
  @media (max-width: 1099px) {
    .panes {
      grid-template-columns: 1fr;
      grid-template-rows: 1fr 1fr;
    }
    .pane {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
  }
  /* On a phone the page scrolls and each side gets most of a screen. */
  @media (max-width: 799px) {
    .compare {
      overflow: auto;
    }
    .panes {
      display: block;
      flex: none;
    }
    .pane {
      height: 72dvh;
    }
  }
</style>
