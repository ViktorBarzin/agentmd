// An in-memory agentmd for the mock: it answers every endpoint in
// docs/api.md from the fixture, and keeps edits, commits and jobs in memory.
import { formatCount } from '../format';
import type {
  AgentFile,
  ApplyRequest,
  CommitRequest,
  Conflict,
  Finding,
  GitResponse,
  Job,
  Proposal,
  PushResponse,
  RuntimeContext,
  SaveRequest,
  SaveResponse,
  State,
} from '../types';
import { buildFixture, CODEX_BUDGET, CTX, describe, lineAt, P, shorten, type RepoGit } from './fixture';
import { unifiedDiff } from './textdiff';
import { byteLength, countLines, fakeSha, hashText } from './util';

export interface MockResponse {
  status: number;
  body: unknown;
}

const ok = (body: unknown): MockResponse => ({ status: 200, body });
const fail = (status: number, error: string, extra: object = {}): MockResponse => ({
  status,
  body: { error, ...extra },
});

/** The analysis of this context fails, so the UI's error state can be seen. */
const FAILING_CONTEXT = CTX.codexTripit;
const JOB_MS = 2000;

interface PendingJob {
  job: Job;
  finishAt: number;
  finish: (job: Job) => void;
}

function isRecord(x: unknown): x is Record<string, unknown> {
  return typeof x === 'object' && x !== null;
}

function str(body: unknown, key: string): string | undefined {
  return isRecord(body) && typeof body[key] === 'string' ? body[key] : undefined;
}

function normalize(s: string): string {
  return s.replace(/\s+/g, ' ').trim().toLowerCase();
}

function removeLines(content: string, start: number, end: number): string {
  const lines = content.split('\n');
  lines.splice(start - 1, end - start + 1);
  // Do not leave two blank lines where the removed block was.
  const i = start - 1;
  if (i > 0 && i < lines.length && lines[i - 1] === '' && lines[i] === '') lines.splice(i, 1);
  return lines.join('\n');
}

function replaceLine(content: string, n: number, text: string): string {
  const lines = content.split('\n');
  lines[n - 1] = text;
  return lines.join('\n');
}

export class MockServer {
  private state: State;
  private contents: Map<string, string>;
  private head: Map<string, string>;
  private links: Map<string, string>;
  private freshAnalysis: Map<string, Finding[]>;
  private probeable: Map<string, RuntimeContext>;
  private repos: Map<string, RepoGit>;
  /** The commit each repository's upstream points at, for push output. */
  private pushedSha = new Map<string, string>();
  private headSha = new Map<string, string>();
  private jobs = new Map<string, PendingJob>();
  private jobSeq = 0;
  private now: () => number;

  constructor(now: () => number = Date.now) {
    this.now = now;
    const fx = buildFixture(now());
    this.state = fx.state;
    this.contents = fx.contents;
    this.head = new Map(fx.contents);
    this.links = fx.links;
    this.freshAnalysis = fx.analysis;
    this.probeable = fx.probeable;
    this.repos = fx.repos;
    for (const repo of this.repos.keys()) {
      const sha = fakeSha('initial ' + repo);
      this.pushedSha.set(repo, sha);
      this.headSha.set(repo, sha);
    }
  }

  private iso(): string {
    return new Date(this.now()).toISOString();
  }

  private canonical(id: string): string {
    let cur = id;
    const seen = new Set<string>();
    while (this.links.has(cur) && !seen.has(cur)) {
      seen.add(cur);
      cur = this.links.get(cur) ?? cur;
    }
    return cur;
  }

  private file(id: string): AgentFile | undefined {
    return this.state.files.find((f) => f.id === id);
  }

  private content(id: string): string {
    return this.contents.get(this.canonical(id)) ?? '';
  }

  private repoOf(id: string): string {
    return this.file(this.canonical(id))?.repo ?? '';
  }

  /** The text git sees: embedded files live inside a JSON settings file. */
  private gitText(id: string, text: string): string {
    const f = this.file(this.canonical(id));
    if (!f?.field) return text;
    return JSON.stringify({ permissions: { defaultMode: 'default' }, [f.field]: text }, null, 2) + '\n';
  }

