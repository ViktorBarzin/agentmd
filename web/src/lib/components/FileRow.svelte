<script lang="ts">
  import type { Membership } from '../contexts';
  import type { FileCounts } from '../findings';
  import { splitPath } from '../format';
  import type { AgentFile } from '../types';
  import FileBadges from './FileBadges.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    file: AgentFile;
    label: string;
    href: string;
    current?: boolean;
    member?: Membership | null;
    counts?: FileCounts;
    dirty?: boolean;
  }
  let { file, label, href, current = false, member = null, counts, dirty = false }: Props = $props();

  const named = $derived(file.kind !== 'instruction' && file.kind !== 'doc' && Boolean(file.name));
  const parts = $derived(splitPath(label));
</script>

<a class="row" class:current {href} aria-current={current ? 'page' : undefined} title={file.display}>
  <span class="kind" style={`color: var(--kind-${file.kind})`}>
    <Icon name={file.kind} size={15} label={file.kind} />
  </span>
  <span class="name">
    {#if named}
      <span class="base">{file.name}</span>
      <span class="dir sub mono">{label}</span>
    {:else}
      <span class="mono"><span class="dir">{parts.dir}</span><span class="base">{parts.name}</span></span>
    {/if}
  </span>
  <FileBadges {file} {member} {counts} {dirty} />
</a>

<style>
  .row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px 8px;
    min-height: 34px;
    padding: 5px 12px 5px 14px;
    color: var(--text);
    text-decoration: none;
    border-left: 3px solid transparent;
  }
  .row:hover {
    background: var(--surface-2);
  }
  .row.current {
    background: var(--accent-bg);
    border-left-color: var(--accent);
  }
  .row:focus-visible {
    outline-offset: -2px;
  }
  .kind {
    display: inline-flex;
    flex: none;
  }
  .name {
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex: 1 1 14rem;
    min-width: 0;
    overflow: hidden;
  }
  .name > span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .mono {
    font-size: 12.5px;
  }
  .dir {
    color: var(--text-3);
  }
  .base {
    font-weight: 550;
  }
  .sub {
    font-size: 11.5px;
    flex: 1 1 auto;
  }
  /* A skill or subagent keeps its name whole; its path gives way. */
  .name > .base:first-child {
    flex-shrink: 0;
    max-width: 100%;
  }
</style>
