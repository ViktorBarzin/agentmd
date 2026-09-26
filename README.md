# agentmd

See, check and edit the markdown files your coding agents read.

A coding agent reads instructions from many places: an org policy, a user file,
one `AGENTS.md` or `CLAUDE.md` per repository and sometimes one per folder,
skills, subagents, commands, and the docs those point to. agentmd finds them,
shows how they refer to each other, reports where they repeat or disagree, and
lets you edit them in the browser. It knows Claude Code, Codex and the
[AGENTS.md](https://agents.md) standard.

- **Files**: every agent file, grouped by org, user, skills and repository, with
  who can edit it and where edits belong (a symlink's target, a built file's
  parts, a root-owned copy's origin).
- **Graph**: which files link to, import, are built from or mention which, and
  for any folder, the order in which a harness loads them.
- **Findings**: duplicates, contradictions, broken references, files over a
  harness's size limit, and files a harness skips. A finding between files that
  load in the same session is a problem; the rest are hints.
- **Editor**: edit in place, see the `git diff`, commit, and fast-forward push.

## Install

Download the binary for your platform from the
[releases](https://github.com/ViktorBarzin/agentmd/releases), unpack it and put
`agentmd` on your `PATH`. Or build it: `make build` (needs Go 1.22+ and Node 20+).

## Use

```sh
agentmd                 # serve the UI on http://127.0.0.1:7390
agentmd --open          # and open it
agentmd check           # print findings; exit 1 when there are problems
agentmd check --probe   # probe Claude Code and Codex first
agentmd check --json    # the whole model, for agents and CI
agentmd probe --harness claude --dir ~/code/app   # what Claude Code loads there
```

agentmd searches `~/code` for repositories by default. Pass `--root <dir>` (more
than once if needed) or set `roots` in the config.

### How agentmd knows what loads

Loading rules change between harness releases and depend on things outside the
files, such as feature flags and which folders you have trusted. So agentmd asks
the harnesses themselves ([ADR-0001](docs/adr/0001-runtime-context-comes-from-probing-the-harness.md)):

- **Claude Code**: agentmd runs `claude -p` in the folder with its API address
  pointed at a stand-in inside agentmd. The stand-in records the request and
  answers with an error, so no model is called and no tokens are spent. The
  probe turns hooks, MCP servers and session history off, pins the API
  settings so a repository's own settings cannot redirect it or run a key
  helper, and uses a throwaway config folder so nothing is written to your
  Claude state.
- **Codex**: agentmd runs `codex debug prompt-input` in the folder with every
  MCP server switched off.
- **AGENTS.md**: every `AGENTS.md` from the repository root down to the folder.

Probes run when you ask (the UI probes everything on first open) and are cached
until something that could change the result changes. Hooks set by an
organisation's managed settings still run during a Claude probe, since no user
setting can turn them off; agentmd removes the tmux variables from the probe's
environment.

### Analysis and fixes

Contradictions and reworded duplicates need a language model. agentmd runs the
agent CLI you already use (`claude -p` or `codex exec`), only when you click
Analyse, and caches the answer by file content
([ADR-0003](docs/adr/0003-analysis-runs-through-the-installed-agent-cli.md)).
With Claude, your own instruction files are not loaded into the analysis, and
the files go in a system-prompt file rather than the prompt. Every finding's
quoted text is checked against the file; a fix is a set of exact replacements
you review as a diff before applying.

## Access

`agentmd serve` listens on `127.0.0.1:7390` and asks for no token
([ADR-0002](docs/adr/0002-localhost-without-a-token.md)). It answers only a
loopback `Host` header, requires `X-Agentmd-Request: 1` on every API call,
sends no CORS headers, and on Linux refuses connections from other OS users.

To put it behind an auth proxy, give it a shared secret; any address other than
loopback requires one:

```sh
agentmd serve --listen 0.0.0.0:7390 \
  --proxy-secret-file /run/secrets/agentmd \
  --identity-header X-Forwarded-User --allow-identity alice
```

Every request must then carry the secret in `X-Agentmd-Proxy-Secret` and an
allowed identity in the identity header. Identities compare without regard to
case, and an allowed identity without `@` also matches that name at any
domain, since proxies send the username or the email depending on the flow
(`--allow-identity alice` lets in `alice@example.com`). A refused identity is
logged and named in the 403 answer. The process always acts as the OS user
who started it. `serve` also accepts a listening socket from systemd, so a
service manager can hold the port across restarts.

## Configuration

Optional: `~/.config/agentmd/config.toml`, merged after
`/etc/agentmd/config.toml`.

```toml
roots = ["~/code"]
exclude = ["**/fixtures/**"]            # folders the walk skips
skip_contexts = ["~/code/templates/*"]  # folders where no session starts

[budget]
file_bytes = 32768                      # your own limit per instruction file

[analysis]
cli = "claude"                          # or "codex"

[[origin]]                              # a root-owned copy and where to edit it
path = "/etc/claude-code/managed-settings.json"
field = "claudeMd"
source = ["~/code/policy/managed-settings.json"]
note = "Installed by the provisioner"

[[build]]                               # a built file and its parts
output = "~/.agents/AGENTS.md"
parts = ["~/.agents/core.md", "~/.agents/personal.md"]
command = "agents-sync"

[[harness]]                             # an extra harness with static rules
name = "pi"
user_files = ["~/.pi/agent/AGENTS.md"]
project_files = ["AGENTS.md"]
skill_dirs = ["~/.agents/skills"]
```

A built file is also recognised by a header comment such as
`<!-- Built by agents-sync from ~/.agents/{core,personal}.md; edit those. -->`.
Files that chezmoi manages, skills installed by the skills CLI and plugin files
are recognised on their own.

Results live in `~/.cache/agentmd`. agentmd runs no background work: it scans
when it starts, when you press rescan, and when its tab regains focus.

## Develop

```sh
make test                     # go vet, go test, vitest
make check                    # svelte-check
make build && (cd web && npm run e2e)   # Playwright against the built binary
cd web && VITE_MOCK=1 npm run dev   # the UI against a built-in mock
cd web && npm run dev         # the UI against agentmd serve on :7390
```

The vocabulary is in [CONTEXT.md](CONTEXT.md), the HTTP API in
[docs/api.md](docs/api.md) and the design in
[docs/plans/2026-09-26-agentmd-v1.md](docs/plans/2026-09-26-agentmd-v1.md).

## Licence

MIT