  private diffFor(id: string): string {
    const key = this.canonical(id);
    const repo = this.repoOf(key);
    if (!repo) return '';
    const f = this.file(key);
    if (!f) return '';
    const before = this.gitText(key, this.head.get(key) ?? '');
    const after = this.gitText(key, this.contents.get(key) ?? '');
    return unifiedDiff(before, after, f.path.slice(repo.length + 1), [hashText(before), hashText(after)]);
  }

  private write(id: string, content: string) {
    const key = this.canonical(id);
    this.contents.set(key, content);
    this.state.files = this.state.files.map((f) => (this.canonical(f.id) === key ? describe(f, content) : f));
    this.afterChange(key);
  }

  /** Keeps findings, context sizes and analysis freshness in step with the files. */
  private afterChange(key: string) {
    const kept: Finding[] = [];
    for (const f of this.state.findings) {
      const next = this.relocate(f);
      if (next) kept.push(next);
    }
    this.state.findings = kept;
    this.updateBudget();

    const sizes = new Map(this.state.files.map((f) => [f.id, f.size]));
    for (const c of this.state.contexts) {
      for (const e of c.entries) {
        if (e.truncated === undefined) e.bytes = sizes.get(e.fileId) ?? e.bytes;
      }
      c.bytes = c.entries.reduce((n, e) => n + e.bytes, 0);
      const touched = c.entries.some((e) => {
        const k = this.canonical(e.fileId);
        if (k === key) return true;
        const built = this.file(k)?.access.builtFrom ?? [];
        return built.includes(key);
      });
      if (touched && c.analysis) c.analysis.stale = true;
    }
  }

  /** Moves a finding's spans to where their quotes are now, or drops it. */
  private relocate(f: Finding): Finding | null {
    const spans = [];
    for (const s of f.spans) {
      if (!s.quote) {
        spans.push(s);
        continue;
      }
      // A span still holds its quote when its first line holds the quote's first line.
      const lines = this.content(s.fileId).split('\n');
      const first = normalize(s.quote.split('\n')[0]);
      if (normalize(lines[s.startLine - 1] ?? '').includes(first)) {
        spans.push(s);
        continue;
      }
      const at = lines.findIndex((l) => normalize(l).includes(first));
      if (at === -1) return null;
      const len = s.endLine - s.startLine;
      spans.push({ ...s, startLine: at + 1, endLine: at + 1 + len });
    }
    return { ...f, spans };
  }

  private updateBudget() {
    const text = this.content(P.infraAgents);
    const size = byteLength(text);
    const entry = this.state.contexts.find((c) => c.id === CTX.codexInfra)?.entries.find((e) => e.fileId === P.infraAgents);
    if (entry) {
      entry.bytes = Math.min(size, CODEX_BUDGET);
      entry.truncated = size > CODEX_BUDGET;
      entry.lostBytes = Math.max(0, size - CODEX_BUDGET);
    }
    const i = this.state.findings.findIndex((f) => f.id === 'budget-infra-codex');
    if (size <= CODEX_BUDGET) {
      if (i !== -1) this.state.findings.splice(i, 1);
      return;
    }
    if (i === -1) return;
    const cutLine = text.slice(0, CODEX_BUDGET).split('\n').length;
    const f = this.state.findings[i];
    f.summary = `~/code/infra/AGENTS.md is ${formatCount(size)} bytes, over the ${formatCount(CODEX_BUDGET)}-byte project-doc budget of Codex`;
    f.detail = `Codex cuts project docs at ${formatCount(CODEX_BUDGET)} bytes, so the last ${formatCount(size - CODEX_BUDGET)} bytes (line ${cutLine} to the end) never reach the agent.`;
    f.spans = [{ fileId: P.infraAgents, startLine: cutLine, endLine: countLines(text) }];
  }

