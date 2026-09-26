// App state shared by every view: the scanned State, the route, the picked
// context, running jobs and unsaved drafts.
import { api, errorMessage } from './api';
import { contextMembers, type Membership } from './contexts';
import { EMPTY_FILTERS, filterFindings, isCompare } from './findings';
import { indexState, type StateIndex } from './model';
import { backgroundView, closeEditor, formatRoute, parseRoute, withContext, type FileRoute, type Route } from './route';
import { createThrottle } from './throttle';
import { matchesQuery } from './tree';
import type { AgentFile, Finding, Job, RuntimeContext, State } from './types';

export interface Draft {
  content: string;
  baseHash: string;
}

export interface Toast {
  id: number;
  text: string;
  tone: 'info' | 'ok' | 'error';
}

export const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

/** Polls a job until it finishes. `cancelled` stops the loop early. */
export async function waitForJob(job: Job, onUpdate: (j: Job) => void, cancelled: () => boolean = () => false): Promise<Job> {
  let current = job;
  while (current.status === 'running') {
    await sleep(700);
    if (cancelled()) return current;
    current = await api.job(current.id);
    onUpdate(current);
  }
  return current;
}

class AppStore {
  data = $state.raw<State | null>(null);
  index = $derived<StateIndex | null>(this.data ? indexState(this.data) : null);
  loading = $state(true);
  loadError = $state<string | null>(null);
  scanning = $state(false);
  route = $state.raw<Route>({ name: 'files' });
  query = $state('');
  now = $state(Date.now());
  /** The last analysis job started for each context. */
  analyseJobs = $state.raw<Record<string, Job>>({});
  /** Files with unsaved edits. */
  dirty = $state.raw<ReadonlySet<string>>(new Set());
  /** Bumped per file to ask open editors to reload it from disk. */
  reloads = $state.raw<Record<string, number>>({});
  toasts = $state.raw<Toast[]>([]);
  /** The finding whose fix proposal dialog is open. */
  proposalFor = $state<string | null>(null);
  /** Context ids being probed, or "*" while every context is. */
  probing = $state.raw<ReadonlySet<string>>(new Set());
  /** Saves the editor that last had focus, for Ctrl/Cmd+S outside CodeMirror. */
  activeSave: (() => void) | null = null;

  context = $derived<RuntimeContext | null>(
    this.route.ctx && this.index ? (this.index.contexts.get(this.route.ctx) ?? null) : null,
  );
  members = $derived<Map<string, Membership> | null>(
    this.context && this.data ? contextMembers(this.context, this.data.refs) : null,
  );
  /** Files that belong to the picked context, or every file. */
  visibleFiles = $derived.by<AgentFile[]>(() => {
    const d = this.data;
    if (!d) return [];
    const m = this.members;
    return m ? d.files.filter((f) => m.has(f.id)) : d.files;
  });
  /** Findings that concern the picked context, or every finding. */
  contextFindings = $derived.by<Finding[]>(() => {
    const d = this.data;
    const ix = this.index;
    if (!d || !ix) return [];
    if (!this.context || !this.members) return d.findings;
    return filterFindings(
      d.findings,
      EMPTY_FILTERS,
      { contexts: ix.contexts, files: ix.files },
      { contextId: this.context.id, members: new Set(this.members.keys()) },
    );
  });
  /** Whether a file matches the header search. */
  queryMatch = $derived.by<(id: string) => boolean>(() => {
    const q = this.query.trim();
    const files = this.index?.files;
    return (id: string) => {
      if (!q) return true;
      const f = files?.get(id);
      return f ? matchesQuery(f, q) : false;
    };
  });

  private drafts = new Map<string, Draft>();
  private toastSeq = 0;
  private focusThrottle = createThrottle(10_000);

