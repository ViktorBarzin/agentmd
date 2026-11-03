package refs_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/refs"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

func build(t *testing.T, tr *testutil.Tree, cfg config.Config) (*discover.Result, []model.Ref) {
	t.Helper()
	env := discover.Env{
		Home: tr.Home, Etc: tr.Etc, CodexHome: filepath.Join(tr.Home, ".codex"), GOOS: "linux",
		Roots: []string{tr.Code}, Config: cfg,
		Codex: discover.CodexSettings{MaxBytes: discover.DefaultCodexMaxBytes},
	}
	res, err := discover.Scan(env)
	if err != nil {
		t.Fatal(err)
	}
	rs := refs.Build(refs.Input{Res: res, Home: tr.Home, Roots: env.Roots, Config: cfg})
	return res, rs
}

type key struct {
	from, to string
	kind     model.RefKind
	sub      string
}

func index(rs []model.Ref) map[key]model.Ref {
	m := map[key]model.Ref{}
	for _, r := range rs {
		m[key{r.From, r.To, r.Kind, r.Sub}] = r
	}
	return m
}

func has(t *testing.T, rs []model.Ref, from, to string, kind model.RefKind, sub string) model.Ref {
	t.Helper()
	r, ok := index(rs)[key{from, to, kind, sub}]
	if !ok {
		var lines []string
		for _, r := range rs {
			lines = append(lines, string(r.Kind)+"/"+r.Sub+" "+r.From+" -> "+r.To)
		}
		sort.Strings(lines)
		t.Fatalf("missing %s/%s %s -> %s; have:\n%s", kind, sub, from, to, strings.Join(lines, "\n"))
	}
	return r
}

func hasNot(t *testing.T, rs []model.Ref, from, to string) {
	t.Helper()
	for _, r := range rs {
		if r.From == from && r.To == to {
			t.Fatalf("unexpected %s/%s %s -> %s", r.Kind, r.Sub, from, to)
		}
	}
}

func TestSymlinks(t *testing.T) {
	tr := testutil.New(t)
	hub := tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "# Hub\n")
	link := tr.Link(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "../.agents/AGENTS.md")
	tr.File(filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md"), "---\nname: tdd\n---\n")
	tr.Link(filepath.Join(tr.Home, ".claude/skills/tdd"), "../../.agents/skills/tdd")
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.Link(filepath.Join(app, "CLAUDE.md"), "AGENTS.md")

	_, rs := build(t, tr, config.Config{})
	has(t, rs, link, hub, model.RefSymlink, "link")
	has(t, rs, filepath.Join(tr.Home, ".claude/skills/tdd/SKILL.md"), filepath.Join(tr.Home, ".agents/skills/tdd/SKILL.md"), model.RefSymlink, "dir")
	r := has(t, rs, filepath.Join(app, "CLAUDE.md"), filepath.Join(app, "AGENTS.md"), model.RefSymlink, "link")
	if !r.Dangling {
		t.Error("a link to a missing file dangles")
	}
}

func TestBuiltFromHeader(t *testing.T) {
	tr := testutil.New(t)
	hub := tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "<!-- Built by agents-sync from ~/.agents/{core,personal}.md; edit those. -->\n\n# Core\n")
	core := tr.File(filepath.Join(tr.Home, ".agents/core.md"), "# Core\n")
	personal := tr.File(filepath.Join(tr.Home, ".agents/personal.md"), "# Personal\n")
	link := tr.Link(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "../.agents/AGENTS.md")

	res, rs := build(t, tr, config.Config{})
	has(t, rs, hub, core, model.RefBuild, "header")
	has(t, rs, hub, personal, model.RefBuild, "header")
	f := res.Files[hub]
	if f.Access.Writable || f.Access.BuiltBy != "agents-sync" || len(f.Access.BuiltFrom) != 2 {
		t.Errorf("hub access = %+v", f.Access)
	}
	if l := res.Files[link]; l.Access.BuiltBy != "agents-sync" || l.Access.Writable {
		t.Errorf("a link to a built file carries its access: %+v", l.Access)
	}
}

