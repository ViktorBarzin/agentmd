<script lang="ts">
  import { untrack } from 'svelte';
  import type { Conflict } from '../types';
  import Dialog from './Dialog.svelte';
  import MergeView from './MergeView.svelte';

  interface Props {
    conflict: Conflict;
    /** The editor's text when the save was refused. */
    mine: string;
    display: string;
    onoverwrite: (content: string) => void;
    ontakedisk: () => void;
    oncancel: () => void;
  }
  let { conflict, mine, display, onoverwrite, ontakedisk, oncancel }: Props = $props();

  let merged = $state(untrack(() => mine));
</script>

<Dialog title="This file changed on disk" wide onclose={oncancel}>
  <p class="lead">
    {conflict.error} Compare the version on disk with yours, then choose which to keep. You can edit your side before saving
    it, for example to bring a change over from the disk version.
  </p>
  <p class="faint mono path">{display}</p>
  <MergeView a={conflict.current} b={mine} labelA="On disk" labelB="Yours (editable)" editableB onchangeB={(d) => (merged = d)} />
  {#snippet footer()}
    <button class="btn" onclick={oncancel}>Cancel</button>
    <button class="btn" onclick={ontakedisk}>Take the disk version</button>
    <button class="btn primary" onclick={() => onoverwrite(merged)}>Overwrite with mine</button>
  {/snippet}
</Dialog>

<style>
  .lead {
    margin: 0 0 6px;
  }
  .path {
    margin: 0 0 10px;
    font-size: 12px;
    overflow-wrap: anywhere;
  }
</style>