  init(): () => void {
    this.route = parseRoute(location.hash);
    const onHash = () => {
      this.route = parseRoute(location.hash);
    };
    const onFocus = () => this.onFocus();
    const onVisible = () => {
      if (document.visibilityState === 'visible') this.onFocus();
    };
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (this.dirty.size > 0) e.preventDefault();
    };
    window.addEventListener('hashchange', onHash);
    window.addEventListener('focus', onFocus);
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('beforeunload', onBeforeUnload);
    const tick = setInterval(() => {
      this.now = Date.now();
    }, 30_000);
    void this.load();
    return () => {
      window.removeEventListener('hashchange', onHash);
      window.removeEventListener('focus', onFocus);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('beforeunload', onBeforeUnload);
      clearInterval(tick);
    };
  }

  async load() {
    this.loading = true;
    this.loadError = null;
    try {
      this.data = await api.state();
      this.focusThrottle.mark();
    } catch (e) {
      this.loadError = errorMessage(e);
    } finally {
      this.loading = false;
      this.now = Date.now();
    }
  }

  async rescan() {
    if (this.scanning) return;
    this.scanning = true;
    this.focusThrottle.mark();
    try {
      this.data = await api.scan();
      this.loadError = null;
    } catch (e) {
      this.toast(`The rescan failed: ${errorMessage(e)}`, 'error');
    } finally {
      this.scanning = false;
      this.now = Date.now();
    }
  }

  /** Fetches the state again without a scan, after saves and jobs. */
  async refresh() {
    try {
      this.data = await api.state();
    } catch (e) {
      this.toast(`Could not refresh: ${errorMessage(e)}`, 'error');
    }
  }

  /** "Claude Code in ~/code/app" for a context or an unprobed candidate. */
  contextLabel(id: string): string {
    const colon = id.indexOf(':');
    const harness = id.slice(0, colon);
    const dir = id.slice(colon + 1);
    const label = this.data?.harnesses.find((h) => h.name === harness)?.label ?? harness;
    const display =
      this.index?.contexts.get(id)?.display ?? this.data?.unprobed?.find((c) => c.harness === harness && c.dir === dir)?.display ?? dir;
    return `${label} in ${display}`;
  }

  /**
   * Probes the given contexts, or every Claude Code and Codex context when the
   * list is empty. Returns whether the probe worked.
   */
  async probe(ids: string[] = []): Promise<boolean> {
    const keys = ids.length ? ids : ['*'];
    if (this.probing.has('*') || keys.some((k) => this.probing.has(k))) return false;
    this.probing = new Set([...this.probing, ...keys]);
    try {
      this.data = await api.probe(ids);
      const what = ids.length === 0 ? 'every context' : ids.length === 1 ? this.contextLabel(ids[0]) : `${ids.length} contexts`;
      this.toast(`Probed ${what}.`, 'ok');
      return true;
    } catch (e) {
      this.toast(`The probe failed: ${errorMessage(e)}`, 'error');
      return false;
    } finally {
      const next = new Set(this.probing);
      for (const k of keys) next.delete(k);
      this.probing = next;
    }
  }

  /** Probes a directory that has not been probed yet, then picks its context. */
  async probeAndPick(id: string) {
    const ok = await this.probe([id]);
    if (ok && this.index?.contexts.has(id)) this.setContext(id);
  }

  isProbing(id: string): boolean {
    return this.probing.has('*') || this.probing.has(id);
  }

  /** Rescans when the tab regains focus, at most once every 10 seconds. */
  onFocus() {
    if (!this.data || this.scanning) return;
    if (this.focusThrottle.tryRun()) void this.rescan();
  }

  // ---- navigation ------------------------------------------------------

  navigate(route: Route) {
    const hash = formatRoute(route);
    if (location.hash !== hash) location.hash = hash;
    else this.route = route;
  }

  hrefFor(route: Route): string {
    return formatRoute(route);
  }

  fileRoute(id: string, line?: number): FileRoute {
    const r: FileRoute = { name: 'file', id };
    if (line) r.line = line;
    const view = backgroundView(this.route);
    if (view !== 'files') r.view = view;
    if (this.route.ctx) r.ctx = this.route.ctx;
    return r;
  }

  openFile(id: string, line?: number) {
    this.navigate(this.fileRoute(id, line));
  }

  findingRoute(f: Finding): Route {
    if (!isCompare(f)) {
      const s = f.spans[0];
      return s ? this.fileRoute(s.fileId, s.startLine) : this.route;
    }
    const r: Route = { name: 'compare', finding: f.id };
    const view = backgroundView(this.route);
    if (view !== 'findings') r.view = view;
    if (this.route.ctx) r.ctx = this.route.ctx;
    return r;
  }

  openFinding(f: Finding) {
    this.navigate(this.findingRoute(f));
  }

  closeEditor() {
    this.navigate(closeEditor(this.route));
  }

  setContext(id: string | undefined) {
    this.navigate(withContext(this.route, id));
  }

  // ---- drafts and reloads ----------------------------------------------

  getDraft(id: string): Draft | undefined {
    return this.drafts.get(id);
  }

  setDraft(id: string, draft: Draft | null) {
    if (draft) this.drafts.set(id, draft);
    else this.drafts.delete(id);
    if (Boolean(draft) !== this.dirty.has(id)) {
      const next = new Set(this.dirty);
      if (draft) next.add(id);
      else next.delete(id);
      this.dirty = next;
    }
  }

  requestReload(ids: string[]) {
    const next = { ...this.reloads };
    for (const id of ids) next[id] = (next[id] ?? 0) + 1;
    this.reloads = next;
  }

  // ---- analysis ----------------------------------------------------------

  private setJob(ctxId: string, job: Job) {
    this.analyseJobs = { ...this.analyseJobs, [ctxId]: job };
  }

  async analyse(ctxId: string) {
    const ctx = this.index?.contexts.get(ctxId);
    const harness = this.data?.harnesses.find((h) => h.name === ctx?.harness)?.label ?? ctx?.harness ?? '';
    const display = ctx ? `${harness} in ${ctx.display}` : ctxId;
    try {
      const job = await api.analyse(ctxId);
      this.setJob(ctxId, job);
      const done = await waitForJob(job, (j) => this.setJob(ctxId, j));
      await this.refresh();
      if (done.status === 'done') {
        const n = done.findings?.length ?? 0;
        this.toast(`Analysed ${display}: ${n === 1 ? '1 finding' : `${n} findings`}.`, 'ok');
      }
    } catch (e) {
      this.setJob(ctxId, {
        id: '',
        kind: 'analyse',
        status: 'error',
        context: ctxId,
        startedAt: new Date().toISOString(),
        error: errorMessage(e),
      });
    }
  }

  // ---- toasts ------------------------------------------------------------

  toast(text: string, tone: Toast['tone'] = 'info') {
    const id = ++this.toastSeq;
    this.toasts = [...this.toasts, { id, text, tone }];
    setTimeout(() => this.dismiss(id), tone === 'error' ? 9000 : 5000);
  }

  dismiss(id: number) {
    this.toasts = this.toasts.filter((t) => t.id !== id);
  }
}

export const store = new AppStore();
