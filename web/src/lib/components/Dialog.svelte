<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    title: string;
    onclose: () => void;
    children: Snippet;
    footer?: Snippet;
    wide?: boolean;
  }
  let { title, onclose, children, footer, wide = false }: Props = $props();

  let dialog: HTMLDialogElement | undefined = $state();
  const uid = $props.id();

  onMount(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog?.showModal();
    return () => {
      if (dialog?.open) dialog.close();
      opener?.focus();
    };
  });

  function onCancel(e: Event) {
    // Escape: let the parent decide, so it can unmount the dialog.
    e.preventDefault();
    onclose();
  }
</script>

<dialog bind:this={dialog} class:wide aria-labelledby={`${uid}-title`} oncancel={onCancel}>
  <div class="frame">
    <header>
      <h2 id={`${uid}-title`}>{title}</h2>
      <button type="button" class="icon-btn" aria-label="Close" onclick={onclose}><Icon name="close" /></button>
    </header>
    <div class="body">
      {@render children()}
    </div>
    {#if footer}
      <footer>{@render footer()}</footer>
    {/if}
  </div>
</dialog>

<style>
  dialog {
    width: min(640px, calc(100vw - 24px));
    max-height: calc(100dvh - 24px);
    padding: 0;
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow);
  }
  dialog.wide {
    width: min(1100px, calc(100vw - 24px));
  }
  dialog::backdrop {
    background: rgba(10, 10, 14, 0.45);
  }
  .frame {
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - 26px);
  }
  header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px 10px 16px;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    flex: 1;
    margin: 0;
    font-size: 15px;
    font-weight: 650;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 14px 16px;
  }
  footer {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
    padding: 10px 16px;
    border-top: 1px solid var(--border);
  }
  @media (max-width: 799px) {
    dialog,
    dialog.wide {
      width: 100vw;
      max-width: 100vw;
      height: 100dvh;
      max-height: 100dvh;
      margin: 0;
      border-radius: 0;
      border: 0;
    }
    .frame {
      height: 100dvh;
      max-height: 100dvh;
    }
  }
</style>
