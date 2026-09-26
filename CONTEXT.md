# agentmd

agentmd shows the markdown files that coding agents read, how they refer to each
other, and where they repeat or contradict each other, so their owner can clean
them up and edit them in one place.

## Files

**Agent file**:
A markdown document an agent reads: an instruction file, a skill, a subagent, a
command, or a doc that one of those points to.
_Avoid_: rules file, prompt file, md file

**Instruction file**:
An agent file a harness loads at session start without being asked, such as
`AGENTS.md`, `CLAUDE.md`, a rules file or an org policy.
_Avoid_: memory file, context file

**Org policy**:
Instructions an administrator installs for every user of a machine, outside any
user's control.
_Avoid_: managed memory, system prompt

**Embedded file**:
An agent file whose text lives in one field of a settings file, such as the org
policy inside a JSON settings file.

**Referenced doc**:
A file an agent file points to by path. An agent reads it only when it follows
the pointer.

**Built file**:
An agent file generated from other files, its **parts**. Edits go to the parts.
_Avoid_: compiled file, generated file

**Managed file**:
An agent file some other tool maintains, such as a dotfile manager, a skill
installer, a plugin system or a provisioner. It has an **origin**: where edits
belong so they survive the tool's next run.

## Loading

**Harness**:
A program that runs an agent and loads agent files for it: Claude Code, Codex,
or any tool following the AGENTS.md standard.
_Avoid_: client, agent, tool

**Runtime context**:
The ordered list of agent files a harness loads when a session starts in one
directory, with the skills and subagents it offers there.
_Avoid_: effective context, prompt

**Probe**:
Starting a harness in a directory so that its runtime context can be recorded,
without reaching a model or running the user's hooks.

**Static context**:
A runtime context worked out from a harness's documented loading rules instead
of a probe.

**Co-loading**:
Two agent files that appear in the same runtime context.

## Relations

**Reference**:
A directed relation from one agent file to another. It has one of four kinds:
symlink, build join, text mention, load order.
_Avoid_: link, dependency, edge

**Build join**:
A reference from a file to a part it is assembled from, either by a build step
or by the harness at load time (an import).

**Text mention**:
A reference made by writing the other file's path or a link to it in the text.

**Load order**:
A reference from one file to the next one in a runtime context.

## Findings

**Finding**:
Something in the agent files worth cleaning up: a duplicate, a reworded
duplicate, a contradiction, a dangling reference, a file over budget, or a file
no harness loads.
_Avoid_: issue, warning, lint

**Problem**:
A finding between files that co-load, or inside one loaded file, so an agent
sees both sides in the same session.

**Hint**:
A finding between files that never co-load, such as the same paragraph in two
unrelated repos.

**Budget**:
The size limit a harness or the owner sets for an instruction file or a runtime
context.

**Fix proposal**:
An edit that an agent suggests for a finding. The owner reviews it as a diff and
applies or discards it.

## People

**Owner**:
The OS user agentmd runs as. agentmd shows and edits that user's agent files
with that user's permissions.
_Avoid_: user (ambiguous with the agent's user), account

**Discovery root**:
A directory tree agentmd searches for repositories and their agent files.
