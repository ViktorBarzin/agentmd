<script lang="ts">
  import { candidateId, groupContexts, probeBacklog } from '../contexts';
  import { plural } from '../format';
  import { store } from '../store.svelte';
  import type { Candidate, RuntimeContext } from '../types';
  import Icon from './Icon.svelte';

  let open = $state(false);
  let filter = $state('');
  let active = $state(0);
  let root: HTMLDivElement | undefined = $state();
  let button: HTMLButtonElement | undefined = $state();
  let input: HTMLInputElement | undefined = $state();
  const uid = $props.id();

  interface Option {
    id: string;
    ctx: RuntimeContext | null;
    /** Set for a directory that has not been probed: choosing it probes it. */
    candidate: Candidate | null;
  }

  const groups = $derived(store.data ? groupContexts(store.data) : []);
  const backlog = $derived(store.data ? probeBacklog(store.data) : { unprobed: 0, stale: 0 });

  const visibleGroups = $derived.by(() => {
    const q = filter.trim().toLowerCase();
    const hit = (display: string, dir: string, label: string, harness: string) =>
      !q || `${display} ${dir} ${label} ${harness}`.toLowerCase().includes(q);
    return groups
      .map((g) => ({
        ...g,
        contexts: g.contexts.filter((c) => hit(c.display, c.dir, g.label, c.harness)),
        unprobed: g.unprobed.filter((c) => hit(c.display, c.dir, g.label, c.harness)),
      }))
      .filter((g) => g.contexts.length + g.unprobed.length > 0);
  });

  const options = $derived.by<Option[]>(() => {
    const out: Option[] = [];
    if (!filter.trim()) out.push({ id: '', ctx: null, candidate: null });
    for (const g of visibleGroups) {
      for (const c of g.contexts) out.push({ id: c.id, ctx: c, candidate: null });
      for (const c of g.unprobed) out.push({ id: candidateId(c), ctx: null, candidate: c });
    }
    return out;
  });

  const current = $derived(store.context);
  const currentLabel = $derived(
    current ? `${groups.find((g) => g.harness === current.harness)?.label ?? current.harness} · ${current.display}` : 'All files',
  );

  function optionId(i: number) {
    return `${uid}-opt-${i}`;
  }

  function show() {
    open = true;
    filter = '';
    const i = options.findIndex((o) => o.id === (current?.id ?? ''));
    active = Math.max(0, i);
    queueMicrotask(() => {
      input?.focus();
      document.getElementById(optionId(active))?.scrollIntoView({ block: 'nearest' });
    });
  }

  function hide(focusButton = true) {
    open = false;
    if (focusButton) button?.focus();
  }

  function choose(o: Option | undefined) {
    if (!o) return;
    hide();
    if (o.candidate) void store.probeAndPick(o.id);
    else store.setContext(o.id || undefined);
  }

  function probeAll() {
    hide();
    void store.probe();
  }

  function move(delta: number) {
    if (options.length === 0) return;
    active = (active + delta + options.length) % options.length;
    document.getElementById(optionId(active))?.scrollIntoView({ block: 'nearest' });
  }

  function onInputKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      move(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      move(-1);
    } else if (e.key === 'Home') {
      e.preventDefault();
      active = 0;
    } else if (e.key === 'End') {
      e.preventDefault();
      active = Math.max(0, options.length - 1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(options[active]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      hide();
    } else if (e.key === 'Tab') {
      hide(false);
    }
  }

  function onButtonKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      show();
    }
  }

  $effect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (root && e.target instanceof Node && !root.contains(e.target)) hide(false);
    };
    window.addEventListener('pointerdown', onDown);
    return () => window.removeEventListener('pointerdown', onDown);
  });

  $effect(() => {
    // Keep the highlighted option in range as the filter narrows the list.
    if (active >= options.length) active = Math.max(0, options.length - 1);
  });

  function indexOf(id: string): number {
    return options.findIndex((o) => o.id === id);
  }
</script>

