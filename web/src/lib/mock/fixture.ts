// A synthetic but realistic State for the mock: one owner, three harnesses,
// about 25 agent files in a home directory and four repositories.
import { formatCount } from '../format';
import type {
  Access,
  AgentFile,
  ContextEntry,
  ContextItem,
  Finding,
  HarnessName,
  Kind,
  Ref,
  RuntimeContext,
  Scope,
  State,
} from '../types';
import * as C from './content';
import { byteLength, countLines, hashText } from './util';

export const HOME = '/home/alex';
export const CODEX_BUDGET = 32768;

export const P = {
  orgClaude: '/etc/claude-code/managed-settings.json#claudeMd',
  orgCodex: '/etc/codex/requirements.toml#additional_developer_instructions',
  origin: '/home/alex/code/infra/workstation/managed-settings.json#claudeMd',
  userClaude: '/home/alex/.claude/CLAUDE.md',
  userCodex: '/home/alex/.codex/AGENTS.md',
  hub: '/home/alex/.agents/AGENTS.md',
  core: '/home/alex/.agents/core.md',
  personal: '/home/alex/.agents/personal.md',
  publishPage: '/home/alex/.agents/skills/publish-page/SKILL.md',
  tdd: '/home/alex/.agents/skills/tdd/SKILL.md',
  tddLink: '/home/alex/.claude/skills/tdd/SKILL.md',
  docsLookup: '/home/alex/.claude/plugins/cache/docs-tools/docs-lookup/1.4.0/skills/docs-lookup/SKILL.md',
  codeReviewer: '/home/alex/.claude/agents/code-reviewer.md',
  codeAgents: '/home/alex/code/AGENTS.md',
  codeClaude: '/home/alex/code/CLAUDE.md',
  infraAgents: '/home/alex/code/infra/AGENTS.md',
  infraClaude: '/home/alex/code/infra/CLAUDE.md',
  terraform: '/home/alex/code/infra/docs/agents/terraform.md',
  networking: '/home/alex/code/infra/docs/agents/networking.md',
  sevTriage: '/home/alex/code/infra/.claude/agents/sev-triage.md',
  tfPlan: '/home/alex/code/infra/.claude/commands/tf-plan.md',
  webappAgents: '/home/alex/code/webapp/AGENTS.md',
  webappFrontend: '/home/alex/code/webapp/frontend/AGENTS.md',
  releaseNotes: '/home/alex/code/webapp/.claude/commands/release-notes.md',
  tripitAgents: '/home/alex/code/tripit/AGENTS.md',
  tripitClaude: '/home/alex/code/tripit/CLAUDE.md',
  oldRunbook: '/home/alex/code/infra/docs/agents/old-runbook.md',
} as const;

export const CTX = {
  claudeHome: 'claude:/home/alex',
  codexHome: 'codex:/home/alex',
  claudeInfra: 'claude:/home/alex/code/infra',
  codexInfra: 'codex:/home/alex/code/infra',
  agentsInfra: 'agents-md:/home/alex/code/infra',
  claudeWebapp: 'claude:/home/alex/code/webapp',
  codexFrontend: 'codex:/home/alex/code/webapp/frontend',
  agentsFrontend: 'agents-md:/home/alex/code/webapp/frontend',
  claudeTripit: 'claude:/home/alex/code/tripit',
  codexTripit: 'codex:/home/alex/code/tripit',
  agentsTripit: 'agents-md:/home/alex/code/tripit',
  /** Not probed until someone asks. */
  claudeFrontend: 'claude:/home/alex/code/webapp/frontend',
  codexWebapp: 'codex:/home/alex/code/webapp',
} as const;

const REPO = {
  code: '/home/alex/code',
  infra: '/home/alex/code/infra',
  webapp: '/home/alex/code/webapp',
  tripit: '/home/alex/code/tripit',
};

interface Spec {
  id: string;
  kind: Kind;
  scope: Scope;
  content?: string;
  /** For a link: the target file id and the path the link points at. */
  link?: { to: string; target: string };
  repo?: string;
  name?: string;
  description?: string;
  access?: Access;
  /** Harnesses that are not visible from the contexts (commands). */
  harnesses?: HarnessName[];
}

const orgAccess = (): Access => ({
  writable: false,
  reason: 'Owned by root. The workstation provisioner installs it.',
  owner: 'root',
  origin: {
    fileId: P.origin,
    path: '/home/alex/code/infra/workstation/managed-settings.json',
    field: 'claudeMd',
    note: 'Installed on every workstation within the hour after it lands on main.',
  },
});

