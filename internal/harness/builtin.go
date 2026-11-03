package harness

import (
	"strings"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// Builtin is what a harness ships inside its own binary: skills with no
// SKILL.md on disk, and slash commands. A probe's skill list may leave some
// of them out, so a mention of one is never a broken reference.
type Builtin struct {
	Skills   []string
	Commands []string
}

// Builtins lists each known harness's built-in skills and slash commands.
var Builtins = map[string]Builtin{
	model.HarnessClaude: {
		Skills: []string{
			"claude-api", "code-review", "dataviz", "fewer-permission-prompts", "init", "keybindings-help",
			"loop", "plugin-authoring", "run", "schedule", "security-review", "simplify", "update-config",
			"workflow-authoring",
		},
		Commands: []string{
			"add-dir", "agents", "bashes", "btw", "bug", "clear", "color", "compact", "config", "context",
			"copy", "cost", "doctor", "effort", "exit", "export", "extra-usage", "fast", "feedback", "help",
			"hooks", "ide", "insights", "install-github-app", "login", "logout", "mcp", "memory", "model",
			"output-style", "permissions", "plan", "plugin", "pr-comments", "privacy-settings", "release-notes",
			"remote-control", "rename", "resume", "review", "rewind", "sandbox", "skills", "stats", "status",
			"statusline", "tasks", "terminal-setup", "theme", "todos", "upgrade", "usage", "vim", "workflows",
		},
	},
	model.HarnessCodex: {
		Commands: []string{
			"approvals", "compact", "diff", "exit", "feedback", "init", "logout", "mcp", "mention", "model",
			"new", "prompts", "quit", "resume", "review", "status", "undo",
		},
	},
}

// Names is a set of skill and slash command names that harnesses offer
// without a file.
type Names struct {
	skills, commands map[string]bool
}

// Offered collects the built-in names of every known harness and the
// bundled skills of each configured one.
func Offered(static []config.StaticHarness) Names {
	n := Names{skills: map[string]bool{}, commands: map[string]bool{}}
	for _, b := range Builtins {
		for _, s := range b.Skills {
			n.skills[s] = true
		}
		for _, c := range b.Commands {
			n.commands[c] = true
		}
	}
	for _, h := range static {
		for _, s := range h.BundledSkills {
			n.skills[s] = true
		}
	}
	return n
}

// Has reports whether a mention's text names one of them. A text with a
// leading slash may name a skill or a command; a bare name only a skill.
func (n Names) Has(text string) bool {
	if name, ok := strings.CutPrefix(text, "/"); ok {
		return n.skills[name] || n.commands[name]
	}
	return n.skills[text]
}