func TestBuiltFromConfigAndOrigin(t *testing.T) {
	tr := testutil.New(t)
	out := tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "# a\n# b\n")
	a := tr.File(filepath.Join(tr.Home, ".agents/a.md"), "# a\n")
	b := tr.File(filepath.Join(tr.Home, ".agents/b.md"), "# b\n")
	settings := tr.File(filepath.Join(tr.Etc, "claude-code/managed-settings.json"), `{"claudeMd": "# Org\n"}`)
	src := tr.File(filepath.Join(tr.Code, "infra/managed-settings.json"), `{"claudeMd": "# Org\n"}`)
	codexCopy := tr.File(filepath.Join(tr.Etc, "codex/requirements.toml"), "additional_developer_instructions = \"# Org\\n\"\n")
	cfg := config.Config{
		Builds: []config.BuildRule{{Output: out, Parts: []string{a, b}, Command: "make agents"}},
		Origins: []config.OriginRule{
			{Path: settings, Field: "claudeMd", Source: []string{filepath.Join(tr.Code, "missing.json"), src}, Note: "installed hourly"},
			{Path: codexCopy, Field: "additional_developer_instructions", Source: []string{src}, SourceField: "claudeMd"},
		},
	}
	res, rs := build(t, tr, cfg)
	has(t, rs, out, a, model.RefBuild, "config")
	has(t, rs, out, b, model.RefBuild, "config")
	has(t, rs, settings+"#claudeMd", src+"#claudeMd", model.RefBuild, "origin")
	has(t, rs, codexCopy+"#additional_developer_instructions", src+"#claudeMd", model.RefBuild, "origin")
	copyFile := res.Files[settings+"#claudeMd"]
	if copyFile.Access.Origin == nil || copyFile.Access.Origin.FileID != src+"#claudeMd" || copyFile.Access.Origin.Note != "installed hourly" {
		t.Errorf("origin = %+v", copyFile.Access.Origin)
	}
	if res.Files[out].Access.BuiltBy != "make agents" {
		t.Errorf("built by %q", res.Files[out].Access.BuiltBy)
	}
}

func TestClaudeImports(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	claude := tr.File(filepath.Join(app, "CLAUDE.md"), strings.Join([]string{
		"# App",
		"See @docs/style.md for style.",
		"Also @~/.claude/shared.md",
		"Mail me at alex@example.com, not an import.",
		"Inline `@docs/not-an-import.md` is code.",
		"```",
		"@docs/in-a-fence.md",
		"```",
		"Missing: @docs/gone.md",
	}, "\n"))
	style := tr.File(filepath.Join(app, "docs/style.md"), "Style.\nSee @nested.md too.\n")
	nested := tr.File(filepath.Join(app, "docs/nested.md"), "Nested.\n")
	shared := tr.File(filepath.Join(tr.Home, ".claude/shared.md"), "Shared.\n")
	tr.File(filepath.Join(app, "docs/not-an-import.md"), "x\n")
	tr.File(filepath.Join(app, "docs/in-a-fence.md"), "x\n")

	res, rs := build(t, tr, config.Config{})
	r := has(t, rs, claude, style, model.RefBuild, "import")
	if r.Line != 2 {
		t.Errorf("import line = %d, want 2", r.Line)
	}
	has(t, rs, claude, shared, model.RefBuild, "import")
	has(t, rs, style, nested, model.RefBuild, "import")
	hasNot(t, rs, claude, filepath.Join(app, "docs/in-a-fence.md"))
	for _, r := range rs {
		if r.Kind == model.RefBuild && strings.HasSuffix(r.To, "not-an-import.md") {
			t.Error("code spans are not imports")
		}
	}
	g := has(t, rs, claude, filepath.Join(app, "docs/gone.md"), model.RefBuild, "import")
	if !g.Dangling {
		t.Error("a missing import dangles")
	}
	if f := res.Files[style]; f == nil || f.Kind != model.KindInstruction {
		t.Errorf("an imported file becomes an instruction node, got %+v", f)
	}
}