const chezmoi = (name: string): Access => ({
  writable: true,
  managed: {
    tool: 'chezmoi',
    source: `~/.local/share/chezmoi/dot_agents/${name}`,
    note: 'Run `chezmoi re-add` after editing here so the source keeps your change.',
  },
});

const SPECS: Spec[] = [
  { id: P.orgClaude, kind: 'instruction', scope: 'org', content: C.ORG_POLICY, access: orgAccess() },
  { id: P.orgCodex, kind: 'instruction', scope: 'org', content: C.ORG_POLICY, access: orgAccess() },
  { id: P.origin, kind: 'instruction', scope: 'project', repo: REPO.infra, content: C.ORG_POLICY_ORIGIN },
  {
    id: P.hub,
    kind: 'instruction',
    scope: 'user',
    content: C.HUB,
    access: {
      writable: false,
      reason: 'Built by agents-sync. Edit its parts instead.',
      builtFrom: [P.core, P.personal],
      builtBy: 'agents-sync',
    },
  },
  { id: P.userClaude, kind: 'instruction', scope: 'user', link: { to: P.hub, target: P.hub } },
  { id: P.userCodex, kind: 'instruction', scope: 'user', link: { to: P.hub, target: P.hub } },
  { id: P.core, kind: 'instruction', scope: 'user', content: C.CORE, access: chezmoi('core.md') },
  { id: P.personal, kind: 'instruction', scope: 'user', content: C.PERSONAL, access: chezmoi('personal.md') },
  {
    id: P.codeReviewer,
    kind: 'subagent',
    scope: 'user',
    content: C.CODE_REVIEWER,
    name: 'code-reviewer',
    description: 'Review a diff for correctness bugs before it lands. Reports findings with file and line.',
  },
  {
    id: P.publishPage,
    kind: 'skill',
    scope: 'user',
    content: C.PUBLISH_PAGE,
    name: 'publish-page',
    description: "Publish a finished design doc as a page on the team's own site. Use for plans, specs and designs, not for drafts.",
  },
  {
    id: P.tdd,
    kind: 'skill',
    scope: 'user',
    content: C.TDD,
    name: 'tdd',
    description: 'Test-driven development. Use when building a feature or fixing a bug test-first.',
    access: {
      writable: true,
      managed: {
        tool: 'skills',
        source: 'github.com/example/skills',
        note: 'Refreshed by `skills update`; edits here are replaced',
        warn: true,
      },
    },
  },
  {
    id: P.tddLink,
    kind: 'skill',
    scope: 'user',
    name: 'tdd',
    description: 'Test-driven development. Use when building a feature or fixing a bug test-first.',
    link: { to: P.tdd, target: '/home/alex/.agents/skills/tdd' },
  },
  {
    id: P.docsLookup,
    kind: 'skill',
    scope: 'plugin',
    content: C.DOCS_LOOKUP,
    name: 'docs-lookup',
    description: 'Look up current library documentation before writing code against an API.',
    access: {
      writable: false,
      reason: 'Installed by the docs-tools plugin. Edits would be lost on the next plugin update.',
      owner: 'alex',
    },
  },
  { id: P.codeAgents, kind: 'instruction', scope: 'project', repo: REPO.code, content: C.CODE_AGENTS },
  { id: P.codeClaude, kind: 'instruction', scope: 'project', repo: REPO.code, link: { to: P.codeAgents, target: 'AGENTS.md' } },
  { id: P.infraAgents, kind: 'instruction', scope: 'project', repo: REPO.infra, content: C.INFRA_AGENTS },
  { id: P.infraClaude, kind: 'instruction', scope: 'project', repo: REPO.infra, link: { to: P.infraAgents, target: 'AGENTS.md' } },
  { id: P.terraform, kind: 'doc', scope: 'project', repo: REPO.infra, content: C.TERRAFORM },
  { id: P.networking, kind: 'doc', scope: 'project', repo: REPO.infra, content: C.NETWORKING },
  {
    id: P.sevTriage,
    kind: 'subagent',
    scope: 'project',
    repo: REPO.infra,
    content: C.SEV_TRIAGE,
    name: 'sev-triage',
    description: 'Triage a live incident. Collects alerts, logs and recent deploys for a stack and proposes a first action. Read-only.',
  },
  {
    id: P.tfPlan,
    kind: 'command',
    scope: 'project',
    repo: REPO.infra,
    content: C.TF_PLAN,
    name: 'tf-plan',
    description: 'Run terragrunt plan for one stack and summarise the changes',
    harnesses: ['claude'],
  },
  { id: P.webappAgents, kind: 'instruction', scope: 'project', repo: REPO.webapp, content: C.WEBAPP_AGENTS },
  { id: P.webappFrontend, kind: 'instruction', scope: 'project', repo: REPO.webapp, content: C.WEBAPP_FRONTEND },
  {
    id: P.releaseNotes,
    kind: 'command',
    scope: 'project',
    repo: REPO.webapp,
    content: C.RELEASE_NOTES,
    name: 'release-notes',
    description: 'Draft user-facing release notes from the commits since the last tag',
    harnesses: ['claude'],
  },
  { id: P.tripitAgents, kind: 'instruction', scope: 'project', repo: REPO.tripit, content: C.TRIPIT_AGENTS },
  { id: P.tripitClaude, kind: 'instruction', scope: 'project', repo: REPO.tripit, link: { to: P.tripitAgents, target: 'AGENTS.md' } },
];

