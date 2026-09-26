// Synthetic agent files for the mock. Everything here is made up: the owner
// "alex", the repositories and every rule. Line numbers matter, because the
// fixture's findings and references point at them (see fixture.test.ts).

function text(lines: string[]): string {
  return lines.join('\n') + '\n';
}

export const ORG_POLICY = text([
  '# Workstation policy',
  '',
  'You are running as your own OS user on a shared development workstation.',
  'These rules apply to every user and sit above your own settings.',
  '',
  '## Git',
  '',
  '- The agent does all git work itself: branch, commit, push and open pull requests. Never ask the user to commit or push.',
  '- The commit message is the audit trail. The subject says what changed and the body says why.',
  '- Never skip hooks, and never force-push the main branch.',
  '',
  '## Infrastructure',
  '',
  '- Infrastructure changes go through Terraform. Never change live resources by hand.',
  '- CI applies committed changes after they land on main. Check the live result afterwards.',
  '',
  '## Secrets',
  '',
  '- Secrets live in the vault. Never print one in chat or write one outside your home directory.',
  '- Keep credential files at mode 600.',
  '',
  '## Access',
  '',
  '- Stay inside your permission tier. Ask an administrator when a task needs more access.',
  '- Read logs and metrics from the shared observability stack, not from one host.',
  '',
  '## Cost',
  '',
  '- Do not start paid services, trials or subscriptions without written approval.',
]);

/** The source of the org policy in the infra repository, one section ahead of the installed copies. */
export const ORG_POLICY_ORIGIN =
  ORG_POLICY + text(['', '## Services', '', '- Check the service catalog before reaching for a public service.']);

export const CORE = text([
  '# Core rules',
  '',
  'How I want every agent to work, whatever the repository.',
  '',
  '## Replies',
  '',
  '- Lead with the answer, then the evidence. Keep replies short.',
  '',
  '## Git',
  '- Commit on a branch, never directly on main.',
  '- Stage files by name. Never run `git add -A` or `git add .`.',
  '- Write the subject as what changed and the body as why.',
  '- Run the tests before every push and wait for CI to pass.',
  '- Never force-push a branch someone else has pulled.',
  '',
  '## Testing',
  '',
  '- Write the failing test first for code with testable behaviour.',
  '- Prefer table-driven and property-based tests.',
  '- Terraform, config and docs changes do not need tests.',
  '',
  '## Done means verified',
  '',
  '- Before saying a change works, exercise it the way a person would.',
  '- For a UI, open the page and look at a screenshot.',
  '- For a CLI, run the built binary and read what it printed.',
  '- Say what you could not verify in the same message as the claim.',
  '',
  '## Publishing',
  '',
  '- Finished design docs go out through the `publish-page` skill.',
  '- Drafts and research notes stay in the repository.',
  '',
  '## Memory',
  '',
  '- Store corrections the moment they happen.',
  '- Keep each memory short and self-contained.',
  '',
  '## Errors',
  'When a command fails, read the whole error before you retry.',
  'Do not run the same command more than twice without changing',
  'something, and say what you changed each time.',
  '',
  '## Cost',
  '',
  '- Stay on free tiers. Ask before anything that costs money.',
]);

export const PERSONAL = text([
  '# Personal preferences',
  '',
  '## About me',
  '',
  '- I work mostly in Go, TypeScript and Terraform.',
  '- I read replies on my phone, so keep tables narrow.',
  '- Dates in ISO 8601, times in 24-hour format.',
  '',
  '## Tools',
  '',
  '- Use `rg` for search and `fd` for finding files.',
  "- Format Go with `gofumpt` and TypeScript with the repository's formatter.",
  '- Prefer a Makefile target over a hand-written command when one exists.',
  '',
  '## Projects',
  '',
  '- webapp is the customer-facing app. Treat its main branch as production.',
  '- tripit is a side project. Experiments are welcome there.',
  '',
  '## Habits',
  '',
  '- Ask before committing anything, even on a branch.',
  '- End each session with a short summary of what changed.',
  '- When a task grows past an hour, stop and check the plan with me.',
  '',
  '## Writing',
  '',
  '- Plain English, short sentences, no filler.',
  '- No em dashes.',
]);

