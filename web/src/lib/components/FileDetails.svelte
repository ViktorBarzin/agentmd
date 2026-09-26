<script lang="ts">
  import { contextsSkipping } from '../contexts';
  import { KIND_LABELS, findingsForFile, sortFindings } from '../findings';
  import { formatBytes, formatCount, lineRange, shortenHome } from '../format';
  import { REF_KIND_LABELS } from '../graph';
  import { store } from '../store.svelte';
  import type { AgentFile, Ref } from '../types';
  import InlineText from './InlineText.svelte';

  interface Props {
    file: AgentFile;
  }
  let { file }: Props = $props();

  const ix = $derived(store.index);
  const home = $derived(store.data?.home ?? '');
  const links = $derived(ix?.linksTo.get(file.id) ?? []);
  const linkTo = $derived((ix?.refsFrom.get(file.id) ?? []).find((r) => r.kind === 'symlink' && !r.dangling)?.to ?? null);
  const out = $derived((ix?.refsFrom.get(file.id) ?? []).filter((r) => r.kind !== 'symlink' || !file.isLink));
  const incoming = $derived((ix?.refsTo.get(file.id) ?? []).filter((r) => r.kind !== 'symlink'));
  /** Contexts that list this file, or a link to it, as an entry. */
  const loaded = $derived([
    ...(ix?.loadedIn.get(file.id) ?? []).map((l) => ({ ...l, via: '' })),
    ...links.flatMap((link) => (ix?.loadedIn.get(link) ?? []).map((l) => ({ ...l, via: link }))),
  ]);
  const findings = $derived(sortFindings(findingsForFile(store.data?.findings ?? [], file.id), ix?.files ?? new Map()));
  const skipped = $derived(contextsSkipping(file.id, store.data?.contexts ?? []));
  const harnessLabel = (name: string) => store.data?.harnesses.find((h) => h.name === name)?.label ?? name;

  function refTarget(r: Ref, side: 'from' | 'to'): string {
    const id = side === 'to' ? r.to : r.from;
    return ix?.files.get(id)?.display ?? shortenHome(id, home);
  }

  function refKind(r: Ref): string {
    return `${REF_KIND_LABELS[r.kind]}${r.sub ? ` (${r.sub})` : ''}`;
  }
</script>