export function shorten(path: string): string {
  if (path === HOME) return '~';
  return path.startsWith(HOME + '/') ? '~' + path.slice(HOME.length) : path;
}

function splitId(id: string): { path: string; field?: string } {
  const i = id.indexOf('#');
  return i === -1 ? { path: id } : { path: id.slice(0, i), field: id.slice(i + 1) };
}

/** Describes a file's content the way the backend does. */
export function describe(meta: AgentFile, content: string): AgentFile {
  return { ...meta, size: byteLength(content), lines: countLines(content), hash: hashText(content) };
}

export function lineAt(content: string, n: number): string {
  return content.split('\n')[n - 1] ?? '';
}

/** A repository's branch as the mock's git sees it. */
export interface RepoGit {
  branch: string;
  upstream: string;
  remote: string;
  ahead: number;
  behind: number;
}

export interface Fixture {
  state: State;
  /** Content by the id of the file that holds it; links resolve to their target. */
  contents: Map<string, string>;
  /** Link id to target id. */
  links: Map<string, string>;
  /** What a fresh analysis of each context returns. */
  analysis: Map<string, Finding[]>;
  /** The contexts that appear when an unprobed directory is probed. */
  probeable: Map<string, RuntimeContext>;
  /** Branch, upstream and distance per repository. */
  repos: Map<string, RepoGit>;
}

function iso(t: number): string {
  return new Date(t).toISOString();
}