export const HUB =
  '<!-- Built by agents-sync from core.md and personal.md. Edit those files, not this one. -->\n\n' +
  CORE +
  '\n' +
  PERSONAL;

export const CODE_AGENTS = text([
  '# ~/code',
  '',
  'A workspace of separate repositories. Each folder is its own git repository',
  'with its own history, CI and AGENTS.md.',
  '',
  '| folder | what it is |',
  '|---|---|',
  '| infra | Terraform for the home cluster and the workstation provisioner |',
  '| webapp | the customer-facing web app |',
  '| tripit | a trip planner, a side project |',
  '',
  "Read a project's own instructions before working in it. infra has the longest",
  'one: infra/AGENTS.md covers stacks, secrets and the apply flow.',
  '',
  '## Working across repositories',
  '',
  '- One change per repository per branch. Do not mix repositories in one commit.',
  '- Shared scripts live in infra/scripts. Do not copy them into other repositories.',
  '- When a change spans repositories, land the one others depend on first.',
]);

const INFRA_HEAD = [
  '# infra',
  '',
  'Terraform and Terragrunt for the home cluster, and the files the provisioner',
  'installs on every workstation.',
  '',
  '## Layout',
  '',
  '- `stacks/<name>/` holds one stack per service, each with its own `terragrunt.hcl` and state.',
  '- `modules/` holds shared Terraform modules. A module change needs a plan for every stack that uses it.',
  '- `workstation/` holds what the provisioner installs on each workstation, including the org policy.',
  '- `scripts/` holds helpers. Each script prints its usage with `--help`.',
  '- `docs/` holds architecture notes, runbooks and post-mortems.',
  '',
  '## Before you change anything',
  '',
  "1. Read the stack's README and the architecture note for the service.",
  '2. Look at the live state first. `kubectl get` and the dashboards are read-only and safe.',
  '3. Claim the stack so other sessions can see you are working on it.',
  '4. Run a plan, and paste the summary into the conversation before you apply.',
  '',
  '## Applying',
  '',
  '- CI applies every stack that changed when a commit lands on main.',
  '- Apply by hand only when CI is down, and say so in the commit message.',
  '- A plan that wants to destroy a stateful resource (a database, a volume, a bucket) needs a second look from a person.',
  '- After an apply, check the rollout: pods ready, the ingress answering, no new alerts.',
  '',
  '## Secrets',
  '',
  '- Secrets live in the vault under `secret/<stack>`. Terraform reads them with the vault provider.',
  '- Never put a secret in a `.tfvars` file, a commit or a log line.',
  '- Rotating a secret means updating the vault first, then restarting the workloads that read it.',
  '',
  '## State',
  '',
  '- Remote state lives in the object store, one key per stack, with locking.',
  '- Never edit state by hand. Use `terragrunt state mv` or an import block, and explain why in the commit.',
  '- A stuck lock usually means an apply died. Check CI before you force-unlock.',
  '',
  '## Conventions',
  '',
  '- Resource names use the stack name as a prefix.',
  '- Every stack sets the same default labels: `stack`, `owner`, `managed-by`.',
  '- Pin provider versions in the stack, never in a module.',
  '- Terraform conventions for naming, providers and state layout are in docs/agents/terraform.md.',
  '- Network layout, VLANs and firewall zones are in the [networking notes](docs/agents/networking.md).',
  '',
  '## Kubernetes',
  '',
  '- Workloads are defined in Terraform with the kubernetes provider. No `kubectl apply` from a laptop.',
  '- Every deployment sets resource requests and limits.',
  '- Health checks are required for anything behind the ingress.',
  '- Use the shared ingress module; it adds TLS, auth and the default middlewares.',
  '',
  '## Debugging',
  '',
  '- Start from the dashboards and the log search, not from one pod.',
  '- Record what you found in the incident note even when the fix is small.',
  '',
  'If a terraform apply errors, read the full output before anything else. Never',
  're-run an identical command a third time; say what you changed between tries.',
  '',
  '## Backups',
  '',
  '- Databases are backed up nightly to the object store; volumes weekly.',
  '- A restore test runs on the first Monday of each month. Its report lands in #ops.',
  '- Before any migration, take a manual backup and note its id in the plan.',
  '',
  '## DNS and certificates',
  '',
  "- DNS records are managed in the `dns` stack. Never edit them in the provider's console.",
  '- Certificates come from the cluster issuer. A certificate that fails to renew raises an alert a week ahead.',
  '',
  '## Monitoring',
  '',
  '- Every stack exports metrics and ships logs to the shared stack.',
  '- New alerts need a runbook link in their annotations.',
  '- Silence an alert only with an expiry and a reason.',
  '',
  '## Incidents',
  '',
  "- Page the owner of the stack first. The owner is in the stack's labels.",
  '- Use the `sev-triage` subagent to collect alerts, logs and recent deploys.',
  '- Keep a timeline in the incident note while you work.',
  '- After the fix, write a post-mortem in `docs/post-mortems/` within two days.',
  '',
  '## Old material',
  'For the pre-2025 incident flow, see docs/agents/old-runbook.md.',
  '',
  '## Workstation',
  '',
  '- The provisioner runs every hour and installs `workstation/` onto each workstation.',
  '- The org policy lives in `workstation/managed-settings.json`. Changes reach every user within the hour after they land on main.',
  '- Test provisioner changes on one workstation first with `--limit`.',
  '',
  '## Cost',
  '',
  '- The cluster runs on hardware we own. New paid services need written approval.',
  '- Prefer the self-hosted service when one exists.',
  '',
  '## Reviews',
  '',
  '- Every change to `modules/` or `workstation/` gets a second pair of eyes.',
  '- Small stack changes can land after CI passes.',
  '- Explain in the commit body why the change is safe to apply.',
  '',
  '## Access',
  '',
  '- Namespace owners can change their own stacks. Cluster-wide changes need an administrator.',
  '- Do not widen RBAC to get past an error. Ask for the access instead.',
  '',
  '## Upgrades',
  '',
  '- Kubernetes minor upgrades happen once a quarter, one node at a time.',
  "- Read the provider's changelog before bumping a pinned version.",
  '- Upgrade one stack first and watch it for a day before the rest.',
  '',
  '## Git',
  '',
  '- Commit on a branch, never directly on main.',
  '- Stage files by name. Never run `git add -A` or `git add .`.',
  '- Write the subject as what changed and the body as why.',
  '- Run the tests before every push and wait for CI to pass.',
  '- Never force-push a branch someone else has pulled.',
  '',
  '## Stack reference',
  '',
  'One entry per stack: what it runs, where its secrets live and how to check it.',
  '',
];

