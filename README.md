# agentmd

See, check and edit the markdown files your coding agents read.

A coding agent reads instructions from many places: an org policy, a user file,
one `AGENTS.md` or `CLAUDE.md` per repository, skills, subagents, commands and
the docs they point to. agentmd finds them, shows how they refer to each other,
reports duplicates, contradictions, broken references and files over a
harness's size limit, and lets you edit them in the browser. It knows Claude
Code, Codex and the AGENTS.md standard.

Status: in development towards v0.1.0. The design is in
[docs/plans/2026-09-26-agentmd-v1.md](docs/plans/2026-09-26-agentmd-v1.md), the
vocabulary in [CONTEXT.md](CONTEXT.md) and the decisions in [docs/adr](docs/adr).

## Licence

MIT
