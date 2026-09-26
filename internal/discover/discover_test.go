package discover_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

func env(tr *testutil.Tree) discover.Env {
	return discover.Env{
		Home:      tr.Home,
		Etc:       tr.Etc,
		CodexHome: filepath.Join(tr.Home, ".codex"),
		GOOS:      "linux",
		Roots:     []string{tr.Code},
		Codex:     discover.CodexSettings{MaxBytes: discover.DefaultCodexMaxBytes},
	}
}

func scan(t *testing.T, e discover.Env) *discover.Result {
	t.Helper()
	res, err := discover.Scan(e)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func must(t *testing.T, res *discover.Result, id string) *model.File {
	t.Helper()
	f, ok := res.Files[id]
	if !ok {
		ids := make([]string, 0, len(res.Files))
		for k := range res.Files {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		t.Fatalf("no file %s; have %v", id, ids)
	}
	return f
}

func TestUserInstructionLinksReachTheHub(t *testing.T) {
	tr := testutil.New(t)
	hub := tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "# Hub\nBe brief.\n")
	tr.Link(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "../.agents/AGENTS.md")
	tr.Link(filepath.Join(tr.Home, ".codex/AGENTS.md"), hub)

	res := scan(t, env(tr))
	link := must(t, res, filepath.Join(tr.Home, ".claude/CLAUDE.md"))
	if !link.IsLink || link.LinkTarget != hub || link.RealPath != hub {
		t.Errorf("link = %+v, want a link to %s", link, hub)
	}
	if link.Hash == "" || link.Lines != 2 {
		t.Errorf("link should carry the target's content facts, got hash %q lines %d", link.Hash, link.Lines)
	}
	h := must(t, res, hub)
	if h.IsLink {
		t.Error("the hub itself is not a link")
	}
	if !reflect.DeepEqual(h.Harnesses, []string{"claude", "codex"}) && !reflect.DeepEqual(h.Harnesses, []string{"codex", "claude"}) {
		t.Errorf("hub harnesses = %v, want claude and codex inherited from its links", h.Harnesses)
	}
	if h.Scope != model.ScopeUser || h.Display != "~/.agents/AGENTS.md" {
		t.Errorf("hub scope %q display %q", h.Scope, h.Display)
	}
}

func TestOrgPolicyFieldsBecomeEmbeddedFiles(t *testing.T) {
	tr := testutil.New(t)
	tr.File(filepath.Join(tr.Etc, "claude-code/managed-settings.json"), `{
  "model": "x",
  "claudeMd": "# Org\nNo secrets in git.\n"
}`)
	tr.File(filepath.Join(tr.Etc, "codex/requirements.toml"), "additional_developer_instructions = \"\"\"\n# Org\nNo secrets in git.\n\"\"\"\n")
	tr.File(filepath.Join(tr.Etc, "claude-code/CLAUDE.md"), "# Also org\n")

	res := scan(t, env(tr))
	id := filepath.Join(tr.Etc, "claude-code/managed-settings.json") + "#claudeMd"
	f := must(t, res, id)
	if f.Content != "# Org\nNo secrets in git.\n" || f.Field != "claudeMd" || f.Scope != model.ScopeOrg {
		t.Errorf("embedded = %+v content %q", f, f.Content)
	}
	if f.Kind != model.KindInstruction || !reflect.DeepEqual(f.Harnesses, []string{"claude"}) {
		t.Errorf("kind %q harnesses %v", f.Kind, f.Harnesses)
	}
	c := must(t, res, filepath.Join(tr.Etc, "codex/requirements.toml")+"#additional_developer_instructions")
	if c.Content != "# Org\nNo secrets in git.\n" {
		t.Errorf("toml field content %q", c.Content)
	}
	if c.Access.Writable {
		t.Error("TOML fields are read-only in the editor")
	}
	must(t, res, filepath.Join(tr.Etc, "claude-code/CLAUDE.md"))
}

func TestEmptyOrMissingFieldIsSkipped(t *testing.T) {
	tr := testutil.New(t)
	tr.File(filepath.Join(tr.Etc, "claude-code/managed-settings.json"), `{"claudeMd": ""}`)
	res := scan(t, env(tr))
	if _, ok := res.Files[filepath.Join(tr.Etc, "claude-code/managed-settings.json")+"#claudeMd"]; ok {
		t.Error("an empty claudeMd is not an agent file")
	}
}

func TestSkillDirectoryLinks(t *testing.T) {
	tr := testutil.New(t)
	tr.File(filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md"), "---\nname: tdd\ndescription: >\n  Test first,\n  then code.\n---\n# TDD\n")
	tr.Link(filepath.Join(tr.Home, ".claude/skills/tdd"), "../../.agents/skills/tdd")
	tr.Link(filepath.Join(tr.Home, ".claude/skills/gone"), "../../.agents/skills/gone")

	res := scan(t, env(tr))
	real := filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md")
	link := must(t, res, filepath.Join(tr.Home, ".claude/skills/tdd/SKILL.md"))
	if !link.IsLink || link.LinkTarget != real {
		t.Errorf("skill reached through a directory link: %+v", link)
	}
	s := must(t, res, real)
	if s.Name != "tdd" || s.Description != "Test first, then code." {
		t.Errorf("frontmatter name %q description %q", s.Name, s.Description)
	}
	if !reflect.DeepEqual(sorted(s.Harnesses), []string{"claude", "codex"}) {
		t.Errorf("skill harnesses = %v", s.Harnesses)
	}
	gone := must(t, res, filepath.Join(tr.Home, ".claude/skills/gone"))
	if !gone.Missing || !gone.IsLink {
		t.Errorf("a dangling skill link stays visible as missing: %+v", gone)
	}
}

func sorted(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

func TestWalkFindsProjectFilesAndSkipsNoise(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.File(filepath.Join(app, "AGENTS.md"), "# App\n")
	tr.Link(filepath.Join(app, "CLAUDE.md"), "AGENTS.md")
	tr.File(filepath.Join(app, "web/AGENTS.md"), "# Web\n")
	tr.File(filepath.Join(app, ".claude/agents/reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs\n---\n")
	tr.File(filepath.Join(app, ".claude/commands/ship.md"), "Ship it\n")
	tr.File(filepath.Join(app, ".claude/rules/style.md"), "Style\n")
	tr.File(filepath.Join(app, ".claude/skills/deploy/SKILL.md"), "---\nname: deploy\n---\n")
	tr.File(filepath.Join(app, ".agents/skills/lint/SKILL.md"), "---\nname: lint\n---\n")
	tr.File(filepath.Join(app, "node_modules/pkg/AGENTS.md"), "# vendored\n")
	tr.File(filepath.Join(app, ".worktrees/topic/AGENTS.md"), "# worktree copy\n")
	tr.File(filepath.Join(app, "docs/README.md"), "not an agent file\n")
	wt := tr.Repo(filepath.Join(app, ".claude/worktrees/agent-1"))
	tr.File(filepath.Join(wt, "AGENTS.md"), "# a worktree checkout\n")
	lib := tr.Repo(filepath.Join(tr.Code, "lib"))
	tr.File(filepath.Join(lib, "CLAUDE.local.md"), "local\n")

	res := scan(t, env(tr))
	for _, rel := range []string{"app/AGENTS.md", "app/CLAUDE.md", "app/web/AGENTS.md", "app/.claude/agents/reviewer.md",
		"app/.claude/commands/ship.md", "app/.claude/rules/style.md", "app/.claude/skills/deploy/SKILL.md",
		"app/.agents/skills/lint/SKILL.md", "lib/CLAUDE.local.md"} {
		must(t, res, filepath.Join(tr.Code, rel))
	}
	for _, rel := range []string{"app/node_modules/pkg/AGENTS.md", "app/.worktrees/topic/AGENTS.md", "app/docs/README.md", "app/.claude/worktrees/agent-1/AGENTS.md"} {
		if _, ok := res.Files[filepath.Join(tr.Code, rel)]; ok {
			t.Errorf("%s should be skipped", rel)
		}
	}
	if got := must(t, res, filepath.Join(app, ".claude/agents/reviewer.md")); got.Kind != model.KindSubagent || got.Name != "reviewer" {
		t.Errorf("subagent %+v", got)
	}
	if got := must(t, res, filepath.Join(app, ".agents/skills/lint/SKILL.md")); got.Kind != model.KindSkill || !reflect.DeepEqual(got.Harnesses, []string{"codex"}) {
		t.Errorf("codex project skill %+v", got)
	}
	if got := must(t, res, filepath.Join(app, "AGENTS.md")); got.Repo != app || got.Scope != model.ScopeProject {
		t.Errorf("repo %q scope %q", got.Repo, got.Scope)
	}
	if !reflect.DeepEqual(res.Repos, []string{app, lib}) {
		t.Errorf("repos = %v", res.Repos)
	}
	wantDirs := []string{app, filepath.Join(app, "web"), lib}
	if !reflect.DeepEqual(res.InstructionDirs, wantDirs) {
		t.Errorf("instruction dirs = %v, want %v", res.InstructionDirs, wantDirs)
	}
}

func TestConfigExtendsWhatCounts(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.File(filepath.Join(app, "TEAM.md"), "# Team rules\n")
	tr.File(filepath.Join(app, "PI.md"), "# Pi rules\n")
	tr.File(filepath.Join(app, "generated/AGENTS.md"), "# generated\n")
	tr.File(filepath.Join(tr.Home, ".pi/agent/AGENTS.md"), "# Pi user file\n")
	e := env(tr)
	e.Codex.FallbackFilenames = []string{"TEAM.md"}
	e.Exclude = []string{"generated"}
	e.Config = config.Config{Harnesses: []config.StaticHarness{{
		Name: "pi", UserFiles: []string{filepath.Join(tr.Home, ".pi/agent/AGENTS.md")}, ProjectFiles: []string{"PI.md"},
	}}}
	res := scan(t, e)
	if got := must(t, res, filepath.Join(app, "TEAM.md")); !reflect.DeepEqual(got.Harnesses, []string{"codex"}) {
		t.Errorf("fallback file harnesses %v", got.Harnesses)
	}
	if got := must(t, res, filepath.Join(app, "PI.md")); !reflect.DeepEqual(got.Harnesses, []string{"pi"}) {
		t.Errorf("static harness project file %v", got.Harnesses)
	}
	if got := must(t, res, filepath.Join(tr.Home, ".pi/agent/AGENTS.md")); got.Scope != model.ScopeUser {
		t.Errorf("pi user file scope %q", got.Scope)
	}
	if _, ok := res.Files[filepath.Join(app, "generated/AGENTS.md")]; ok {
		t.Error("excluded directory was walked")
	}
}

func TestDanglingInstructionLink(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.Link(filepath.Join(app, "CLAUDE.md"), "AGENTS.md")
	res := scan(t, env(tr))
	f := must(t, res, filepath.Join(app, "CLAUDE.md"))
	if !f.Missing || f.LinkTarget != filepath.Join(app, "AGENTS.md") {
		t.Errorf("dangling link %+v", f)
	}
}

func TestReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anything")
	}
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	p := tr.File(filepath.Join(app, "AGENTS.md"), "# ro\n")
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	res := scan(t, env(tr))
	f := must(t, res, p)
	if f.Access.Writable || f.Access.Reason == "" {
		t.Errorf("access = %+v", f.Access)
	}
}

func TestParseFrontmatter(t *testing.T) {
	got := discover.ParseFrontmatter("---\nname: \"quoted name\"\ndescription: |\n  line one\n  line two\nallowed-tools: Bash\n---\nbody\n")
	want := map[string]string{"name": "quoted name", "description": "line one line two", "allowed-tools": "Bash"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(discover.ParseFrontmatter("no front matter")) != 0 {
		t.Error("no front matter gives no keys")
	}
}

func TestDisplay(t *testing.T) {
	cases := map[string]string{
		"/home/alex":              "~",
		"/home/alex/code/x.md":    "~/code/x.md",
		"/home/alexander/x.md":    "/home/alexander/x.md",
		"/etc/claude-code/x.json": "/etc/claude-code/x.json",
	}
	for in, want := range cases {
		if got := discover.Display("/home/alex", in); got != want {
			t.Errorf("Display(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodexSettings(t *testing.T) {
	dir := t.TempDir()
	if got := discover.LoadCodexSettings(dir); got.MaxBytes != 32768 || got.FallbackFilenames != nil {
		t.Errorf("defaults = %+v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("project_doc_max_bytes = 65536\nproject_doc_fallback_filenames = [\"CLAUDE.md\"]\nmodel = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := discover.LoadCodexSettings(dir)
	if got.MaxBytes != 65536 || !reflect.DeepEqual(got.FallbackFilenames, []string{"CLAUDE.md"}) {
		t.Errorf("settings = %+v", got)
	}
}