<div class="details">
  <section>
    <h3>Paths</h3>
    <dl>
      <dt>Path</dt>
      <dd class="mono">{file.path}{file.field ? `#${file.field}` : ''}</dd>
      {#if file.realPath !== file.path}
        <dt>Real path</dt>
        <dd class="mono">{file.realPath}</dd>
      {/if}
      {#if file.field}
        <dt>Field</dt>
        <dd class="mono">{file.field}</dd>
      {/if}
      {#if file.repo}
        <dt>Repository</dt>
        <dd class="mono">{shortenHome(file.repo, home)}</dd>
      {/if}
      <dt>Kind</dt>
      <dd>{file.kind}, {file.scope} scope</dd>
      {#if file.name}
        <dt>Name</dt>
        <dd>{file.name}</dd>
      {/if}
      {#if file.description}
        <dt>Description</dt>
        <dd>{file.description}</dd>
      {/if}
      <dt>Size</dt>
      <dd>{formatBytes(file.size)} ({formatCount(file.size)} bytes), {formatCount(file.lines)} lines</dd>
      <dt>Hash</dt>
      <dd class="mono">{file.hash}</dd>
      <dt>Access</dt>
      <dd>
        {file.access.writable ? 'Writable' : `Read-only${file.access.reason ? `: ${file.access.reason}` : ''}`}
        {#if file.access.owner}<span class="faint">, owner {file.access.owner}</span>{/if}
      </dd>
    </dl>
  </section>

  {#if links.length || file.isLink}
    <section>
      <h3>Links</h3>
      <ul>
        {#if file.isLink}
          <li>
            This path links to
            {#if linkTo}
              <a href={store.hrefFor(store.fileRoute(linkTo))} class="mono">{ix?.files.get(linkTo)?.display ?? linkTo}</a>
            {:else}
              <span class="mono">{shortenHome(file.linkTarget ?? file.realPath, home)}</span>
            {/if}
          </li>
        {/if}
        {#each links as l (l)}
          <li><a class="mono" href={store.hrefFor(store.fileRoute(l))}>{ix?.files.get(l)?.display ?? l}</a> links here</li>
        {/each}
      </ul>
    </section>
  {/if}

  <section>
    <h3>References out</h3>
    {#if out.length === 0}
      <p class="faint">This file does not point at other agent files.</p>
    {:else}
      <ul>
        {#each out as r, i (i)}
          <li>
            <span class="kind">{refKind(r)}</span>
            {#if r.dangling}
              <a class="mono error-text" href={store.hrefFor(store.fileRoute(file.id, r.line))}>{shortenHome(r.to, home)}</a>
              <span class="badge problem">missing</span>
            {:else}
              <a class="mono" href={store.hrefFor(store.fileRoute(r.to))}>{refTarget(r, 'to')}</a>
            {/if}
            {#if r.line}<a class="faint" href={store.hrefFor(store.fileRoute(file.id, r.line))}>line {r.line}</a>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section>
    <h3>References in</h3>
    {#if incoming.length === 0}
      <p class="faint">No agent file points at this one.</p>
    {:else}
      <ul>
        {#each incoming as r, i (i)}
          <li>
            <span class="kind">{refKind(r)}</span>
            <a class="mono" href={store.hrefFor(store.fileRoute(r.from, r.line))}>{refTarget(r, 'from')}</a>
            {#if r.line}<span class="faint">line {r.line}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section>
    <h3>Loaded in</h3>
    {#if loaded.length === 0}
      <p class="faint">No context loads this file at session start.</p>
    {:else}
      <ul>
        {#each loaded as l (l.context.id + l.via)}
          <li>
            <a href={store.hrefFor({ ...store.fileRoute(file.id), ctx: l.context.id })}>
              {harnessLabel(l.context.harness)} <span class="mono">{l.context.display}</span>
            </a>
            <span class="faint">
              position {l.position} of {l.context.entries.length}{l.context.source === 'static' ? ', static' : ''}{l.via
                ? `, through ${ix?.files.get(l.via)?.display ?? l.via}`
                : ''}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if skipped.length}
    <section>
      <h3>Skipped in</h3>
      <ul>
        {#each skipped as s (s.context.id)}
          <li class="skip">
            <a href={store.hrefFor({ ...store.fileRoute(file.id), ctx: s.context.id })}>
              {harnessLabel(s.context.harness)} <span class="mono">{s.context.display}</span>
            </a>
            {#if s.entry.reason}<span class="faint"><InlineText text={s.entry.reason} /></span>{/if}
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <section>
    <h3>Findings</h3>
    {#if findings.length === 0}
      <p class="faint">No findings in this file.</p>
    {:else}
      <ul>
        {#each findings as f (f.id)}
          {@const s = f.spans.find((x) => x.fileId === file.id)}
          <li class="finding">
            <span class={`dot ${f.severity}`} aria-hidden="true"></span>
            <span>
              <a href={store.hrefFor(store.findingRoute(f))}>{KIND_LABELS[f.kind]}: {f.summary}</a>
              {#if s}<span class="faint">lines {lineRange(s.startLine, s.endLine)}</span>{/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<style>
  .details {
    height: 100%;
    overflow: auto;
    padding: 4px 14px 24px;
    font-size: 13px;
  }
  section {
    padding: 10px 0;
    border-bottom: 1px solid var(--border);
  }
  section:last-child {
    border-bottom: 0;
  }
  h3 {
    margin: 0 0 6px;
    font-size: 11.5px;
    font-weight: 650;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  dl {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 3px 12px;
    margin: 0;
  }
  dt {
    color: var(--text-2);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  dd.mono {
    font-size: 12px;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 4px;
  }
  li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 2px 8px;
    min-width: 0;
  }
  li a.mono {
    font-size: 12px;
    overflow-wrap: anywhere;
  }
  .kind {
    color: var(--text-2);
    font-size: 12px;
  }
  li.finding {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: baseline;
    gap: 8px;
  }
  li.finding .faint {
    margin-left: 6px;
    white-space: nowrap;
  }
  p {
    margin: 0;
  }
</style>