  private tick() {
    const t = this.now();
    for (const p of this.jobs.values()) {
      if (p.job.status === 'running' && p.finishAt <= t) {
        p.job.finishedAt = this.iso();
        p.finish(p.job);
      }
    }
  }

  private startJob(partial: Pick<Job, 'kind' | 'context' | 'findingId'>, finish: (job: Job) => void): Job {
    const job: Job = { id: `job-${++this.jobSeq}`, status: 'running', startedAt: this.iso(), ...partial };
    this.jobs.set(job.id, { job, finishAt: this.now() + JOB_MS, finish });
    return structuredClone(job);
  }

  private snapshot(): State {
    return structuredClone(this.state);
  }

  // ---- endpoints -------------------------------------------------------

  handle(method: string, path: string, query: URLSearchParams, body: unknown): MockResponse {
    this.tick();
    const route = `${method} ${path}`;
    if (route === 'GET /state') return ok(this.snapshot());
    if (route === 'POST /scan') {
      this.state.scannedAt = this.iso();
      return ok(this.snapshot());
    }
    if (route === 'GET /file') return this.getFile(query.get('id') ?? '');
    if (route === 'PUT /file') return this.save(body);
    if (route === 'GET /git') return this.git(query.get('id') ?? '');
    if (route === 'POST /commit') return this.commit(body);
    if (route === 'POST /push') return this.push(body);
    if (route === 'POST /probe') return this.probe(body);
    if (route === 'POST /analyse') return this.analyse(str(body, 'context') ?? '');
    if (route === 'POST /fix') return this.fix(str(body, 'findingId') ?? '');
    if (route === 'POST /apply') return this.apply(body);
    if (method === 'GET' && path.startsWith('/jobs/')) {
      const p = this.jobs.get(decodeURIComponent(path.slice('/jobs/'.length)));
      return p ? ok(structuredClone(p.job)) : fail(404, 'No such job.');
    }
    return fail(404, `No endpoint ${method} /api${path}.`);
  }

  private getFile(id: string): MockResponse {
    const f = this.file(id);
    if (!f) return fail(404, `No agent file with id ${id}.`);
    return ok({ file: structuredClone(f), content: this.content(id) });
  }

  private checkWritable(f: AgentFile): MockResponse | null {
    if (!f.access.writable) return fail(403, `${f.display} is read-only: ${f.access.reason ?? 'not writable'}`);
    return null;
  }

  private saveOne(req: SaveRequest): SaveResponse {
    this.write(req.id, req.content);
    const f = this.file(req.id);
    if (!f) throw new Error('file vanished');
    return { file: structuredClone(f), content: this.content(req.id), repo: this.repoOf(req.id), diff: this.diffFor(req.id) };
  }

  private save(body: unknown): MockResponse {
    const id = str(body, 'id');
    const content = str(body, 'content');
    const baseHash = str(body, 'baseHash');
    if (id === undefined || content === undefined || baseHash === undefined) {
      return fail(400, 'A save needs id, content and baseHash.');
    }
    const f = this.file(id);
    if (!f) return fail(404, `No agent file with id ${id}.`);
    const denied = this.checkWritable(f);
    if (denied) return denied;
    const current = this.content(id);
    const hash = hashText(current);
    if (baseHash !== hash) {
      const conflict: Conflict = { error: `${f.display} changed on disk since you opened it.`, current, hash };
      return { status: 409, body: conflict };
    }
    return ok(this.saveOne({ id, content, baseHash }));
  }

  private git(id: string): MockResponse {
    const f = this.file(id);
    if (!f) return fail(404, `No agent file with id ${id}.`);
    const repo = this.repoOf(id);
    const diff = this.diffFor(id);
    const g = this.repos.get(repo);
    const res: GitResponse = {
      repo,
      diff,
      dirty: diff !== '',
      branch: g?.branch ?? '',
      upstream: g?.upstream ?? '',
      ahead: g?.ahead ?? 0,
      behind: g?.behind ?? 0,
    };
    return ok(res);
  }