export function buildFixture(now: number = Date.now()): Fixture {
  const contents = new Map<string, string>();
  const links = new Map<string, string>();
  for (const s of SPECS) {
    if (s.content !== undefined) contents.set(s.id, s.content);
    if (s.link) links.set(s.id, s.link.to);
  }

  const specById = new Map(SPECS.map((s) => [s.id, s]));
  const files: AgentFile[] = SPECS.map((s) => {
    const { path, field } = splitId(s.id);
    const target = s.link ? specById.get(s.link.to) : undefined;
    const content = contents.get(s.link ? s.link.to : s.id) ?? '';
    const access: Access = structuredClone(target?.access ?? s.access ?? { writable: true });
    const meta: AgentFile = {
      id: s.id,
      path,
      realPath: target ? splitId(target.id).path : path,
      display: field ? `${shorten(path)}#${field}` : shorten(path),
      kind: s.kind,
      scope: s.scope,
      harnesses: [...(s.harnesses ?? [])],
      size: 0,
      lines: 0,
      hash: '',
      access,
    };
    if (field) meta.field = field;
    if (s.repo) meta.repo = s.repo;
    if (s.name) meta.name = s.name;
    if (s.description) meta.description = s.description;
    if (s.link) {
      meta.isLink = true;
      meta.linkTarget = s.link.target;
    }
    return describe(meta, content);
  });
  const fileById = new Map(files.map((f) => [f.id, f]));
  const sizeOf = (id: string) => fileById.get(id)?.size ?? 0;

  const harness = {
    claude: { version: '2.1.283' },
    codex: { version: '0.157.1' },
  };

  const entry = (fileId: string, label: string): ContextEntry => ({ fileId, label, bytes: sizeOf(fileId) });
  const claudeSkills: ContextItem[] = [
    { name: 'tdd', fileId: P.tddLink },
    { name: 'docs-lookup', fileId: P.docsLookup },
  ];
  const codexSkills: ContextItem[] = [
    { name: 'publish-page', fileId: P.publishPage },
    { name: 'tdd', fileId: P.tdd },
  ];
  const probed = (id: string, entries: ContextEntry[], extra: Partial<RuntimeContext> = {}): RuntimeContext => {
    const [h, dir] = [id.slice(0, id.indexOf(':')), id.slice(id.indexOf(':') + 1)];
    const isClaude = h === 'claude';
    return {
      id,
      harness: h,
      dir,
      display: shorten(dir),
      source: 'probe',
      version: isClaude ? harness.claude.version : harness.codex.version,
      probedAt: iso(now - 4 * 60_000),
      entries,
      skills: isClaude ? claudeSkills : codexSkills,
      subagents: isClaude ? [{ name: 'code-reviewer', fileId: P.codeReviewer }] : [],
      bytes: entries.reduce((n, e) => n + e.bytes, 0),
      ...extra,
    };
  };
  const statik = (id: string, entries: ContextEntry[]): RuntimeContext => {
    const dir = id.slice(id.indexOf(':') + 1);
    return {
      id,
      harness: 'agents-md',
      dir,
      display: shorten(dir),
      source: 'static',
      entries,
      skills: [],
      subagents: [],
      bytes: entries.reduce((n, e) => n + e.bytes, 0),
    };
  };

  const orgC = entry(P.orgClaude, 'Org policy');
  const orgX = entry(P.orgCodex, 'Org policy (developer message)');
  const userC = entry(P.userClaude, 'User instructions');
  const userX = entry(P.userCodex, 'User instructions');
  const project = (id: string) => entry(id, 'Project instructions');
  const doc = (id: string) => entry(id, 'Project doc');

  const infraSize = sizeOf(P.infraAgents);
  const infraCut: ContextEntry = {
    fileId: P.infraAgents,
    label: 'Project doc',
    bytes: Math.min(infraSize, CODEX_BUDGET),
    truncated: infraSize > CODEX_BUDGET,
    lostBytes: Math.max(0, infraSize - CODEX_BUDGET),
  };

  const contexts: RuntimeContext[] = [
    probed(CTX.claudeHome, [orgC, userC]),
    probed(CTX.codexHome, [orgX, userX]),
    probed(CTX.claudeInfra, [orgC, userC, project(P.codeClaude), project(P.infraClaude)], {
      subagents: [
        { name: 'code-reviewer', fileId: P.codeReviewer },
        { name: 'sev-triage', fileId: P.sevTriage },
      ],
      analysis: { at: iso(now - 3 * 3600_000), cli: 'claude', findings: 2 },
    }),
    probed(CTX.codexInfra, [orgX, userX, infraCut]),
    statik(CTX.agentsInfra, [doc(P.infraAgents)]),
    probed(CTX.claudeWebapp, [orgC, userC, project(P.codeClaude)], {
      analysis: { at: iso(now - 2 * 86400_000), cli: 'claude', findings: 0, stale: true },
    }),
    probed(CTX.codexFrontend, [orgX, userX, doc(P.webappAgents), doc(P.webappFrontend)]),
    statik(CTX.agentsFrontend, [doc(P.webappAgents), doc(P.webappFrontend)]),
    probed(CTX.claudeTripit, [orgC, userC, project(P.codeClaude), project(P.tripitClaude)]),
    probed(CTX.codexTripit, [orgX, userX, doc(P.tripitAgents)]),
    statik(CTX.agentsTripit, [doc(P.tripitAgents)]),
  ];

  // Two directories nobody has probed yet. Claude Code and Codex contexts only
  // exist once probed, so these are candidates until then.
  const probeable = new Map<string, RuntimeContext>([
    [CTX.claudeFrontend, probed(CTX.claudeFrontend, [orgC, userC, project(P.codeClaude)])],
    [CTX.codexWebapp, probed(CTX.codexWebapp, [orgX, userX, doc(P.webappAgents)])],
  ]);
  const unprobed = [...probeable.values()].map((c) => ({ harness: c.harness, dir: c.dir, display: c.display }));

  const repos = new Map<string, RepoGit>([
    [REPO.code, { branch: 'master', upstream: 'origin/master', remote: 'git@git.example.com:alex/code.git', ahead: 0, behind: 0 }],
    [REPO.infra, { branch: 'main', upstream: 'origin/main', remote: 'git@git.example.com:alex/infra.git', ahead: 0, behind: 0 }],
    // A local branch with no upstream: nothing to push to.
    [REPO.webapp, { branch: 'pr-rules', upstream: '', remote: '', ahead: 0, behind: 0 }],
    // Someone pushed from another machine, so a push here is refused.
    [REPO.tripit, { branch: 'main', upstream: 'origin/main', remote: 'git@git.example.com:alex/tripit.git', ahead: 0, behind: 2 }],
  ]);

  // A file loads in a harness when a context of that harness lists it, or
  // reaches it through a link.
  for (const c of contexts) {
    const ids = [...c.entries.map((e) => e.fileId), ...[...c.skills, ...c.subagents].map((i) => i.fileId ?? '')];
    for (let id of ids) {
      const seen = new Set<string>();
      while (id && !seen.has(id)) {
        seen.add(id);
        const f = fileById.get(id);
        if (f && !f.harnesses.includes(c.harness)) f.harnesses.push(c.harness);
        id = links.get(id) ?? '';
      }
    }
  }
  const order = ['claude', 'codex', 'agents-md'];
  for (const f of files) f.harnesses.sort((a, b) => order.indexOf(a) - order.indexOf(b));

  const refs: Ref[] = [
    { from: P.userClaude, to: P.hub, kind: 'symlink', sub: 'link' },
    { from: P.userCodex, to: P.hub, kind: 'symlink', sub: 'link' },
    { from: P.tddLink, to: P.tdd, kind: 'symlink', sub: 'dir' },
    { from: P.codeClaude, to: P.codeAgents, kind: 'symlink', sub: 'link' },
    { from: P.infraClaude, to: P.infraAgents, kind: 'symlink', sub: 'link' },
    { from: P.tripitClaude, to: P.tripitAgents, kind: 'symlink', sub: 'link' },
    { from: P.hub, to: P.core, kind: 'build', sub: 'header', line: 1 },
    { from: P.hub, to: P.personal, kind: 'build', sub: 'header', line: 1 },
    { from: P.orgClaude, to: P.origin, kind: 'build', sub: 'origin' },
    { from: P.orgCodex, to: P.origin, kind: 'build', sub: 'origin' },
    { from: P.codeAgents, to: P.infraAgents, kind: 'mention', sub: 'path', line: 13, text: 'infra/AGENTS.md' },
    { from: P.infraAgents, to: P.terraform, kind: 'mention', sub: 'path', line: 45, text: 'docs/agents/terraform.md' },
    {
      from: P.infraAgents,
      to: P.networking,
      kind: 'mention',
      sub: 'link',
      line: 46,
      text: '[networking notes](docs/agents/networking.md)',
    },
    {
      from: P.infraAgents,
      to: P.oldRunbook,
      kind: 'mention',
      sub: 'path',
      line: 88,
      text: 'docs/agents/old-runbook.md',
      dangling: true,
    },
    { from: P.core, to: P.publishPage, kind: 'mention', sub: 'skill', line: 31, text: '`publish-page` skill' },
    { from: P.sevTriage, to: P.networking, kind: 'mention', sub: 'path', line: 12, text: 'docs/agents/networking.md' },
    { from: P.tfPlan, to: P.terraform, kind: 'mention', sub: 'path', line: 10, text: 'docs/agents/terraform.md' },
    { from: P.webappFrontend, to: P.webappAgents, kind: 'mention', sub: 'path', line: 3, text: '../AGENTS.md' },
  ];

  const infraLines = countLines(C.INFRA_AGENTS);
  const cutLine = C.INFRA_AGENTS.slice(0, CODEX_BUDGET).split('\n').length;
  const lost = infraSize - CODEX_BUDGET;

  const analysisFindings: Finding[] = [
    {
      id: 'contra-commit',
      kind: 'contradiction',
      severity: 'problem',
      source: 'analysis',
      summary: 'personal.md says to ask before committing; the org policy says the agent commits without asking',
      detail:
        'In ~/code/infra, Claude Code loads the org policy first and your personal rules through ~/.claude/CLAUDE.md, so the agent gets both instructions in one session.',
      spans: [
        { fileId: P.personal, startLine: 22, endLine: 22, quote: lineAt(C.PERSONAL, 22) },
        { fileId: P.orgClaude, startLine: 8, endLine: 8, quote: lineAt(C.ORG_POLICY, 8) },
      ],
      contexts: [CTX.claudeInfra],
    },
    {
      id: 'reworded-retry',
      kind: 'reworded',
      severity: 'problem',
      source: 'analysis',
      summary: 'The rule about retrying a failed command appears twice in different words',
      detail:
        'core.md lines 40-42 and infra/AGENTS.md lines 60-61 say the same thing. Both load in Claude Code sessions in ~/code/infra.',
      spans: [
        { fileId: P.core, startLine: 40, endLine: 42, quote: lineAt(C.CORE, 40) },
        { fileId: P.infraAgents, startLine: 60, endLine: 61, quote: lineAt(C.INFRA_AGENTS, 60) },
      ],
      contexts: [CTX.claudeInfra],
    },
  ];

  const findings: Finding[] = [
    {
      id: 'dup-core-infra',
      kind: 'duplicate',
      severity: 'problem',
      source: 'rule',
      summary: 'The same five git rules are in ~/.agents/core.md and ~/code/infra/AGENTS.md',
      detail:
        'Both files load in Claude Code and Codex sessions in ~/code/infra, so the agent reads these rules twice. core.md reaches those sessions through ~/.agents/AGENTS.md, which is built from it.',
      spans: [
        { fileId: P.core, startLine: 10, endLine: 14, quote: lineAt(C.CORE, 10) },
        { fileId: P.infraAgents, startLine: 120, endLine: 124, quote: lineAt(C.INFRA_AGENTS, 120) },
      ],
      contexts: [CTX.claudeInfra, CTX.codexInfra],
    },
    {
      id: 'budget-infra-codex',
      kind: 'budget',
      severity: 'problem',
      source: 'rule',
      summary: `~/code/infra/AGENTS.md is ${formatCount(infraSize)} bytes, over the ${formatCount(CODEX_BUDGET)}-byte project-doc budget of Codex`,
      detail: `Codex cuts project docs at ${formatCount(CODEX_BUDGET)} bytes, so the last ${formatCount(lost)} bytes (line ${cutLine} to the end) never reach the agent.`,
      spans: [{ fileId: P.infraAgents, startLine: cutLine, endLine: infraLines }],
      contexts: [CTX.codexInfra],
    },
    ...analysisFindings.map((f) => structuredClone(f)),
    {
      id: 'dup-webapp-tripit',
      kind: 'duplicate',
      severity: 'hint',
      source: 'rule',
      summary: 'The same pull request rules are in ~/code/webapp/AGENTS.md and ~/code/tripit/AGENTS.md',
      detail:
        'The two files never load in the same session, so no agent reads both. Keep them in step, or let them drift on purpose.',
      spans: [
        { fileId: P.webappAgents, startLine: 5, endLine: 9, quote: lineAt(C.WEBAPP_AGENTS, 5) },
        { fileId: P.tripitAgents, startLine: 7, endLine: 11, quote: lineAt(C.TRIPIT_AGENTS, 7) },
      ],
    },
    {
      id: 'dangling-old-runbook',
      kind: 'dangling',
      severity: 'hint',
      source: 'rule',
      summary: '~/code/infra/AGENTS.md mentions docs/agents/old-runbook.md, which does not exist',
      detail: `Resolved to ${P.oldRunbook}. An agent that follows the pointer finds nothing.`,
      spans: [{ fileId: P.infraAgents, startLine: 88, endLine: 88, quote: 'docs/agents/old-runbook.md' }],
    },
    {
      id: 'notloaded-webapp',
      kind: 'not-loaded',
      severity: 'hint',
      source: 'rule',
      summary: 'Claude Code does not load ~/code/webapp/AGENTS.md in ~/code/webapp',
      detail:
        'Claude Code reads AGENTS.md only when no CLAUDE.md sits further up the tree, and ~/code/CLAUDE.md does. Codex and the AGENTS.md standard still load it.',
      spans: [{ fileId: P.webappAgents, startLine: 1, endLine: 1 }],
      contexts: [CTX.claudeWebapp],
    },
  ];

  const state: State = {
    owner: 'alex',
    home: HOME,
    roots: ['/home/alex/code'],
    scannedAt: iso(now),
    harnesses: [
      { name: 'claude', label: 'Claude Code', available: true, version: harness.claude.version, probe: true },
      { name: 'codex', label: 'Codex', available: true, version: harness.codex.version, probe: true },
      { name: 'agents-md', label: 'AGENTS.md', available: true, probe: false },
    ],
    files,
    refs,
    contexts,
    findings,
    analysis: 'claude',
    unprobed,
  };

  return {
    state,
    contents,
    links,
    analysis: new Map([[CTX.claudeInfra, analysisFindings]]),
    probeable,
    repos,
  };
}
