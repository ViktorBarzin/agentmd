// Mirrors internal/model/model.go. Keep the two in step.

export type Kind = 'instruction' | 'skill' | 'subagent' | 'command' | 'doc';
export type Scope = 'org' | 'user' | 'project' | 'plugin' | 'other';
export type HarnessName = 'claude' | 'codex' | 'agents-md' | string;
export type RefKind = 'symlink' | 'build' | 'mention' | 'load';
export type Severity = 'problem' | 'hint';
export type FindingKind =
  | 'duplicate'
  | 'reworded'
  | 'contradiction'
  | 'dangling'
  | 'budget'
  | 'not-loaded';

export interface Origin {
  fileId?: string;
  path: string;
  field?: string;
  note?: string;
}

export interface Managed {
  tool: string;
  source?: string;
  note?: string;
  /** Edits in place would be lost on the tool's next run. */
  warn?: boolean;
}

export interface Access {
  writable: boolean;
  reason?: string;
  owner?: string;
  builtFrom?: string[];
  builtBy?: string;
  origin?: Origin;
  managed?: Managed;
}

export interface AgentFile {
  /** The path it was discovered at, or "<path>#<field>" for an embedded file. */
  id: string;
  path: string;
  realPath: string;
  /** Home-shortened path for display, e.g. "~/code/infra/AGENTS.md". */
  display: string;
  field?: string;
  kind: Kind;
  scope: Scope;
  harnesses: HarnessName[];
  repo?: string;
  name?: string;
  description?: string;
  size: number;
  lines: number;
  hash: string;
  isLink?: boolean;
  linkTarget?: string;
  missing?: boolean;
  access: Access;
}

export interface Ref {
  from: string;
  /** A file id, or the resolved path when the reference dangles. */
  to: string;
  kind: RefKind;
  /** import | header | config | origin | link | path | skill | dir */
  sub?: string;
  line?: number;
  text?: string;
  dangling?: boolean;
  context?: string;
}

export interface ContextEntry {
  fileId: string;
  label?: string;
  bytes: number;
  truncated?: boolean;
  lostBytes?: number;
}

export interface ContextItem {
  name: string;
  fileId?: string;
}

export interface AnalysisInfo {
  at: string;
  cli: string;
  stale?: boolean;
  findings: number;
}

export interface RuntimeContext {
  /** "<harness>:<dir>" */
  id: string;
  harness: HarnessName;
  dir: string;
  display: string;
  source: 'probe' | 'static';
  version?: string;
  probedAt?: string;
  entries: ContextEntry[];
  skills: ContextItem[];
  subagents: ContextItem[];
  bytes: number;
  error?: string;
  analysis?: AnalysisInfo;
}

export interface Span {
  fileId: string;
  /** 1-based, inclusive. */
  startLine: number;
  endLine: number;
  quote?: string;
}

export interface Finding {
  id: string;
  kind: FindingKind;
  severity: Severity;
  summary: string;
  detail?: string;
  spans: Span[];
  contexts?: string[];
  source: 'rule' | 'analysis';
}

export interface HarnessInfo {
  name: HarnessName;
  label: string;
  available: boolean;
  version?: string;
  probe: boolean;
}

/** A Claude Code or Codex context that could be probed but has not been yet. */
export interface Candidate {
  harness: HarnessName;
  dir: string;
  display: string;
}

export interface State {
  owner: string;
  home: string;
  roots: string[];
  scannedAt: string;
  harnesses: HarnessInfo[];
  files: AgentFile[];
  refs: Ref[];
  contexts: RuntimeContext[];
  findings: Finding[];
  /** The CLI used for analysis ("claude" or "codex"), empty when none is installed. */
  analysis?: string;
  /** Directories whose Claude Code or Codex context has not been probed yet. */
  unprobed?: Candidate[];
}

export interface FileResponse {
  file: AgentFile;
  content: string;
}

export interface SaveRequest {
  id: string;
  content: string;
  baseHash: string;
}

export interface SaveResponse {
  file: AgentFile;
  content: string;
  /** The repository holding the file, empty when it is not in one. */
  repo: string;
  /** `git diff` for the file after the save, empty when clean or not in git. */
  diff: string;
}

/** Body of a 409 answer to a save. */
export interface Conflict {
  error: string;
  current: string;
  hash: string;
}

export interface GitResponse {
  repo: string;
  diff: string;
  dirty: boolean;
  /** The current branch, empty when the file is not in a repository. */
  branch: string;
  /** The branch's upstream, such as "origin/main", empty when it has none. */
  upstream: string;
  /** Commits on the branch that the upstream lacks. */
  ahead: number;
  /** Commits on the upstream that the branch lacks. */
  behind: number;
}

export interface CommitRequest {
  ids: string[];
  message: string;
}

export interface CommitResponse {
  repo: string;
  commit: string;
  output: string;
}

/** Pushes the repository holding this file. It only fast-forwards the upstream. */
export interface PushRequest {
  id: string;
}

/** A refused push answers 409 with git's refusal in `error`. */
export interface PushResponse {
  repo: string;
  branch: string;
  upstream: string;
  output: string;
}

export interface ProposalEdit {
  fileId: string;
  display: string;
  baseHash: string;
  before: string;
  after: string;
}

export interface Proposal {
  findingId: string;
  rationale: string;
  edits: ProposalEdit[];
}

export interface Job {
  id: string;
  kind: 'analyse' | 'fix';
  status: 'running' | 'done' | 'error';
  context?: string;
  findingId?: string;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  findings?: Finding[];
  proposal?: Proposal;
}

export interface ApplyRequest {
  edits: SaveRequest[];
}

export interface ApplyResponse {
  results: SaveResponse[];
}

/** Every error answer has this shape. */
export interface ApiError {
  error: string;
}
