<script lang="ts">
  import type { Membership } from '../contexts';
  import type { FileCounts } from '../findings';
  import { plural, shortenHome } from '../format';
  import { store } from '../store.svelte';
  import type { AgentFile } from '../types';
  import Icon from './Icon.svelte';

  interface Props {
    file: AgentFile;
    member?: Membership | null;
    counts?: FileCounts;
    dirty?: boolean;
    harnesses?: boolean;
  }
  let { file, member = null, counts, dirty = false, harnesses = true }: Props = $props();

  const home = $derived(store.data?.home ?? '');
  const labels = $derived(new Map((store.data?.harnesses ?? []).map((h) => [h.name, h.label])));

  const memberLabel = $derived.by(() => {
    if (!member) return '';
    const n = member.position;
    switch (member.role) {
      case 'entry':
        return `#${n}`;
      case 'link-target':
        return `#${n} via link`;
      case 'source': {
        const via = member.via ? store.index?.files.get(member.via) : undefined;
        return via?.access.builtFrom?.includes(file.id) ? `part of #${n}` : `origin of #${n}`;
      }
      case 'skill':
        return 'skill';
      case 'subagent':
        return 'subagent';
    }
    return '';
  });

  const memberTitle = $derived.by(() => {
    if (!member || !store.context) return '';
    const where = store.context.display;
    switch (member.role) {
      case 'entry':
        return `Loads at position ${member.position} in ${where}`;
      case 'link-target':
        return `Loads at position ${member.position} in ${where}, through a link`;
      case 'source':
        return `Edits to what loads at position ${member.position} in ${where} belong here`;
      default:
        return `Offered in ${where}`;
    }
  });
</script>

<span class="badges">
  {#if dirty}
    <span class="badge accent" title="Unsaved changes">unsaved</span>
  {/if}
  {#if member}
    <span class="badge accent" title={memberTitle}>{memberLabel}</span>
  {/if}
  {#if file.isLink}
    <span class="badge mono" title={`This path links to ${file.linkTarget ?? file.realPath}`}>
      <Icon name="link" size={11} />
      <span class="clip">{shortenHome(file.linkTarget ?? file.realPath, home)}</span>
    </span>
  {/if}
  {#if file.access.builtFrom?.length}
    <span class="badge" title={`Built from ${plural(file.access.builtFrom.length, 'part')}${file.access.builtBy ? ` by ${file.access.builtBy}` : ''}`}>
      <Icon name="built" size={11} />built
    </span>
  {/if}
  {#if !file.access.writable}
    <span class="badge" title={file.access.reason ?? 'Read-only'}><Icon name="lock" size={11} />read-only</span>
  {/if}
  {#if file.access.managed}
    <span
      class="badge"
      class:warn={file.access.managed.warn}
      title={`Managed by ${file.access.managed.tool}${file.access.managed.source ? ` from ${file.access.managed.source}` : ''}`}
    >
      <Icon name="managed" size={11} />{file.access.managed.tool}
    </span>
  {/if}
  {#if file.missing}
    <span class="badge problem">missing</span>
  {/if}
  {#if harnesses}
    {#each file.harnesses as h (h)}
      <span class="badge mono harness" title={`Loaded by ${labels.get(h) ?? h}`}>{h}</span>
    {/each}
  {/if}
  {#if counts?.problem}
    <span class="badge problem" title={plural(counts.problem, 'problem')}>
      <span aria-hidden="true">{counts.problem}</span><span class="sr-only">{plural(counts.problem, 'problem')}</span>
    </span>
  {/if}
  {#if counts?.hint}
    <span class="badge hint" title={plural(counts.hint, 'hint')}>
      <span aria-hidden="true">{counts.hint}</span><span class="sr-only">{plural(counts.hint, 'hint')}</span>
    </span>
  {/if}
</span>

<style>
  .badges {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }
  .clip {
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 22ch;
  }
  .harness {
    color: var(--text-3);
    background: transparent;
  }
</style>