  private push(body: unknown): MockResponse {
    const id = str(body, 'id');
    if (id === undefined) return fail(400, 'A push needs the id of a file in the repository.');
    if (!this.file(id)) return fail(404, `No agent file with id ${id}.`);
    const repo = this.repoOf(id);
    const g = this.repos.get(repo);
    if (!repo || !g) return fail(400, 'This file is not in a git repository, so there is nothing to push.');
    if (!g.upstream) {
      return fail(
        409,
        `fatal: The current branch ${g.branch} has no upstream branch.\n` +
          'agentmd only fast-forwards an existing upstream; set one from a terminal first.',
      );
    }
    if (g.behind > 0) {
      return fail(
        409,
        `To ${g.remote}\n` +
          ` ! [rejected]        ${g.branch} -> ${g.branch} (non-fast-forward)\n` +
          `error: failed to push some refs to '${g.remote}'\n` +
          'hint: Updates were rejected because the tip of your current branch is behind\n' +
          "hint: its remote counterpart. Integrate the remote changes (e.g. 'git pull ...')\n" +
          'hint: before pushing again.\n' +
          "hint: See the 'Note about fast-forwards' in 'git push --help' for details.",
      );
    }
    if (g.ahead === 0) return fail(409, `Everything up-to-date: ${g.upstream} already has every commit on ${g.branch}.`);
    const from = this.pushedSha.get(repo) ?? fakeSha(repo);
    const to = this.headSha.get(repo) ?? fakeSha(repo + this.now());
    this.pushedSha.set(repo, to);
    g.ahead = 0;
    const res: PushResponse = {
      repo,
      branch: g.branch,
      upstream: g.upstream,
      output: `To ${g.remote}\n   ${from.slice(0, 7)}..${to.slice(0, 7)}  ${g.branch} -> ${g.branch}\n`,
    };
    return ok(res);
  }

  private commit(body: unknown): MockResponse {
    const req: CommitRequest = {
      message: str(body, 'message') ?? '',
      ids: isRecord(body) && Array.isArray(body.ids) ? body.ids.filter((x): x is string => typeof x === 'string') : [],
    };
    const message = req.message.trim();
    const ids = req.ids;
    if (!message) return fail(400, 'Write a commit message first.');
    if (ids.length === 0) return fail(400, 'Name at least one file to commit.');
    const keys = [...new Set(ids.map((id) => this.canonical(id)))];
    const repos = new Set(keys.map((k) => this.repoOf(k)));
    if (repos.size !== 1) return fail(400, 'These files are in different repositories. Commit them one repository at a time.');
    const repo = [...repos][0];
    if (!repo) return fail(400, 'This file is not in a git repository, so there is nothing to commit.');
    if (/^wip\b/i.test(message)) {
      return fail(
        422,
        'git commit failed (exit status 1):\n' +
          'commit-msg hook: a subject starting with "wip" is not allowed on this branch.\n' +
          'Write the subject as what changed, for example "Move the git rules into core.md".',
      );
    }
    const changed = keys.filter((k) => this.head.get(k) !== this.contents.get(k));
    if (changed.length === 0) return fail(400, 'Nothing to commit: the file matches the last commit.');
    let added = 0;
    let removed = 0;
    for (const k of changed) {
      const d = this.diffFor(k).split('\n');
      added += d.filter((l) => l.startsWith('+') && !l.startsWith('+++')).length;
      removed += d.filter((l) => l.startsWith('-') && !l.startsWith('---')).length;
      this.head.set(k, this.contents.get(k) ?? '');
    }
    const sha = fakeSha(message + this.now());
    const g = this.repos.get(repo);
    if (g) g.ahead++;
    this.headSha.set(repo, sha);
    const subject = message.split('\n')[0];
    const files = changed.length === 1 ? '1 file changed' : `${changed.length} files changed`;
    const output = `[${g?.branch ?? 'main'} ${sha.slice(0, 7)}] ${subject}\n ${files}, ${added} insertion${added === 1 ? '' : 's'}(+), ${removed} deletion${removed === 1 ? '' : 's'}(-)\n`;
    return ok({ repo, commit: sha, output });
  }