const STACKS: Array<[string, string]> = [
  ['auth', 'single sign-on for every web service'],
  ['dns', 'the internal resolver and the public zone records'],
  ['ingress', 'the reverse proxy and its middlewares'],
  ['certs', 'the certificate issuer and its renewal jobs'],
  ['monitoring', 'metrics collection and the alert rules'],
  ['logging', 'log shipping and the log search'],
  ['dashboards', 'the dashboards for every stack'],
  ['backup', 'nightly database dumps and weekly volume snapshots'],
  ['object-store', 'S3-compatible storage for state, backups and uploads'],
  ['registry', 'the container registry and its garbage collection'],
  ['git', 'the git forge and its runners'],
  ['ci', 'the CI runners that plan and apply stacks'],
  ['postgres', 'the shared Postgres cluster'],
  ['redis', 'the shared cache'],
  ['queue', 'the message queue used by the importers'],
  ['search', 'full-text search for the wiki and the tickets'],
  ['mail', 'outbound mail relay and the inbound spam filter'],
  ['wiki', 'the team wiki'],
  ['chat', 'the team chat server'],
  ['tickets', 'the issue tracker'],
  ['vpn', 'remote access for people and for the backup site'],
  ['proxy', 'the outbound proxy for traffic that must not leave from home'],
  ['firewall', 'firewall rules between the VLANs'],
  ['status', 'the public status page'],
  ['uptime', 'external probes for every public endpoint'],
  ['secrets', 'the vault and its auto-unseal'],
  ['identity', 'users, groups and service accounts'],
  ['photos', 'the photo library'],
  ['media', 'the media server and its transcoders'],
  ['books', 'the ebook library'],
  ['files', 'file sync for every user'],
  ['calendar', 'shared calendars and contacts'],
  ['notes', 'the notes app'],
  ['rss', 'the feed reader'],
  ['recipes', 'the recipe manager'],
  ['budget', 'the household budget app'],
  ['home-automation', 'the home automation hub and its integrations'],
  ['printing', 'the print server'],
  ['webapp', 'the customer-facing web app and its API'],
  ['tripit', 'the trip planner and its importers'],
  ['scheduler', 'cron jobs that do not belong to a single stack'],
  ['analytics', 'privacy-friendly web analytics'],
  ['pastebin', 'a pastebin for logs and snippets'],
  ['bookmarks', 'the shared bookmark manager'],
  ['forms', 'the form builder'],
  ['url-shortener', 'short links for the team'],
  ['docs-site', 'the public documentation site'],
  ['blog', 'the public blog'],
  ['job-runner', 'one-off batch jobs'],
  ['cache', 'the HTTP cache in front of the object store'],
];

