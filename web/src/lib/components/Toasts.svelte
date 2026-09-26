<script lang="ts">
  import { store } from '../store.svelte';
  import Icon from './Icon.svelte';
</script>

<div class="toasts" role="status" aria-live="polite">
  {#each store.toasts as t (t.id)}
    <div class={`toast ${t.tone}`}>
      <span class="text">{t.text}</span>
      <button class="icon-btn" aria-label="Dismiss" onclick={() => store.dismiss(t.id)}><Icon name="close" size={14} /></button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    left: 50%;
    bottom: 16px;
    z-index: 100;
    display: grid;
    gap: 8px;
    width: min(460px, calc(100vw - 24px));
    transform: translateX(-50%);
    pointer-events: none;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 6px 6px 12px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--surface);
    box-shadow: var(--shadow);
    font-size: 13px;
    pointer-events: auto;
  }
  .toast.ok {
    border-left: 4px solid var(--ok);
  }
  .toast.error {
    border-left: 4px solid var(--problem);
  }
  .text {
    flex: 1;
  }
  @media (max-width: 799px) {
    .toasts {
      bottom: 72px;
    }
  }
</style>
