# Runtime context comes from probing the harness

agentmd learns what Claude Code and Codex load by starting them and recording
what they would send, rather than by re-implementing their loading rules. The
rules change between releases and carry behaviour nobody documents: Claude Code
reads `AGENTS.md` only when no `CLAUDE.md` exists anywhere up the tree and only
behind a feature flag, and Codex cuts project docs at a shared 32,768-byte budget
in the middle of a line. A probe reports the version that is installed, so a
finding about co-loading or truncation reflects what the agent actually saw.

## How each harness is probed

- **Claude Code**: `claude -p` runs with `ANTHROPIC_BASE_URL` pointing at a
  stand-in API inside agentmd. The stand-in records the first request and answers
  with an error, so no model is called and no tokens are spent. The request lists
  each instruction file by path in load order, then the skills and subagents on
  offer. The probe passes `disableAllHooks`, `--no-session-persistence` and
  `--strict-mcp-config`, so the user's hooks, session history and MCP servers stay
  untouched.
- **Codex**: `codex debug prompt-input` prints the model-visible input as JSON.
  Codex joins instruction files without per-file markers, so agentmd attributes
  the text to candidate files by matching their content, which also shows where a
  file was cut.
- **The AGENTS.md standard** has no single program to probe, so it uses a
  static context: every `AGENTS.md` from the repository root down to the
  directory.

## Consequences

- A static context is also the fallback when a harness is not installed, for
  example in CI. The UI and the JSON output label each context as probed or
  static.
- Hooks set by an org policy still run during a Claude probe, because a user
  setting cannot turn them off. agentmd removes terminal-multiplexer variables
  from the probe's environment, and the README says so.
- Probes run on demand and are cached until a file that could change the result
  changes.
