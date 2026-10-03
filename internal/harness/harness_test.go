package harness_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/harness"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/probe"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

func setup(t *testing.T, tr *testutil.Tree) (harness.Env, *discover.Result) {
	t.Helper()
	codex := discover.CodexSettings{MaxBytes: 100}
	env := harness.Env{Home: tr.Home, Etc: tr.Etc, CodexHome: filepath.Join(tr.Home, ".codex"),
		ClaudeConfigDir: filepath.Join(tr.Home, ".claude"), GOOS: "linux", Roots: []string{tr.Code}, Codex: codex}
	res, err := discover.Scan(discover.Env{Home: tr.Home, Etc: tr.Etc, CodexHome: env.CodexHome, GOOS: "linux",
		Roots: env.Roots, Codex: codex})
	if err != nil {
		t.Fatal(err)
	}
	return env, res
}

func ids(es []model.ContextEntry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.FileID)
	}
	return out
}

func TestAgentsMDChain(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	root := tr.File(filepath.Join(app, "AGENTS.md"), "# root\n")
	web := tr.File(filepath.Join(app, "web/AGENTS.md"), "# web\n")
	tr.File(filepath.Join(tr.Code, "AGENTS.md"), "# above the repo\n")
	env, res := setup(t, tr)
	c, ok := harness.AgentsMD(env, res, filepath.Join(app, "web"))
	if !ok || !reflect.DeepEqual(ids(c.Entries), []string{root, web}) || c.Source != "static" {
		t.Errorf("context = %+v", c)
	}
	if _, ok := harness.AgentsMD(env, res, tr.Home); ok {
		t.Error("no AGENTS.md context outside a repository")
	}
}

func TestFromClaude(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.File(filepath.Join(app, "AGENTS.md"), "# app\n")
	tr.File(filepath.Join(tr.Code, "CLAUDE.md"), "# code\n")
	tr.File(filepath.Join(tr.Etc, "claude-code/managed-settings.json"), `{"claudeMd":"# org\n"}`)
	user := tr.File(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "# user\n")
	tr.File(filepath.Join(tr.Home, ".claude/skills/tdd/SKILL.md"), "---\nname: tdd\n---\n")
	tr.File(filepath.Join(app, ".claude/agents/reviewer.md"), "---\nname: reviewer\n---\n")
	// Claude Code lists custom commands with the skills; a skill wins a name
	// both share.
	deploy := tr.File(filepath.Join(app, ".claude/commands/deploy.md"), "Deploy the app.\n")
	tr.File(filepath.Join(app, ".claude/commands/tdd.md"), "A command named like a skill.\n")
	env, res := setup(t, tr)
	scratch := filepath.Join(tr.Root, "cache/claude-123")
	cap := &probe.ClaudeResult{ScratchDir: scratch, ClaudeCapture: &probe.ClaudeCapture{
		Files: []probe.Loaded{
			{Label: "<managed-settings>", Content: "# org"},
			{Label: scratch + "/CLAUDE.md", Content: "# user\n"},
			{Label: filepath.Join(tr.Code, "CLAUDE.md"), Content: "# code"},
		},
		Skills:    []probe.Item{{Name: "tdd"}, {Name: "builtin-skill"}, {Name: "deploy"}},
		Subagents: []probe.Item{{Name: "reviewer"}, {Name: "general-purpose"}},
	}}
	c := harness.FromClaude(env, res, app, "2.1.283 (Claude Code)", cap, time.Now())
	want := []string{filepath.Join(tr.Etc, "claude-code/managed-settings.json") + "#claudeMd", user, filepath.Join(tr.Code, "CLAUDE.md")}
	if !reflect.DeepEqual(ids(c.Entries), want) {
		t.Errorf("entries = %v, want %v", ids(c.Entries), want)
	}
	if c.Entries[1].Label != user {
		t.Errorf("the throwaway config path is not shown: label %q", c.Entries[1].Label)
	}
	if c.Bytes != len("# org")+len("# user\n")+len("# code") || c.Source != "probe" || c.ID != "claude:"+app {
		t.Errorf("bytes %d source %q id %q", c.Bytes, c.Source, c.ID)
	}
	if c.Skills[0].FileID != filepath.Join(tr.Home, ".claude/skills/tdd/SKILL.md") || c.Skills[1].FileID != "" || c.Skills[2].FileID != deploy {
		t.Errorf("skills = %+v", c.Skills)
	}
	if c.Subagents[0].FileID != filepath.Join(app, ".claude/agents/reviewer.md") || c.Subagents[1].FileID != "" {
		t.Errorf("subagents = %+v", c.Subagents)
	}
	if len(c.Skipped) != 1 || c.Skipped[0].FileID != filepath.Join(app, "AGENTS.md") || !strings.Contains(c.Skipped[0].Reason, "further up") {
		t.Errorf("skipped = %+v", c.Skipped)
	}
}

