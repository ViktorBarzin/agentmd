# Analysis runs through the installed agent CLI

Contradictions and reworded duplicates need a language model, and agentmd gets
one by running the agent CLI the owner already uses (`claude -p` or
`codex exec`) instead of calling a model API itself. The owner has already
signed in and chosen a model there, so agentmd holds no API key, adds no
provider SDK, and spends only from the plan the owner already has. It runs only
when asked, and results are cached by the content of the files analysed.

## Consequences

- Output quality and cost follow the owner's CLI settings. agentmd asks for JSON
  through each CLI's schema option and discards any finding whose quoted text is
  not in the file at the lines it names.
- The CLI flags agentmd relies on can change between releases. The analysis
  runner checks the CLI version and reports a clear error when a flag is
  rejected.
- Without an installed CLI, the deterministic findings still work and the
  analysis buttons explain what is missing.
