# Runtime context comes from probing the harness

agentmd learns what Claude Code and Codex load by starting them and recording
what they would send, rather than by re-implementing their loading rules. The
rules change between releases and depend on state outside the files: Claude
Code reads `AGENTS.md` only when no `CLAUDE.md` exists up the tree and only
behind a remotely set feature flag, drops `@imports` in folders that are not
trusted, and Codex cuts project docs at a shared 32,768-byte budget in the
middle of a line. A probe reports what the installed version does on this
machine, so a finding about co-loading or truncation reflects what the agent
actually saw. Where a directory has not been probed, agentmd says so instead of
guessing.

## How each harness is probed

- **Claude Code**: `claude -p` runs with its API address pinned to a stand-in
  inside agentmd. The stand-in records the request and answers with an error, so
  no model is called and no tokens are spent. The request lists each
  instruction file by path in load order, then the skills and subagents on
  offer.
- **Codex**: `codex debug prompt-input` prints the model-visible input as JSON.
  Codex joins instruction files without per-file markers, so agentmd attributes
  the text to candidate files by matching their contents in order, which also
  shows where a file was cut.
- **The AGENTS.md standard** has no single program to probe, so it uses a
  static context: every `AGENTS.md` from the repository root down to the
  directory.

## A probe must not act on the owner's behalf

A probe runs in folders the owner did not write, such as a freshly cloned
repository, and `claude -p` applies that folder's settings without asking. In a
test, a repository's settings file ran its own key-helper command and sent the
request to its own server. So every Claude probe:

- pins the API address, key, providers and every command-running helper in its
  own `--settings`, which outranks user and project settings, and turns hooks
  off;
- runs with `--strict-mcp-config` and `--no-session-persistence`, stdin from
  `/dev/null`, and no terminal-multiplexer variables;
- uses a throwaway `CLAUDE_CONFIG_DIR` that links the owner's `~/.claude` and
  copies `~/.claude.json`, so trust decisions and feature flags match and
  nothing is written to the owner's state;
- refuses to run when the org policy routes Claude to another endpoint, and
  fails when the stand-in sees no request.

Codex probes switch off each configured MCP server by name.

## Consequences

- Hooks set by an org policy still run during a Claude probe, because no user
  setting can turn them off.
- Without an installed harness, for example in CI, agentmd reports Claude Code
  and Codex contexts as not probed, and co-loading findings come from the
  AGENTS.md contexts only.
- The list of settings to pin must follow Claude Code releases. A live test with
  a hostile repository catches a new setting that slips through.
- Probes run on demand and are cached until anything that could change the
  result changes.
