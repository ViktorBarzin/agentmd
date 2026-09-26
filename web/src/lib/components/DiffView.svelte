<script lang="ts">
  import { parseDiff } from '../diff';

  interface Props {
    diff: string;
    label?: string;
  }
  let { diff, label = 'Changes' }: Props = $props();

  const lines = $derived(parseDiff(diff));
</script>

<div class="diff" role="region" aria-label={label}>
  <div class="inner">
    {#each lines as l, i (i)}
      <div class={`dl ${l.kind}`}>
        <span class="no" aria-hidden="true">{l.oldLine ?? ''}</span>
        <span class="no" aria-hidden="true">{l.newLine ?? ''}</span>
        <span class="tx">{l.text || ' '}</span>
      </div>
    {/each}
  </div>
</div>

<style>
  .diff {
    max-height: 320px;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--editor-bg);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.5;
  }
  .inner {
    display: table;
    min-width: 100%;
  }
  .dl {
    display: table-row;
  }
  .dl > span {
    display: table-cell;
    white-space: pre;
  }
  .no {
    width: 1%;
    padding: 0 6px;
    color: var(--text-3);
    text-align: right;
    user-select: none;
    border-right: 1px solid var(--border);
  }
  .tx {
    padding: 0 10px 0 8px;
  }
  .add {
    background: var(--diff-add-bg);
  }
  .add .tx {
    color: var(--diff-add-text);
  }
  .del {
    background: var(--diff-del-bg);
  }
  .del .tx {
    color: var(--diff-del-text);
  }
  .hunk {
    background: var(--surface-2);
  }
  .hunk .tx {
    color: var(--diff-hunk);
  }
  .meta .tx {
    color: var(--text-3);
  }
  .note .tx {
    color: var(--text-3);
    font-style: italic;
  }
</style>
