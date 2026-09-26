<script lang="ts">
  import { inlineParts } from '../inline';

  interface Props {
    text: string;
  }
  let { text }: Props = $props();

  const parts = $derived(inlineParts(text));
</script>

{#each parts as p, i (i)}{#if p.code}<code>{p.text}</code>{:else}{p.text}{/if}{/each}

<style>
  code {
    padding: 0 3px;
    border-radius: 3px;
    background: color-mix(in srgb, currentColor 10%, transparent);
    font-size: 0.92em;
  }
</style>