func TestMentions(t *testing.T) {
	tr := testutil.New(t)
	infra := tr.Repo(filepath.Join(tr.Code, "infra"))
	tr.File(filepath.Join(infra, "docs/agents/terraform.md"), "# TF\n")
	tr.File(filepath.Join(infra, "docs/agents/secrets.md"), "# Secrets\n")
	tr.Dir(filepath.Join(infra, "docs/runbooks"))
	root := tr.File(filepath.Join(tr.Code, "AGENTS.md"), "Read `infra/AGENTS.md` first.\n")
	agents := tr.File(filepath.Join(infra, "AGENTS.md"), strings.Join([]string{
		"# Infra",
		"- Terraform rules: `docs/agents/terraform.md`.",
		"- Secrets: [secrets](docs/agents/secrets.md#vault)",
		"- Old runbook: docs/runbooks/old-runbook.md",
		"- Example: `path/to/doc.md`",
		"- Template: `docs/agents/users/<user>/AGENTS.md`",
		"- Site: https://example.com/docs/a.md",
		"- Asset: /assets/page.css",
		"```",
		"cat docs/runbooks/in-fence.md",
		"```",
	}, "\n"))

	res, rs := build(t, tr, config.Config{})
	has(t, rs, root, agents, model.RefMention, "path")
	r := has(t, rs, agents, filepath.Join(infra, "docs/agents/terraform.md"), model.RefMention, "path")
	if r.Line != 2 {
		t.Errorf("mention line = %d", r.Line)
	}
	has(t, rs, agents, filepath.Join(infra, "docs/agents/secrets.md"), model.RefMention, "link")
	d := has(t, rs, agents, filepath.Join(infra, "docs/runbooks/old-runbook.md"), model.RefMention, "path")
	if !d.Dangling || d.Line != 4 {
		t.Errorf("a missing file in an existing folder dangles: %+v", d)
	}
	for _, r := range rs {
		switch {
		case strings.Contains(r.To, "path/to"), strings.Contains(r.To, "<user>"), strings.Contains(r.To, "example.com"),
			strings.Contains(r.To, "page.css"), strings.Contains(r.To, "in-fence"):
			t.Errorf("should not be a reference: %+v", r)
		}
	}
	if f := res.Files[filepath.Join(infra, "docs/agents/terraform.md")]; f == nil || f.Kind != model.KindDoc {
		t.Errorf("a mentioned markdown file becomes a doc node: %+v", f)
	}
}

func TestReferencedDocsAreOneHop(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.File(filepath.Join(app, "AGENTS.md"), "See `docs/one.md`.\n")
	one := tr.File(filepath.Join(app, "docs/one.md"), "See `two.md` and `../AGENTS.md`.\n")
	two := filepath.Join(app, "docs/two.md")
	tr.File(two, "End.\n")
	res, rs := build(t, tr, config.Config{})
	if _, ok := res.Files[two]; ok {
		t.Error("a doc's own mentions do not add second-hop nodes")
	}
	has(t, rs, one, filepath.Join(app, "AGENTS.md"), model.RefMention, "path")
}

func TestSkillMentions(t *testing.T) {
	tr := testutil.New(t)
	skill := tr.File(filepath.Join(tr.Home, ".agents/skills/publish-page/SKILL.md"), "---\nname: publish-page\n---\n")
	core := tr.File(filepath.Join(tr.Home, ".agents/core.md"), strings.Join([]string{
		"Publish with the `publish-page` skill.",
		"Run /publish-page when done.",
		"Use the `missing-thing` skill.",
		"Paths like /etc/hosts are not skills.",
	}, "\n"))
	hub := tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "x\n")
	_ = hub
	cfg := config.Config{Builds: []config.BuildRule{{Output: hub, Parts: []string{core}}}}
	_, rs := build(t, tr, cfg)
	r := has(t, rs, core, skill, model.RefMention, "skill")
	if r.Line != 1 {
		t.Errorf("first mention line = %d", r.Line)
	}
	var dangling bool
	for _, r := range rs {
		if r.From == core && r.Sub == "skill" && r.Dangling && r.Text == "missing-thing" {
			dangling = true
		}
		if strings.Contains(r.To, "hosts") {
			t.Errorf("/etc/hosts is not a skill: %+v", r)
		}
	}
	if !dangling {
		t.Error("a backticked skill name that no harness offers dangles")
	}
}

func TestNoSelfReferences(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	p := tr.File(filepath.Join(app, "AGENTS.md"), "This file is `AGENTS.md`.\n")
	_, rs := build(t, tr, config.Config{})
	hasNot(t, rs, p, p)
}

