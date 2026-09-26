<script lang="ts">
  import { lintGutter, setDiagnostics } from '@codemirror/lint';
  import { Compartment, EditorState, Prec } from '@codemirror/state';
  import { EditorView, keymap } from '@codemirror/view';
  import { onMount, untrack } from 'svelte';
  import { editorBasics, findingDiagnostics, showSpan, spanField, type LineSpan } from '../cm';
  import type { Finding } from '../types';

  interface Props {
    /** The document to show; it replaces the editor's text when resetKey changes. */
    doc: string;
    resetKey: number;
    readOnly: boolean;
    fileId: string;
    findings: Finding[];
    span: LineSpan | null;
    /** Changes whenever the span should be scrolled to again. */
    scrollKey: string;
    label: string;
    onchange: (doc: string) => void;
    onsave: () => void;
    onfocus?: () => void;
  }
  let { doc, resetKey, readOnly, fileId, findings, span, scrollKey, label, onchange, onsave, onfocus }: Props = $props();

  let host: HTMLDivElement | undefined = $state();
  let view = $state.raw<EditorView | null>(null);
  let spanShownAt = 0;
  const readOnlyConf = new Compartment();

  onMount(() => {
    if (!host) return;
    const initial = untrack(() => ({ doc, readOnly, label }));
    const v = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: initial.doc,
        extensions: [
          Prec.highest(
            keymap.of([
              {
                key: 'Mod-s',
                preventDefault: true,
                run: () => {
                  onsave();
                  return true;
                },
              },
            ]),
          ),
          editorBasics(),
          lintGutter(),
          spanField,
          readOnlyConf.of(EditorState.readOnly.of(initial.readOnly)),
          EditorView.contentAttributes.of({ 'aria-label': initial.label }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) onchange(u.state.doc.toString());
            if (u.focusChanged && u.view.hasFocus) onfocus?.();
          }),
        ],
      }),
    });
    view = v;
    // Panels around the editor can open just after a span is shown (the git
    // status arrives later), so keep the span in view while the size settles.
    const ro = new ResizeObserver(() => {
      if (span && performance.now() - spanShownAt < 1500) showSpan(v, span);
    });
    ro.observe(host);
    return () => {
      ro.disconnect();
      v.destroy();
      view = null;
    };
  });

  // Replace the text when the parent asks: a load, a revert, taking the disk version.
  $effect(() => {
    void resetKey;
    const v = view;
    if (!v) return;
    const next = untrack(() => doc);
    if (v.state.doc.toString() !== next) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: next } });
    }
  });

  $effect(() => {
    const v = view;
    const ro = readOnly;
    if (v) v.dispatch({ effects: readOnlyConf.reconfigure(EditorState.readOnly.of(ro)) });
  });

  $effect(() => {
    // A document reset maps the old diagnostics away, so recompute after one.
    void resetKey;
    const v = view;
    const list = findings;
    const id = fileId;
    if (v) v.dispatch(setDiagnostics(v.state, findingDiagnostics(v.state.doc, list, id)));
  });

  $effect(() => {
    void scrollKey;
    void resetKey;
    const v = view;
    const s = span;
    if (!v) return;
    showSpan(v, s);
    spanShownAt = performance.now();
  });
</script>

<div class="code" bind:this={host}></div>

<style>
  .code {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .code :global(.cm-editor) {
    flex: 1;
    min-height: 0;
  }
</style>
