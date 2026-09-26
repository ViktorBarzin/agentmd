<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { api, ApiError, errorMessage } from '../api';
  import type { LineSpan } from '../cm';
  import { findingsForFile } from '../findings';
  import { sentence, shortenHome } from '../format';
  import { store } from '../store.svelte';
  import type { AgentFile, Conflict, GitResponse } from '../types';
  import ChangesPanel from './ChangesPanel.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import ConflictDialog from './ConflictDialog.svelte';
  import Icon from './Icon.svelte';
  import InlineText from './InlineText.svelte';

  interface Props {
    fileId: string;
    span: LineSpan | null;
    scrollKey: string;
    /** Compare mode: fewer banners, no details. */
    compact?: boolean;
  }
  let { fileId, span, scrollKey, compact = false }: Props = $props();

  let file = $state.raw<AgentFile | null>(null);
  let saved = $state('');
  let baseHash = $state('');
  let doc = $state('');
  let resetKey = $state(0);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let conflict = $state.raw<Conflict | null>(null);
  let restored = $state(false);
  let diskChanged = $state(false);
  /** The file's repository status: diff, branch, upstream and distance. */
  let git = $state.raw<GitResponse | null>(null);
  let justSaved = $state(false);
  let showChanges = $state(false);
  let loadedId = '';
  let seenReload = 0;
  let lastDiskHash: string | undefined;

  const dirty = $derived(!loading && doc !== saved);
  const writable = $derived(file?.access.writable ?? false);
  const findings = $derived(findingsForFile(store.data?.findings ?? [], fileId));
  const home = $derived(store.data?.home ?? '');
  const diskHash = $derived(store.index?.files.get(fileId)?.hash);

  async function loadGit(id: string) {
    try {
      const g = await api.git(id);
      if (id !== fileId) return;
      git = g;
      if (g.dirty) showChanges = true;
    } catch {
      if (id === fileId) git = null;
    }
  }

  async function load(id: string, quiet = false) {
    if (!quiet) loading = true;
    loadError = null;
    try {
      const res = await api.file(id);
      if (id !== fileId) return;
      const switched = loadedId !== id;
      loadedId = id;
      file = res.file;
      saved = res.content;
      baseHash = res.file.hash;
      lastDiskHash = res.file.hash;
      diskChanged = false;
      conflict = null;
      saveError = null;
      const draft = store.getDraft(id);
      if (draft && draft.content !== res.content) {
        doc = draft.content;
        baseHash = draft.baseHash;
        restored = true;
      } else {
        doc = res.content;
        restored = false;
        store.setDraft(id, null);
      }
      if (switched) {
        justSaved = false;
        showChanges = false;
        git = null;
      }
      resetKey++;
      void loadGit(id);
    } catch (e) {
      if (id === fileId) loadError = errorMessage(e);
    } finally {
      if (id === fileId) loading = false;
    }
  }

  $effect(() => {
    const id = fileId;
    untrack(() => {
      seenReload = store.reloads[id] ?? 0;
      void load(id);
    });
  });

  // Another part of the UI (a fix proposal) changed this file on disk.
  $effect(() => {
    const n = store.reloads[fileId] ?? 0;
    untrack(() => {
      if (n > seenReload) {
        seenReload = n;
        if (dirty) diskChanged = true;
        else void load(fileId, true);
      }
    });
  });

  // A rescan found a new version on disk.
  $effect(() => {
    const h = diskHash;
    untrack(() => {
      if (loading || saving || h === undefined) return;
      if (lastDiskHash !== undefined && h !== lastDiskHash && h !== baseHash) {
        if (dirty) diskChanged = true;
        else void load(fileId, true);
      }
      lastDiskHash = h;
    });
  });

  function onchange(next: string) {
    doc = next;
    store.setDraft(fileId, next !== saved ? { content: next, baseHash } : null);
  }

  async function save(override?: { content: string; baseHash: string }) {
    if (!file || !writable || saving) return;
    if (!override && !dirty) return;
    saving = true;
    saveError = null;
    const content = override?.content ?? doc;
    try {
      const res = await api.save({ id: fileId, content, baseHash: override?.baseHash ?? baseHash });
      file = res.file;
      saved = res.content;
      baseHash = res.file.hash;
      lastDiskHash = res.file.hash;
      if (override || doc === content) {
        doc = res.content;
        if (override) resetKey++;
      }
      store.setDraft(fileId, doc !== saved ? { content: doc, baseHash } : null);
      git = {
        branch: '',
        upstream: '',
        ahead: 0,
        behind: 0,
        ...git,
        repo: res.repo,
        diff: res.diff,
        dirty: res.diff !== '',
      };
      justSaved = true;
      showChanges = true;
      conflict = null;
      diskChanged = false;
      restored = false;
      void store.refresh();
    } catch (e) {
      if (e instanceof ApiError && e.conflict) conflict = e.conflict;
      else saveError = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  function revert() {
    store.setDraft(fileId, null);
    restored = false;
    void load(fileId, true);
  }

  function takeDisk(c: Conflict) {
    saved = c.current;
    baseHash = c.hash;
    lastDiskHash = c.hash;
    doc = c.current;
    resetKey++;
    store.setDraft(fileId, null);
    conflict = null;
    diskChanged = false;
  }

  function oncommitted() {
    justSaved = false;
    void loadGit(fileId);
    void store.refresh();
  }

  // Ctrl/Cmd+S outside CodeMirror saves the editor that last had focus.
  const saveFromShortcut = () => void save();
  function onfocus() {
    store.activeSave = saveFromShortcut;
  }
  onDestroy(() => {
    if (store.activeSave === saveFromShortcut) store.activeSave = null;
  });

  const access = $derived(file?.access);
  const origin = $derived(access?.origin);
  const originFileId = $derived(origin?.fileId && store.index?.files.has(origin.fileId) ? origin.fileId : null);
  const parts = $derived(access?.builtFrom ?? []);
  const label = $derived(`${file?.display ?? fileId}${writable ? '' : ' (read-only)'}`);
</script>

<div class="file-editor" class:compact>
  {#if loading && !file}
    <div class="empty"><span class="spinner"></span> Loading the file</div>
  {:else if loadError && !file}
    <div class="pad">
      <div class="banner error" role="alert">
        <div class="banner-body">
          Could not open {fileId}: {loadError}
          <div class="banner-actions"><button class="btn small" onclick={() => load(fileId)}>Try again</button></div>
        </div>
      </div>
    </div>
  {:else if file}
    <div class="banners">
      {#if file.missing}
        <div class="banner error">This file no longer exists on disk.</div>
      {/if}
      {#if parts.length}
        <div class="banner">
          <Icon name="built" />
          <div class="banner-body">
            Read-only: built from {parts.length === 1 ? 'one part' : `${parts.length} parts`}{access?.builtBy ? ` by ${access.builtBy}` : ''}. Edit the parts instead.
            <div class="banner-actions">
              {#each parts as p (p)}
                <button class="btn small" onclick={() => store.openFile(p)}>
                  <Icon name="edit" size={13} /><span class="mono">{store.index?.files.get(p)?.display ?? shortenHome(p, home)}</span>
                </button>
              {/each}
            </div>
          </div>
        </div>
      {:else if !writable}
        <div class="banner">
          <Icon name="lock" />
          <div class="banner-body">
            Read-only: <InlineText text={sentence(access?.reason ?? 'you cannot write to this file')} />
            {#if access?.owner}<span class="faint">Owner: {access.owner}.</span>{/if}
          </div>
        </div>
      {/if}
      {#if origin}
        <div class="banner">
          <Icon name="open" />
          <div class="banner-body">
            Installed from <span class="mono">{shortenHome(origin.path, home)}</span>{origin.field ? `, field ${origin.field}` : ''}.
            {#if origin.note}<InlineText text={sentence(origin.note)} />{/if}
            <div class="banner-actions">
              <button class="btn small" disabled={!originFileId} onclick={() => originFileId && store.openFile(originFileId)}>
                <Icon name="edit" size={13} />Open the origin
              </button>
              {#if !originFileId}<span class="faint">agentmd did not find the origin among the scanned files.</span>{/if}
            </div>
          </div>
        </div>
      {/if}
      {#if access?.managed && !compact}
        <div class="banner" class:warn={access.managed.warn}>
          <Icon name="managed" />
          <div class="banner-body">
            Managed by {access.managed.tool}{access.managed.source ? ` from ${access.managed.source}` : ''}.
            <InlineText
              text={sentence(access.managed.note ?? (access.managed.warn ? 'Edits here are replaced on its next run' : ''))}
            />
          </div>
        </div>
      {:else if access?.managed?.warn}
        <div class="banner warn">
          <Icon name="managed" />
          <div class="banner-body">
            Managed by {access.managed.tool}:
            <InlineText text={sentence(access.managed.note ?? 'edits here are replaced on its next run')} />
          </div>
        </div>
      {/if}
      {#if file.isLink && writable && !compact}
        <div class="banner">
          <Icon name="link" />
          <div class="banner-body">This path links to <span class="mono">{shortenHome(file.realPath, home)}</span>. Saving writes there.</div>
        </div>
      {/if}
      {#if restored}
        <div class="banner">
          <Icon name="info" />
          <div class="banner-body">Your unsaved changes from earlier in this session are back.</div>
        </div>
      {/if}
      {#if diskChanged}
        <div class="banner warn" role="status">
          <Icon name="info" />
          <div class="banner-body">
            This file changed on disk while you were editing it. Saving will ask you which version to keep.
            <div class="banner-actions">
              <button class="btn small" onclick={revert}>Discard mine and reload</button>
            </div>
          </div>
        </div>
      {/if}
    </div>

    <div class="toolbar">
      <span class="state" aria-live="polite">
        {#if saving}
          <span class="spinner"></span> Saving
        {:else if dirty}
          <span class="dot unsaved"></span> Unsaved changes
        {:else if !writable}
          <Icon name="lock" size={14} /> Read-only
        {:else}
          <span class="faint">No changes</span>
        {/if}
      </span>
      {#if findings.length && !compact}
        <span class="faint nfind">{findings.length === 1 ? '1 finding' : `${findings.length} findings`} marked in the gutter</span>
      {/if}
      {#if writable}
        <button class="btn small" onclick={revert} disabled={!dirty || saving}><Icon name="undo" size={14} />Revert</button>
        <button class="btn small primary" onclick={() => save()} disabled={!dirty || saving} title="Save (Ctrl+S)">
          <Icon name="save" size={14} />Save
        </button>
      {/if}
    </div>
    {#if saveError}
      <div class="pad"><div class="banner error" role="alert">The save failed: {saveError}</div></div>
    {/if}

    <CodeEditor
      {doc}
      {resetKey}
      readOnly={!writable}
      {fileId}
      {findings}
      {span}
      {scrollKey}
      {label}
      {onchange}
      onsave={() => save()}
      {onfocus}
    />

    {#if showChanges}
      {#key fileId}
        <ChangesPanel {fileId} {git} {justSaved} {oncommitted} onpushed={() => loadGit(fileId)} />
      {/key}
    {/if}

    {#if conflict}
      <ConflictDialog
        {conflict}
        mine={doc}
        display={file.display}
        onoverwrite={(content) => conflict && save({ content, baseHash: conflict.hash })}
        ontakedisk={() => conflict && takeDisk(conflict)}
        oncancel={() => (conflict = null)}
      />
    {/if}
  {/if}
</div>

<style>
  .file-editor {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    background: var(--editor-bg);
  }
  .banners {
    display: grid;
    gap: 6px;
    padding: 8px 12px 0;
    background: var(--surface);
  }
  .banners:empty {
    display: none;
  }
  .banners :global(.banner > svg) {
    margin-top: 2px;
  }
  .pad {
    padding: 8px 12px;
  }
  .toolbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    color: var(--text-2);
    margin-right: auto;
  }
  .nfind {
    font-size: 12px;
  }
  @media (max-width: 599px) {
    .nfind {
      display: none;
    }
  }
  .dot.unsaved {
    background: var(--accent);
  }
  .empty {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }
</style>