func TestExpandBraces(t *testing.T) {
	cases := map[string][]string{
		"~/.agents/{core,personal}.md": {"~/.agents/core.md", "~/.agents/personal.md"},
		"a.md":                         {"a.md"},
		"{a,b}/{x,y}.md":               {"a/x.md", "a/y.md", "b/x.md", "b/y.md"},
	}
	for in, want := range cases {
		got := refs.ExpandBraces(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("ExpandBraces(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// danglingSkills lists the names of the dangling skill mentions from one file.
func danglingSkills(rs []model.Ref, from string) []string {
	var out []string
	for _, r := range rs {
		if r.From == from && r.Sub == "skill" && r.Dangling {
			out = append(out, r.Text)
		}
	}
	sort.Strings(out)
	return out
}

func TestSlashSkillMentions(t *testing.T) {
	cases := []struct {
		name, line string
		want       []string
	}{
		{"backticked after run", "Before publishing, run `/doc-tone`.", []string{"/doc-tone"}},
		{"backticked then skill", "Run the `/doc-tone` skill first.", []string{"/doc-tone"}},
		{"bare then skill", "Finish with the /wrap-up skill.", []string{"/wrap-up"}},
		{"bare after invoke", "Invoke /grill-me when the plan is fuzzy.", []string{"/grill-me"}},
		{"bare after typing", "Typing /triage starts it.", []string{"/triage"}},
		{"an offered skill links instead", "Run /publish-page when done.", nil},
		{"an offered skill, backticked", "Use the `/publish-page` skill.", nil},
		{"a path after run", "Then run /usr/local/bin/agentmd check.", nil},
		{"a root folder after run", "Run `/tmp` cleanup weekly.", nil},
		{"a root folder before skill", "The /etc skill folder is empty.", nil},
		{"an endpoint", "Hit `/healthz` to check the pod.", nil},
		{"an endpoint after call", "Then call `/streams` for the list.", nil},
		{"a URL", "See https://example.com/doc-tone skill notes.", nil},
		{"a backticked slash with no skill context", "The `/relay` route forwards it.", nil},
		{"a command inside a longer code span", "Bake it in (`RUN /docker-mods || true` then `ENV X=`).", nil},
		{"a shouted Dockerfile verb", "RUN /docker-mods at build time.", nil},
		{"a code span that does not close on the line", "The container runs `/gluetun-entrypoint", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := testutil.New(t)
			tr.File(filepath.Join(tr.Home, ".agents/skills/publish-page/SKILL.md"), "---\nname: publish-page\n---\n")
			f := tr.File(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "# User\n"+c.line+"\n")
			_, rs := build(t, tr, config.Config{})
			if got := danglingSkills(rs, f); strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("dangling skills = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSlashSkillInFenceIsNotAMention(t *testing.T) {
	tr := testutil.New(t)
	f := tr.File(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "# User\n```\nrun `/doc-tone`\n```\n")
	_, rs := build(t, tr, config.Config{})
	if got := danglingSkills(rs, f); len(got) != 0 {
		t.Errorf("dangling skills = %v, want none", got)
	}
}

func TestExamplePathsDoNotDangle(t *testing.T) {
	cases := []struct {
		token    string
		dangling bool
	}{
		{"docs/adr/0054-x.md", false},
		{"docs/adr/foo.md", false},
		{"docs/adr/0001-example.md", false},
		{"docs/adr/0002_xxx.md", false},
		{"docs/adr/2026-07-14-anubis-x-real-ip.md", true},
		{"docs/adr/0055-real-decision.md", true},
		{"docs/adr/x-ray.md", true},
		{"docs/adr/0054.md", true},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			tr := testutil.New(t)
			app := tr.Repo(filepath.Join(tr.Code, "app"))
			tr.Dir(filepath.Join(app, "docs/adr"))
			f := tr.File(filepath.Join(app, "AGENTS.md"), "# App\nWhen it lands, rename "+c.token+" to its final name.\n")
			_, rs := build(t, tr, config.Config{})
			var got bool
			for _, r := range rs {
				if r.From == f && r.Dangling && r.Text == c.token {
					got = true
				}
			}
			if got != c.dangling {
				t.Errorf("dangling = %v, want %v; refs %+v", got, c.dangling, rs)
			}
		})
	}
}

func TestExampleNamesStillLinkWhenTheFileExists(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	doc := tr.File(filepath.Join(app, "docs/adr/0054-x.md"), "# X\n")
	f := tr.File(filepath.Join(app, "AGENTS.md"), "# App\nSee docs/adr/0054-x.md.\n")
	_, rs := build(t, tr, config.Config{})
	has(t, rs, f, doc, model.RefMention, "path")
}