  /** Probes the listed contexts, or every Claude Code and Codex context when the list is empty. */
  private probe(body: unknown): MockResponse {
    const list = isRecord(body) && Array.isArray(body.contexts) ? body.contexts.filter((x): x is string => typeof x === 'string') : [];
    const all = list.length === 0;
    for (const c of this.state.contexts) {
      if (c.source === 'probe' && (all || list.includes(c.id))) c.probedAt = this.iso();
    }
    const sizes = new Map(this.state.files.map((f) => [f.id, f.size]));
    const still = [];
    for (const cand of this.state.unprobed ?? []) {
      const id = `${cand.harness}:${cand.dir}`;
      const template = this.probeable.get(id);
      if (!template || !(all || list.includes(id))) {
        still.push(cand);
        continue;
      }
      const entries = template.entries.map((e) => ({ ...e, bytes: sizes.get(e.fileId) ?? e.bytes }));
      this.state.contexts.push({
        ...structuredClone(template),
        entries,
        bytes: entries.reduce((n, e) => n + e.bytes, 0),
        probedAt: this.iso(),
      });
    }
    this.state.unprobed = still;
    return ok(this.snapshot());
  }

  private analyse(contextId: string): MockResponse {
    const cli = this.state.analysis;
    if (!cli) return fail(400, 'No agent CLI was found, so analysis is not available.');
    const ctx = this.state.contexts.find((c) => c.id === contextId);
    if (!ctx) return fail(404, `No context ${contextId}.`);
    const job = this.startJob({ kind: 'analyse', context: contextId }, (j) => {
      if (contextId === FAILING_CONTEXT) {
        j.status = 'error';
        j.error = `${cli} -p exited with status 1: the prompt is longer than the model's context window.`;
        return;
      }
      const fresh = (this.freshAnalysis.get(contextId) ?? [])
        .map((f) => this.relocate(structuredClone(f)))
        .filter((f): f is Finding => f !== null);
      this.state.findings = [
        ...this.state.findings.filter((f) => !(f.source === 'analysis' && f.contexts?.includes(contextId))),
        ...fresh,
      ];
      ctx.analysis = { at: this.iso(), cli, findings: fresh.length };
      j.status = 'done';
      j.findings = structuredClone(fresh);
    });
    return ok(job);
  }

  private proposalFor(f: Finding): { proposal?: Proposal; error?: string } {
    const edit = (fileId: string, after: string) => {
      const before = this.content(fileId);
      const meta = this.file(fileId);
      return { fileId, display: meta?.display ?? shorten(fileId), baseHash: hashText(before), before, after };
    };
    const span = (fileId: string) => f.spans.find((s) => s.fileId === fileId);
    switch (f.id) {
      case 'dup-core-infra': {
        const s = span(P.infraAgents);
        if (!s) break;
        const text = this.content(P.infraAgents);
        // Take the "## Git" heading with the list when nothing else sits under it.
        const from = lineAt(text, s.startLine - 2) === '## Git' ? s.startLine - 2 : s.startLine;
        return {
          proposal: {
            findingId: f.id,
            rationale:
              'Both files load in ~/code/infra. core.md applies in every repository, so keep the rules there and remove the copy from infra/AGENTS.md. This also brings infra/AGENTS.md a little closer to the Codex budget.',
            edits: [edit(P.infraAgents, removeLines(text, from, s.endLine))],
          },
        };
      }
      case 'reworded-retry': {
        const s = span(P.infraAgents);
        if (!s) break;
        return {
          proposal: {
            findingId: f.id,
            rationale:
              'core.md already says this for every repository, in more detail. Remove the reworded copy from infra/AGENTS.md so the agent reads the rule once.',
            edits: [edit(P.infraAgents, removeLines(this.content(P.infraAgents), s.startLine, s.endLine))],
          },
        };
      }
      case 'contra-commit': {
        const s = span(P.personal);
        if (!s) break;
        return {
          proposal: {
            findingId: f.id,
            rationale:
              'The org policy sits above your own files on this workstation and tells the agent to commit without asking, so it cannot follow both rules. This keeps the part of your rule the policy allows: you still hear about every commit.',
            edits: [edit(P.personal, replaceLine(this.content(P.personal), s.startLine, '- Tell me what you committed at the end of each session.'))],
          },
        };
      }
      case 'dangling-old-runbook': {
        const s = span(P.infraAgents);
        if (!s) break;
        const text = this.content(P.infraAgents);
        const from = lineAt(text, s.startLine - 1) === '## Old material' ? s.startLine - 1 : s.startLine;
        return {
          proposal: {
            findingId: f.id,
            rationale:
              'docs/agents/old-runbook.md is gone, and the Incidents section above covers the current flow. Remove the pointer and its heading.',
            edits: [edit(P.infraAgents, removeLines(text, from, s.endLine))],
          },
        };
      }
      case 'budget-infra-codex':
        return {
          error: `No edit proposed. Bringing ~/code/infra/AGENTS.md under ${formatCount(CODEX_BUDGET)} bytes means moving the stack reference somewhere else, and where it belongs is your call.`,
        };
      case 'dup-webapp-tripit':
        return {
          error: 'No edit proposed. webapp and tripit are separate repositories that never load together, so keeping both copies is reasonable.',
        };
      case 'notloaded-webapp':
        return {
          error: 'No edit proposed. The fix is a new file, ~/code/webapp/CLAUDE.md linking to AGENTS.md, and fix proposals only edit existing files.',
        };
    }
    return { error: 'The lines this finding points at have changed. Rescan, then try again.' };
  }