func TestFromCodexAttributesAndFindsTheCut(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	user := tr.File(filepath.Join(tr.Home, ".codex/AGENTS.md"), "\n# user rules\n\n")
	root := tr.File(filepath.Join(app, "AGENTS.md"), "# root doc\nline two\n")
	mid := tr.File(filepath.Join(app, "pkg/AGENTS.md"), "# pkg doc that is long enough to be cut\n")
	leaf := tr.File(filepath.Join(app, "pkg/deep/AGENTS.md"), "# never reached\n")
	tr.File(filepath.Join(tr.Etc, "codex/requirements.toml"), "additional_developer_instructions = \"# org\"\n")
	env, res := setup(t, tr)
	// Codex trims the user file, joins project docs with a blank line and cut
	// the second doc part way through.
	text := "# user rules" + "\n\n--- project-doc ---\n\n" + "# root doc\nline two\n" + "\n\n" + "# pkg doc that"
	cap := &probe.CodexCapture{HasBlock: true, Instructions: text, OrgPolicy: "# org",
		Skills: []probe.Item{{Name: "tdd", Path: filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md")}}}
	tr.File(filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md"), "---\nname: tdd\n---\n")
	c := harness.FromCodex(env, res, filepath.Join(app, "pkg/deep"), "codex-cli 0.157.1", cap, time.Now())
	want := []string{user, root, mid, filepath.Join(tr.Etc, "codex/requirements.toml") + "#additional_developer_instructions"}
	if !reflect.DeepEqual(ids(c.Entries), want) {
		t.Fatalf("entries = %v, want %v", ids(c.Entries), want)
	}
	cut := c.Entries[2]
	if !cut.Truncated || cut.Bytes != len("# pkg doc that") || cut.LostBytes != len("# pkg doc that is long enough to be cut\n")-len("# pkg doc that") {
		t.Errorf("cut entry = %+v", cut)
	}
	if len(c.Skipped) != 1 || c.Skipped[0].FileID != leaf || !strings.Contains(c.Skipped[0].Reason, "budget") {
		t.Errorf("skipped = %+v", c.Skipped)
	}
	if len(c.Skills) != 1 || c.Skills[0].FileID != filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md") {
		t.Errorf("skills = %+v", c.Skills)
	}
}

func TestFromCodexDocContainingTheSeparator(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	doc := tr.File(filepath.Join(app, "AGENTS.md"), "# doc\n\n--- project-doc ---\n\nstill the same doc\n")
	env, res := setup(t, tr)
	cap := &probe.CodexCapture{HasBlock: true, Instructions: "# doc\n\n--- project-doc ---\n\nstill the same doc"}
	c := harness.FromCodex(env, res, app, "v", cap, time.Now())
	if !reflect.DeepEqual(ids(c.Entries), []string{doc}) || c.Entries[0].Truncated {
		t.Errorf("a doc that contains the marker is one entry: %+v", c.Entries)
	}
}

func TestFingerprint(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	web := tr.Dir(filepath.Join(app, "web"))
	tr.File(filepath.Join(app, "AGENTS.md"), "# app\n")
	skill := tr.File(filepath.Join(tr.Home, ".claude/skills/tdd/SKILL.md"), "---\nname: tdd\n---\n")
	global := tr.File(filepath.Join(tr.Home, ".claude.json"), `{"projects":{}}`)
	fp := func() string {
		env, res := setup(t, tr)
		return harness.Fingerprint(env, res, harness.NewStats(), "claude", "1.0", web, nil, harness.LoadClaudeState(global))
	}
	base := fp()
	if fp() != base {
		t.Fatal("fingerprint is stable when nothing changes")
	}
	touch := func(p string) {
		future := time.Now().Add(time.Hour)
		if err := os.Chtimes(p, future, future); err != nil {
			t.Fatal(err)
		}
	}
	tr.File(filepath.Join(app, "CLAUDE.md"), "# new file up the tree\n")
	after := fp()
	if after == base {
		t.Error("a new CLAUDE.md up the tree changes the fingerprint")
	}
	os.WriteFile(skill, []byte("---\nname: tdd\n---\nedited\n"), 0o644)
	touch(skill)
	if fp() == after {
		t.Error("editing a SKILL.md changes the fingerprint")
	}
	cur := fp()
	os.WriteFile(global, []byte(`{"projects":{"`+web+`":{"hasTrustDialogAccepted":true,"lastCost":1}}}`), 0o600)
	trusted := fp()
	if trusted == cur {
		t.Error("trusting the folder changes the fingerprint")
	}
	os.WriteFile(global, []byte(`{"projects":{"`+web+`":{"hasTrustDialogAccepted":true,"lastCost":2}}}`), 0o600)
	if fp() != trusted {
		t.Error("session statistics in the state file do not change the fingerprint")
	}
	tr.File(filepath.Join(tr.Code, "other/AGENTS.md"), "# unrelated repo\n")
	if fp() != trusted {
		t.Error("a file in an unrelated folder does not change the fingerprint")
	}
}

// Updaters and harnesses rewrite files without changing them (Codex unpacks
// its bundled skills, a skills updater copies a set over itself), so the
// fingerprint follows content, not modification times.
func TestFingerprintFollowsContentNotTimestamps(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	doc := tr.File(filepath.Join(app, "AGENTS.md"), "# app\n")
	skill := tr.File(filepath.Join(tr.Home, ".codex/skills/.system/imagegen/SKILL.md"), "---\nname: imagegen\n---\n")
	settings := tr.File(filepath.Join(tr.Home, ".claude/settings.json"), `{"model":"opus"}`)
	fp := func() string {
		env, res := setup(t, tr)
		return harness.Fingerprint(env, res, harness.NewStats(), "claude", "1.0", app, []string{doc}, harness.ClaudeState{})
	}
	base := fp()
	future := time.Now().Add(time.Hour)
	for _, p := range []string{doc, skill, settings} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(p, data, 0o644)
		os.Chtimes(p, future, future)
	}
	if fp() != base {
		t.Error("rewriting files with the same content keeps the fingerprint")
	}
	os.WriteFile(skill, []byte("---\nname: imagegen\n---\nnew\n"), 0o644)
	os.Chtimes(skill, future, future)
	if fp() == base {
		t.Error("new content changes the fingerprint")
	}
}

func TestClaudeStateIgnoresRefreshTimestamps(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".claude.json")
	write := func(s string) harness.ClaudeState {
		os.WriteFile(p, []byte(s), 0o600)
		return harness.LoadClaudeState(p)
	}
	a := write(`{"cachedGrowthBookFeatures":{"tengu_agents_md_mod":true,"tengu_import":{"on":1},"tengu_banner_color":"red"},"cachedGrowthBookFeaturesAt":1000,"numStartups":5}`)
	b := write(`{"numStartups":6,"cachedGrowthBookFeaturesAt":2000,"cachedGrowthBookFeatures":{"tengu_import":{"on":1},"tengu_banner_color":"blue","tengu_agents_md_mod":true}}`)
	if a.Flags != b.Flags {
		t.Error("a refresh that moves the timestamp, reorders keys or changes an unrelated flag keeps the same hash")
	}
	c := write(`{"cachedGrowthBookFeatures":{"tengu_agents_md_mod":false,"tengu_import":{"on":1},"tengu_banner_color":"blue"}}`)
	if c.Flags == a.Flags {
		t.Error("flipping a flag that changes instruction loading changes the hash")
	}
}
