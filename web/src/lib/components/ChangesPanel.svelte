<script lang="ts">
  import { api, errorMessage } from '../api';
  import { diffStats, parseDiff } from '../diff';
  import { plural, shortenHome } from '../format';
  import { store } from '../store.svelte';
  import type { CommitResponse, GitResponse, PushResponse } from '../types';
  import DiffView from './DiffView.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    fileId: string;
    git: GitResponse | null;
    /** True right after a save from this editor. */
    justSaved: boolean;
    /** Called after a commit, so the editor reads the git status again. */
    oncommitted: () => void;
    /** Called after a push, for the same reason. */
    onpushed: () => void;
  }
  let { fileId, git, justSaved, oncommitted, onpushed }: Props = $props();

  let message = $state('');
  let committing = $state(false);
  let result = $state.raw<CommitResponse | null>(null);
  let error = $state<string | null>(null);
  let pushing = $state(false);
  let pushed = $state.raw<PushResponse | null>(null);
  let pushError = $state<string | null>(null);
  // Changes found when the file opens start folded, so the editor keeps its
  // room; a save from this editor unfolds them.
  let open = $state(false);
  const uid = $props.id();

  $effect(() => {
    if (justSaved) open = true;
  });

  const repo = $derived(git?.repo ?? '');
  const diff = $derived(git?.diff ?? '');
  const stats = $derived(diffStats(parseDiff(diff)));
  const home = $derived(store.data?.home ?? '');
  const where = $derived(shortenHome(repo, home));
  /** A commit from this panel unlocks the push step. */
  const canPush = $derived(Boolean(result && git?.upstream && git.ahead > 0));

  async function commit(e: SubmitEvent) {
    e.preventDefault();
    if (!message.trim() || committing) return;
    committing = true;
    error = null;
    result = null;
    pushed = null;
    pushError = null;
    try {
      result = await api.commit({ ids: [fileId], message: message.trim() });
      message = '';
      oncommitted();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      committing = false;
    }
  }

  async function push() {
    if (pushing) return;
    pushing = true;
    pushError = null;
    pushed = null;
    try {
      pushed = await api.push({ id: fileId });
      onpushed();
    } catch (err) {
      pushError = errorMessage(err);
    } finally {
      pushing = false;
    }
  }
</script>

<section class="changes" aria-label="Changes">
  {#if !repo}
    {#if justSaved}
      <div class="banner ok"><Icon name="check" />Saved. This file is not in a git repository, so there is nothing to commit.</div>
    {/if}
  {:else}
    {#if diff}
      <div class="head">
        <button type="button" class="toggle" aria-expanded={open} onclick={() => (open = !open)}>
          <Icon name={open ? 'chevron-down' : 'chevron-right'} size={14} />
          {justSaved ? 'Saved. ' : ''}Uncommitted changes in <span class="mono">{where}</span>
        </button>
        <span class="stats"><span class="add">+{stats.added}</span> <span class="del">-{stats.removed}</span></span>
      </div>
      {#if open}
        <DiffView {diff} label="Uncommitted changes to this file" />
        <form class="commit" onsubmit={commit}>
          <label for={`${uid}-msg`}>Commit message</label>
          <textarea
            id={`${uid}-msg`}
            rows="3"
            bind:value={message}
            placeholder="What changed, then a blank line and why"
          ></textarea>
          <div class="row">
            <span class="faint">Commits only this file, with your git identity and hooks.</span>
            <button class="btn primary" type="submit" disabled={!message.trim() || committing}>
              {#if committing}<span class="spinner"></span>{:else}<Icon name="git" size={14} />{/if}
              Commit
            </button>
          </div>
        </form>
      {/if}
    {:else if justSaved && !result}
      <div class="banner ok"><Icon name="check" />Saved. The file matches the last commit.</div>
    {/if}

    {#if result}
      <div class="banner ok result">
        <Icon name="check" />
        <div class="banner-body">
          Committed <code>{result.commit.slice(0, 7)}</code> in <span class="mono">{shortenHome(result.repo, home)}</span>.
          {#if result.output}<pre>{result.output}</pre>{/if}
        </div>
      </div>
      {#if git && !pushed}
        {#if canPush}
          <div class="push">
            <button class="btn" onclick={push} disabled={pushing}>
              {#if pushing}<span class="spinner"></span>{:else}<Icon name="open" size={14} />{/if}
              Push to {git.upstream} ({plural(git.ahead, 'commit')})
            </button>
            <span class="faint">
              {#if git.behind > 0}
                {git.upstream} has {plural(git.behind, 'commit')} that {git.branch} lacks, so git will refuse a fast-forward.
              {:else}
                Fast-forward only. agentmd never forces or rebases.
              {/if}
            </span>
          </div>
        {:else if git.branch && !git.upstream}
          <p class="faint note">Branch {git.branch} has no upstream, so there is nowhere to push it from here.</p>
        {/if}
      {/if}
    {/if}

    {#if pushed}
      <div class="banner ok result">
        <Icon name="check" />
        <div class="banner-body">
          Pushed {pushed.branch} to {pushed.upstream}.
          {#if pushed.output}<pre>{pushed.output}</pre>{/if}
        </div>
      </div>
    {/if}
    {#if pushError}
      <div class="banner error result" role="alert">
        <Icon name="info" />
        <div class="banner-body">
          Git refused the push. Nothing changed on the remote.
          <pre>{pushError}</pre>
        </div>
      </div>
    {/if}
    {#if error}
      <div class="banner error result" role="alert">
        <Icon name="info" />
        <div class="banner-body">
          The commit did not go through.
          <pre>{error}</pre>
        </div>
      </div>
    {/if}
  {/if}
</section>

<style>
  .changes {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 8px 12px;
    border-top: 1px solid var(--border);
    background: var(--surface);
    max-height: 50%;
    overflow: auto;
    flex: none;
  }
  .changes > :global(*) {
    flex-shrink: 0;
  }
  .changes :global(.diff) {
    max-height: 200px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 0;
    border: 0;
    background: none;
    color: var(--text);
    font-weight: 600;
    font-size: 13px;
    cursor: pointer;
    text-align: left;
  }
  .toggle .mono {
    font-weight: 500;
    font-size: 12.5px;
  }
  .stats {
    margin-left: auto;
    font-family: var(--font-mono);
    font-size: 12px;
  }
  .add {
    color: var(--diff-add-text);
  }
  .del {
    color: var(--diff-del-text);
  }
  .commit {
    display: grid;
    gap: 6px;
  }
  label {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-2);
  }
  textarea {
    width: 100%;
    resize: vertical;
    font-family: var(--font-mono);
    font-size: 12.5px;
  }
  .row,
  .push {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px 10px;
    font-size: 12px;
  }
  .row {
    justify-content: space-between;
  }
  .note {
    margin: 0;
    font-size: 12px;
  }
  .result pre {
    margin: 6px 0 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 12px;
  }
  .banner :global(svg) {
    margin-top: 1px;
  }
</style>