  private fix(findingId: string): MockResponse {
    if (!this.state.analysis) return fail(400, 'No agent CLI was found, so fix proposals are not available.');
    const finding = this.state.findings.find((f) => f.id === findingId);
    if (!finding) return fail(404, `No finding ${findingId}.`);
    const job = this.startJob({ kind: 'fix', findingId }, (j) => {
      const current = this.state.findings.find((f) => f.id === findingId);
      const result = current ? this.proposalFor(current) : { error: 'This finding is gone. Rescan to see the current list.' };
      if (result.proposal) {
        j.status = 'done';
        j.proposal = result.proposal;
      } else {
        j.status = 'error';
        j.error = result.error;
      }
    });
    return ok(job);
  }

  private apply(body: unknown): MockResponse {
    const raw: unknown[] = isRecord(body) && Array.isArray(body.edits) ? body.edits : [];
    const req: ApplyRequest = { edits: [] };
    for (const e of raw) {
      const id = str(e, 'id');
      const content = str(e, 'content');
      const baseHash = str(e, 'baseHash');
      if (id === undefined || content === undefined || baseHash === undefined) {
        return fail(400, 'Each edit needs id, content and baseHash.');
      }
      req.edits.push({ id, content, baseHash });
    }
    const edits = req.edits;
    if (edits.length === 0) return fail(400, 'Nothing to apply.');
    for (const e of edits) {
      const f = this.file(e.id);
      if (!f) return fail(404, `No agent file with id ${e.id}.`);
      const denied = this.checkWritable(f);
      if (denied) return denied;
      const current = this.content(e.id);
      if (hashText(current) !== e.baseHash) {
        return fail(409, `${f.display} changed after the proposal was made. Propose the fix again.`, {
          current,
          hash: hashText(current),
        });
      }
    }
    return ok({ results: edits.map((e) => this.saveOne(e)) });
  }

  // ---- controls for trying the UI --------------------------------------

  /** Changes a file behind the UI's back, as another editor would. */
  touch(id: string): string {
    const key = this.canonical(id);
    const lines = this.content(key).split('\n');
    lines.splice(3, 0, '- Added in another editor while this page was open.');
    this.write(key, lines.join('\n'));
    return hashText(this.content(key));
  }

  setAnalysis(cli: string) {
    this.state.analysis = cli || undefined;
  }

  lineOf(id: string, n: number): string {
    return lineAt(this.content(id), n);
  }
}
