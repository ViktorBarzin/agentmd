<script lang="ts">
  import type { LineSpan } from '../cm';
  import { spanForLine } from '../findings';
  import { store } from '../store.svelte';
  import FileBadges from './FileBadges.svelte';
  import FileDetails from './FileDetails.svelte';
  import FileEditor from './FileEditor.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    fileId: string;
    line?: number;
  }
  let { fileId, line }: Props = $props();

  let tab = $state<'edit' | 'details'>('edit');
  const uid = $props.id();

  const file = $derived(store.index?.files.get(fileId) ?? null);
  const span = $derived.by<LineSpan | null>(() => {
    if (!line) return null;
    const s = spanForLine(store.data?.findings ?? [], fileId, line);
    return s ? { start: s.startLine, end: s.endLine } : { start: line, end: line };
  });

  function onTabKey(e: KeyboardEvent) {
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      e.preventDefault();
      tab = tab === 'edit' ? 'details' : 'edit';
      document.getElementById(`${uid}-tab-${tab}`)?.focus();
    }
  }
</script>

<section class="panel" aria-label="Editor">
  <header>
    <div class="title">
      {#if file}
        <span class="kind" style={`color: var(--kind-${file.kind})`}><Icon name={file.kind} label={file.kind} /></span>
      {/if}
      <h2 class="path mono" title={file?.path ?? fileId}>{file?.display ?? fileId}</h2>
      <button class="icon-btn" aria-label="Close the editor" onclick={() => store.closeEditor()}>
        <Icon name="close" />
      </button>
    </div>
    <div class="sub">
      {#if file}
        <FileBadges
          {file}
          member={store.members?.get(file.id) ?? null}
          counts={store.index?.counts.get(file.id)}
          dirty={store.dirty.has(file.id)}
        />
      {/if}
      <div class="tabs" role="tablist" aria-label="Editor views">
        <button
          id={`${uid}-tab-edit`}
          role="tab"
          aria-selected={tab === 'edit'}
          aria-controls={`${uid}-edit`}
          tabindex={tab === 'edit' ? 0 : -1}
          onclick={() => (tab = 'edit')}
          onkeydown={onTabKey}
        >
          <Icon name="edit" size={14} />Edit
        </button>
        <button
          id={`${uid}-tab-details`}
          role="tab"
          aria-selected={tab === 'details'}
          aria-controls={`${uid}-details`}
          tabindex={tab === 'details' ? 0 : -1}
          onclick={() => (tab = 'details')}
          onkeydown={onTabKey}
        >
          <Icon name="details" size={14} />Details
        </button>
      </div>
    </div>
  </header>

  <div class="tabpanel" id={`${uid}-edit`} role="tabpanel" aria-labelledby={`${uid}-tab-edit`} hidden={tab !== 'edit'}>
    <FileEditor {fileId} {span} scrollKey={`${fileId}:${line ?? ''}`} />
  </div>
  <div class="tabpanel" id={`${uid}-details`} role="tabpanel" aria-labelledby={`${uid}-tab-details`} hidden={tab !== 'details'}>
    {#if file}
      <FileDetails {file} />
    {:else}
      <p class="empty">agentmd has no record of this file. It may have moved since the last scan.</p>
    {/if}
  </div>
</section>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    background: var(--surface);
  }
  header {
    padding: 8px 8px 0 12px;
    border-bottom: 1px solid var(--border);
  }
  .title {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .kind {
    display: inline-flex;
  }
  h2.path {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: 13px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .sub {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: 6px 12px;
    padding: 4px 4px 0 0;
  }
  .tabs {
    display: flex;
    gap: 2px;
    margin-left: auto;
  }
  [role='tab'] {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 10px;
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--text-2);
    font-size: 12.5px;
    cursor: pointer;
  }
  [role='tab'][aria-selected='true'] {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }
  .tabpanel {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .tabpanel[hidden] {
    display: none;
  }
</style>