<div class="picker" bind:this={root}>
  <button
    bind:this={button}
    type="button"
    class="trigger"
    class:picked={current !== null}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={`Context: ${currentLabel}. Choose a context`}
    title={currentLabel}
    onclick={() => (open ? hide() : show())}
    onkeydown={onButtonKey}
  >
    <span class="label">{currentLabel}</span>
    {#if current}
      <span class="tag" class:static={current.source === 'static'}>{current.source === 'probe' ? 'probed' : 'static'}</span>
    {/if}
    <Icon name="chevron-down" size={14} />
  </button>

  {#if open}
    <div class="popup" role="presentation">
      <input
        bind:this={input}
        bind:value={filter}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls={`${uid}-list`}
        aria-activedescendant={options[active] ? optionId(active) : undefined}
        aria-autocomplete="list"
        aria-label="Filter contexts"
        placeholder="Filter contexts"
        onkeydown={onInputKey}
        oninput={() => (active = 0)}
      />
      <div class="list" id={`${uid}-list`} role="listbox" aria-label="Contexts">
        {#if !filter.trim()}
          <div
            id={optionId(0)}
            role="option"
            tabindex="-1"
            aria-selected={current === null}
            class="option all"
            class:active={active === 0}
            onpointerenter={() => (active = 0)}
            onclick={() => choose(options[0])}
            onkeydown={onInputKey}
          >
            <span class="dir">All files</span>
            <span class="meta">every file, no context</span>
          </div>
        {/if}
        {#each visibleGroups as g (g.harness)}
          <div role="group" aria-labelledby={`${uid}-g-${g.harness}`}>
            <div class="group-label" id={`${uid}-g-${g.harness}`}>
              {g.label}{g.version ? ` ${g.version}` : ''}
            </div>
            {#each g.contexts as c (c.id)}
              {@const i = indexOf(c.id)}
              <div
                id={optionId(i)}
                role="option"
                tabindex="-1"
                aria-selected={current?.id === c.id}
                class="option"
                class:active={active === i}
                onpointerenter={() => (active = i)}
                onclick={() => choose(options[i])}
                onkeydown={onInputKey}
              >
                <span class="dir mono">{c.display}</span>
                <span class="tag" class:static={c.source === 'static'}>{c.source === 'probe' ? 'probed' : 'static'}</span>
                {#if c.stale}<span class="tag none" title="A file this context loads changed since the probe">stale</span>{/if}
                <span class="meta">
                  {#if c.error}<span class="error-text">error</span>{:else}{plural(c.entries.length, 'file')}{/if}
                </span>
              </div>
            {/each}
            {#each g.unprobed as c (c.dir)}
              {@const id = candidateId(c)}
              {@const i = indexOf(id)}
              <div
                id={optionId(i)}
                role="option"
                tabindex="-1"
                aria-selected="false"
                aria-label={`${c.display}, not probed. Choose to probe it`}
                class="option"
                class:active={active === i}
                onpointerenter={() => (active = i)}
                onclick={() => choose(options[i])}
                onkeydown={onInputKey}
              >
                <span class="dir mono unprobed">{c.display}</span>
                <span class="tag none">not probed</span>
                <span class="meta">{store.isProbing(id) ? 'probing' : 'probe'}</span>
              </div>
            {/each}
          </div>
        {:else}
          <div class="no-match">No context matches.</div>
        {/each}
      </div>
      {#if backlog.unprobed + backlog.stale > 0 || store.probingAll}
        <div class="foot">
          <span class="faint">
            {#if store.probingAll}
              {store.probeStatus}
            {:else}
              {[
                backlog.unprobed ? `${plural(backlog.unprobed, 'directory', 'directories')} not probed` : '',
                backlog.stale ? `${backlog.stale} stale` : '',
              ]
                .filter(Boolean)
                .join(', ')}
            {/if}
          </span>
          <button type="button" class="btn small" onclick={probeAll} disabled={store.probingAll}>
            {#if store.probingAll}<span class="spinner"></span>{/if}Probe all
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .picker {
    position: relative;
    min-width: 0;
  }
  .trigger {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    max-width: 100%;
    padding: 0 8px 0 10px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-size: 13px;
  }
  .trigger:hover {
    background: var(--surface-2);
  }
  .trigger.picked {
    border-color: var(--accent);
    background: var(--accent-bg);
  }
  .label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .tag {
    flex: none;
    padding: 0 5px;
    border-radius: 3px;
    background: var(--ok-bg);
    color: var(--ok-text);
    font-size: 10.5px;
    line-height: 17px;
  }
  .tag.static {
    background: var(--surface-3);
    color: var(--text-2);
  }
  .tag.none {
    background: var(--hint-bg);
    color: var(--hint-text);
  }
  .dir.unprobed {
    color: var(--text-2);
  }
  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 6px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
    font-size: 12px;
  }
  .popup {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 50;
    width: min(380px, calc(100vw - 24px));
    padding: 8px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--surface);
    box-shadow: var(--shadow);
  }
  .popup input {
    width: 100%;
    margin-bottom: 6px;
  }
  .list {
    max-height: min(60vh, 420px);
    overflow: auto;
  }
  .group-label {
    padding: 8px 8px 4px;
    color: var(--text-3);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .option {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border-radius: 5px;
    cursor: pointer;
  }
  .option.active {
    background: var(--surface-2);
  }
  .option[aria-selected='true'] .dir {
    color: var(--accent-text);
    font-weight: 600;
  }
  .dir {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .option .dir.mono {
    font-size: 12.5px;
  }
  .meta {
    margin-left: auto;
    color: var(--text-3);
    font-size: 12px;
    white-space: nowrap;
  }
  .no-match {
    padding: 12px 8px;
    color: var(--text-2);
  }
  @media (max-width: 799px) {
    .popup {
      position: fixed;
      left: 12px;
      right: 12px;
      top: 96px;
      width: auto;
    }
  }
</style>