const OWNERS = ['platform', 'data', 'apps', 'home'];
const NOTES = [
  'Restarts are safe; clients retry.',
  'Upgrades need the maintenance window on Sunday morning.',
  'Scale it down before a node drain, or the drain stalls.',
  'Its database is part of the nightly backup.',
  'Keep it on the fast storage class; it is latency sensitive.',
  'The upstream chart is pinned; read its changelog before bumping.',
  'Two replicas, spread across nodes.',
];

function title(name: string): string {
  return name
    .split('-')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

function stackEntry(i: number): string[] {
  const [name, what] = STACKS[i % STACKS.length];
  const owner = OWNERS[i % OWNERS.length];
  return [
    `### ${name}`,
    '',
    `- Runs ${what}.`,
    `- Namespace \`${name}\`, owned by the ${owner} team. Applied by CI from \`stacks/${name}\`.`,
    `- Secrets: \`secret/${name}\` in the vault, read at deploy time.`,
    `- Check: \`kubectl -n ${name} get pods\`, then the ${title(name)} dashboard.`,
    `- ${NOTES[i % NOTES.length]}`,
    '',
  ];
}

const CLOSING = [
  '- Add an entry when you add a stack, in the same commit.',
  '- Remove the entry in the commit that removes the stack.',
  '- Keep each entry to five bullets. Longer notes go in the stack README.',
  '- Name dashboards rather than linking them; the links change.',
  '- Fix a wrong entry in the same change that fixes the stack.',
  '- Owners review their entries once a quarter.',
  '- The provisioner does not read this file. Only agents and people do.',
  '- When this list and the live cluster disagree, the cluster is right.',
  '- Entries are in the order the stacks were added.',
  '- A stack without an owner goes to the platform team.',
  '- Mark a stack that is being retired with the date it goes away.',
  '- Keep secret paths here, never the secrets themselves.',
  '- A new namespace needs an entry before its first deploy.',
  '- Ask in #ops when you are unsure who owns a stack.',
];

function hexOfLength(n: number, seed: number): string {
  let out = '';
  let x = seed >>> 0 || 1;
  while (out.length < n) {
    x ^= x << 13;
    x >>>= 0;
    x ^= x >>> 17;
    x ^= x << 5;
    x >>>= 0;
    out += x.toString(16).padStart(8, '0');
  }
  return out.slice(0, n);
}

/** Size of infra/AGENTS.md: Codex's 32,768-byte budget plus 3,291 bytes it cuts. */
export const INFRA_SIZE = 32768 + 3291;

function buildInfra(): string {
  const lines = [...INFRA_HEAD];
  const size = () => text(lines).length;
  // Whole stack entries while they fit, then closing bullets, then a
  // generated index comment whose length makes the file exactly INFRA_SIZE.
  for (let i = 0; size() + stackEntry(i).join('\n').length + 1 <= INFRA_SIZE - 600; i++) lines.push(...stackEntry(i));
  lines.push('## Keeping this file current', '');
  for (const b of CLOSING) {
    if (size() + b.length + 1 > INFRA_SIZE - 60) break;
    lines.push(b);
  }
  lines.push('');
  const prefix = '<!-- stack reference index: ';
  const suffix = ' -->';
  const room = INFRA_SIZE - size() - 1 - prefix.length - suffix.length;
  lines.push(prefix + hexOfLength(room, 0x5eed) + suffix);
  return text(lines);
}

export const INFRA_AGENTS = buildInfra();

export const TERRAFORM = text([
  '# Terraform conventions',
  '',
  'Referenced from infra/AGENTS.md. Read it before adding a stack or a module.',
  '',
  '## Naming',
  '',
  '- Resources: `<stack>-<purpose>`, lower case, hyphens.',
  '- Variables: snake_case, with a description and a type.',
  '- Outputs: only what another stack reads.',
  '',
  '## Providers',
  '',
  '- Pin every provider with `~>` to a minor version.',
  '- Configure providers in the stack, never inside a module.',
  '',
  '## State',
  '',
  '- One state key per stack: `stacks/<name>/terraform.tfstate`.',
  '- Move resources between stacks with `moved` blocks, not by editing state.',
  '',
  '## Modules',
  '',
  '- A module has a README with its inputs, outputs and one example.',
  '- Keep modules small. A module that grows past 300 lines should be split.',
]);

export const NETWORKING = text([
  '# Networking notes',
  '',
  '- Three VLANs: `lan` for people, `iot` for devices, `srv` for the cluster.',
  '- The firewall allows `lan` to reach `srv` on 443 only. `iot` reaches nothing but DNS and NTP.',
  "- The cluster's ingress has one public address. Everything else stays private.",
  '- Changes to firewall rules go through the `firewall` stack and need a second review.',
]);

export const SEV_TRIAGE = text([
  '---',
  'name: sev-triage',
  'description: Triage a live incident. Collects alerts, logs and recent deploys for a stack and proposes a first action. Read-only.',
  'tools: Bash, Read, Grep',
  '---',
  '',
  'You triage incidents for the home cluster. You never change anything.',
  '',
  '1. List the firing alerts for the stack and when each started.',
  '2. Pull the last hour of error logs from the log search.',
  '3. List the deploys and applies of the last 24 hours for the stack.',
  '4. Check the network path if the alert is about reachability: docs/agents/networking.md.',
  '5. Propose one first action and say what it will tell us.',
  '',
  'Report in five lines or fewer. Link every alert and log query you used.',
]);

export const TF_PLAN = text([
  '---',
  'description: Run terragrunt plan for one stack and summarise the changes',
  'argument-hint: <stack>',
  '---',
  '',
  'Run `terragrunt plan` in `stacks/$ARGUMENTS` and summarise the result:',
  '',
  '- resources to add, change and destroy, with counts;',
  '- any destroy of a stateful resource, called out first;',
  '- anything that differs from the conventions in docs/agents/terraform.md.',
  '',
  'Do not apply.',
]);

export const WEBAPP_AGENTS = text([
  '# webapp',
  '',
  'The customer-facing web app: a Svelte frontend and a Go API. Pull requests follow these rules:',
  '',
  '- Keep each pull request to one change that someone can review in ten minutes.',
  '- Put before and after screenshots in the description of every UI change.',
  '- Link the issue the pull request closes, or say why there is none.',
  '- Squash when you merge, and delete the branch afterwards.',
  '- Never merge your own pull request without a second review.',
  '',
  '## Layout',
  '',
  '- `api/` is the Go service. `make test` runs its tests against a throwaway database.',
  '- `frontend/` is the Svelte app. It has its own AGENTS.md.',
  '- `deploy/` holds the container build. CI builds and pushes the image on every merge.',
  '',
  '## Releases',
  '',
  '- Main deploys to production after CI passes. There is no staging.',
  '- Feature flags live in `api/flags.go`. Remove a flag within a month of turning it on for everyone.',
  '- Write user-facing release notes with the `/release-notes` command.',
]);

export const WEBAPP_FRONTEND = text([
  '# webapp frontend',
  '',
  'Rules for the Svelte app. The repository rules in ../AGENTS.md apply too.',
  '',
  '- Svelte 5 with runes. No new stores; use `$state` and `$derived`.',
  '- Components live in `src/lib/components/`, one component per file.',
  '- Every interactive element is reachable by keyboard and has a visible focus ring.',
  '- Check each page at 390 px wide as well as on a desktop screen.',
  '- Run `npm run check` and `npm test` before you push.',
]);

export const RELEASE_NOTES = text([
  '---',
  'description: Draft user-facing release notes from the commits since the last tag',
  '---',
  '',
  'List the commits since the last tag with `git log --oneline $(git describe --tags --abbrev=0)..HEAD`.',
  'Group them into New, Improved and Fixed. Leave out refactors and CI changes.',
  'Write one plain sentence per item, in the words a customer would use.',
]);

export const TRIPIT_AGENTS = text([
  '# tripit',
  '',
  'A trip planner: it imports bookings from email and shows them on a map and a timeline.',
  '',
  '## Pull requests',
  '',
  '- Keep each pull request to one change that someone can review in ten minutes.',
  '- Put before and after screenshots in the description of every UI change.',
  '- Link the issue the pull request closes, or say why there is none.',
  '- Squash when you merge, and delete the branch afterwards.',
  '- Never merge your own pull request without a second review.',
  '',
  '## Stack',
  '',
  '- SvelteKit on the frontend, Python for the importers.',
  '- The geocoder is rate-limited; cache every lookup in `data/geocode.sqlite`.',
  '- Tests: `make test` for the importers, `npm test` for the frontend.',
  '',
  '## Data',
  '',
  '- Real booking emails never go into fixtures. Use the samples in `fixtures/synthetic/`.',
]);

export const CODE_REVIEWER = text([
  '---',
  'name: code-reviewer',
  'description: Review a diff for correctness bugs before it lands. Reports findings with file and line.',
  'tools: Read, Grep, Bash',
  '---',
  '',
  'Review the diff you are given. Look for bugs first: wrong conditions, missing error',
  'handling, races, off-by-one errors, and tests that do not test what they claim.',
  '',
  "Report each finding with its file, line, what goes wrong and a suggested fix.",
  "Skip style comments unless the repository's linter would reject the code.",
]);

export const PUBLISH_PAGE = text([
  '---',
  'name: publish-page',
  "description: Publish a finished design doc as a page on the team's own site. Use for plans, specs and designs, not for drafts.",
  '---',
  '',
  '# Publish a page',
  '',
  "1. Render the markdown with the site's renderer: `pages render <doc.md>`.",
  '2. Open the rendered page locally and look at every diagram and table.',
  '3. Copy the page into the site repository and push it.',
  '4. Hand the URL back to the user.',
  '',
  'Re-publish when the doc changes status: draft, approved, done.',
]);

export const TDD = text([
  '---',
  'name: tdd',
  'description: Test-driven development. Use when building a feature or fixing a bug test-first.',
  '---',
  '',
  '# Test-driven development',
  '',
  'Work in small red-green-refactor loops.',
  '',
  '1. Write one failing test that describes the next piece of behaviour.',
  '2. Run it and watch it fail for the reason you expect.',
  '3. Write the least code that makes it pass.',
  '4. Refactor with the tests green.',
  '',
  'Prefer tests through public interfaces. A test that breaks on every refactor is testing the wrong thing.',
]);

export const DOCS_LOOKUP = text([
  '---',
  'name: docs-lookup',
  'description: Look up current library documentation before writing code against an API.',
  '---',
  '',
  '# Docs lookup',
  '',
  'Before you use a library API you have not used in this session, fetch its current',
  'documentation and check the signature and the version it applies to.',
]);
