<script lang="ts">
  import { MergeView, unifiedMergeView } from '@codemirror/merge';
  import { EditorState, type Extension } from '@codemirror/state';
  import { EditorView, lineNumbers } from '@codemirror/view';
  import { onMount, untrack } from 'svelte';
  import { markdownExtensions } from '../cm';

  interface Props {
    /** The old side: before a fix, or the file on disk. */
    a: string;
    /** The new side: after a fix, or your edits. */
    b: string;
    labelA: string;
    labelB: string;
    /** Lets the owner edit side b, for example to merge a conflict by hand. */
    editableB?: boolean;
    onchangeB?: (doc: string) => void;
  }
  let { a, b, labelA, labelB, editableB = false, onchangeB }: Props = $props();

  let host: HTMLDivElement | undefined = $state();
  let narrow = $state(false);

  onMount(() => {
    const mq = window.matchMedia('(max-width: 799px)');
    narrow = mq.matches;
    const onChange = (e: MediaQueryListEvent) => {
      narrow = e.matches;
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  });

  $effect(() => {
    const target = host;
    const unified = narrow;
    const docA = a;
    if (!target) return;
    // Only a new side a or a layout change rebuilds the view; side b is the
    // starting document and may be edited in place.
    const { docB, editable, nameA, nameB } = untrack(() => ({ docB: b, editable: editableB, nameA: labelA, nameB: labelB }));
    const common: Extension[] = [lineNumbers(), ...markdownExtensions()];
    const bExtensions: Extension[] = [
      ...common,
      EditorState.readOnly.of(!editable),
      EditorView.contentAttributes.of({ 'aria-label': nameB }),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) onchangeB?.(u.state.doc.toString());
      }),
    ];
    if (unified) {
      const view = new EditorView({
        parent: target,
        state: EditorState.create({
          doc: docB,
          extensions: [
            ...bExtensions,
            unifiedMergeView({ original: docA, mergeControls: false, collapseUnchanged: { margin: 3, minSize: 6 } }),
          ],
        }),
      });
      return () => view.destroy();
    }
    const view = new MergeView({
      parent: target,
      a: {
        doc: docA,
        extensions: [...common, EditorState.readOnly.of(true), EditorView.contentAttributes.of({ 'aria-label': nameA })],
      },
      b: { doc: docB, extensions: bExtensions },
      collapseUnchanged: { margin: 3, minSize: 6 },
      gutter: true,
      highlightChanges: true,
    });
    return () => view.destroy();
  });
</script>

<div class="merge">
  {#if narrow}
    <div class="labels unified">
      <span><span class="key del"></span>{labelA}</span>
      <span><span class="key add"></span>{labelB}</span>
    </div>
  {:else}
    <div class="labels">
      <span>{labelA}</span>
      <span>{labelB}</span>
    </div>
  {/if}
  <div class="host" bind:this={host}></div>
</div>

<style>
  .merge {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--editor-bg);
  }
  .labels {
    display: grid;
    grid-template-columns: 1fr 1fr;
    border-bottom: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 12px;
    font-weight: 600;
  }
  .labels span {
    padding: 4px 10px;
  }
  .labels.unified {
    display: flex;
    gap: 12px;
  }
  .labels.unified span {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .key {
    width: 10px;
    height: 10px;
    border-radius: 2px;
    padding: 0 !important;
  }
  .key.del {
    background: var(--diff-del-strong);
  }
  .key.add {
    background: var(--diff-add-strong);
  }
  .host {
    max-height: 55vh;
    overflow: auto;
  }
  .host :global(.cm-mergeView) {
    max-height: none;
  }
  .host :global(.cm-editor) {
    height: auto;
  }
</style>
