package findings

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/ViktorBarzin/agentmd/internal/model"
)

func file(id, content string) *model.File {
	return &model.File{ID: id, Path: id, RealPath: id, Display: id, Kind: model.KindInstruction, Content: content,
		Size: len(content), Lines: strings.Count(content, "\n") + 1, Hash: fmt.Sprintf("%x", len(content)) + content,
		Access: model.Access{Writable: true}}
}

func files(fs ...*model.File) map[string]*model.File {
	m := map[string]*model.File{}
	for _, f := range fs {
		m[f.ID] = f
	}
	return m
}

func ctx(id string, ids ...string) model.Context {
	c := model.Context{ID: id, Harness: strings.SplitN(id, ":", 2)[0]}
	for _, i := range ids {
		c.Entries = append(c.Entries, model.ContextEntry{FileID: i})
	}
	return c
}

const rule = "Never commit secrets to git and always run the tests before you push a change."

// paragraph is long enough to count as a duplicate between unrelated files.
const paragraph = "When a change touches the database, write the migration first, run it against a copy of the real data, " +
	"and only then change the code that reads the new columns, so a failed deploy never leaves the schema and the code out of step."

func byKind(fs []model.Finding, kind string) []model.Finding {
	var out []model.Finding
	for _, f := range fs {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

func TestDuplicateBetweenCoLoadedFilesIsAProblem(t *testing.T) {
	user := file("/u/AGENTS.md", "# User\n\n"+rule+"\n")
	repo := file("/r/AGENTS.md", "# Repo\nIntro line.\n\n"+rule+"\nMore.\n")
	fs := Compute(Input{Files: files(user, repo), Contexts: []model.Context{ctx("claude:/r", user.ID, repo.ID)}})
	d := byKind(fs, model.FindDuplicate)
	if len(d) != 1 {
		t.Fatalf("want one duplicate, got %+v", fs)
	}
	f := d[0]
	if f.Severity != model.Problem || len(f.Contexts) != 1 || f.Contexts[0] != "claude:/r" {
		t.Errorf("finding = %+v", f)
	}
	lines := map[string]int{}
	for _, s := range f.Spans {
		lines[s.FileID] = s.StartLine
	}
	if len(lines) != 2 || lines[user.ID] != 3 || lines[repo.ID] != 4 {
		t.Errorf("spans = %+v", f.Spans)
	}
	if !strings.HasPrefix(f.Spans[0].Quote, "never commit secrets") {
		t.Errorf("quote = %q", f.Spans[0].Quote)
	}
}

func TestDuplicateInUnrelatedReposIsAHint(t *testing.T) {
	a := file("/code/a/AGENTS.md", "# A\n"+paragraph+"\n")
	b := file("/code/b/AGENTS.md", "# B\n"+paragraph+"\n")
	fs := Compute(Input{Files: files(a, b), Contexts: []model.Context{ctx("claude:/code/a", a.ID), ctx("claude:/code/b", b.ID)}})
	d := byKind(fs, model.FindDuplicate)
	if len(d) != 1 || d[0].Severity != model.Hint || len(d[0].Contexts) != 0 {
		t.Fatalf("got %+v", d)
	}
}

func TestShortOverlapIsNotADuplicate(t *testing.T) {
	a := file("/a.md", "always run the tests first please\n")
	b := file("/b.md", "you should always run the tests first\n")
	if d := byKind(Compute(Input{Files: files(a, b)}), model.FindDuplicate); len(d) != 0 {
		t.Errorf("fewer than %d shared words is not a duplicate: %+v", Window, d)
	}
}

func TestBuiltFilesAndCopiesAreNotDuplicates(t *testing.T) {
	core := file("/h/.agents/core.md", "# Core\n"+rule+"\n")
	hub := file("/h/.agents/AGENTS.md", "<!-- built -->\n# Core\n"+rule+"\n")
	hub.Access.BuiltFrom = []string{core.ID}
	link := &model.File{ID: "/h/.claude/CLAUDE.md", Path: "/h/.claude/CLAUDE.md", RealPath: hub.ID, IsLink: true, LinkTarget: hub.ID,
		Kind: model.KindInstruction, Content: hub.Content, Access: hub.Access}
	src := file("/code/infra/settings.json#claudeMd", "# Org\n"+strings.Repeat("org policy words here ", 3)+"\n")
	src.Field = "claudeMd"
	c1 := file("/etc/claude-code/managed-settings.json#claudeMd", src.Content)
	c1.Field = "claudeMd"
	c1.Access.Origin = &model.Origin{FileID: src.ID}
	c2 := file("/etc/codex/requirements.toml#x", src.Content)
	c2.Field = "x"
	c2.Access.Origin = &model.Origin{FileID: src.ID}
	repo := file("/code/infra/AGENTS.md", "# Infra\n"+rule+"\n")
	fs := Compute(Input{Files: files(core, hub, link, src, c1, c2, repo), Contexts: []model.Context{
		ctx("claude:/code/infra", c1.ID, link.ID, repo.ID),
		ctx("codex:/code/infra", c2.ID, repo.ID),
	}})
	d := byKind(fs, model.FindDuplicate)
	if len(d) != 1 {
		t.Fatalf("want only core vs infra, got %+v", d)
	}
	got := map[string]bool{d[0].Spans[0].FileID: true, d[0].Spans[1].FileID: true}
	if !got[core.ID] || !got[repo.ID] {
		t.Errorf("the part stands in for the built file: %+v", d[0].Spans)
	}
	if d[0].Severity != model.Problem {
		t.Error("the part is loaded wherever its built file is, so this co-loads")
	}
}

func TestIdenticalCopiesAreOneFinding(t *testing.T) {
	body := "---\nname: tdd\n---\n# TDD\n" + rule + "\n"
	a, b, c := file("/code/a/.claude/skills/tdd/SKILL.md", body), file("/code/b/.claude/skills/tdd/SKILL.md", body), file("/code/c/.claude/skills/tdd/SKILL.md", body)
	for _, f := range []*model.File{a, b, c} {
		f.Kind = model.KindSkill
		f.Hash = "same"
	}
	d := byKind(Compute(Input{Files: files(a, b, c)}), model.FindDuplicate)
	if len(d) != 1 || len(d[0].Spans) != 3 || !strings.Contains(d[0].Summary, "3 identical copies") {
		t.Fatalf("got %+v", d)
	}
}

// Swapping which file comes first must not change what is found.
func TestDuplicateSpansAreSymmetric(t *testing.T) {
	vocab := strings.Fields("agent file rule test commit push secret skill harness context probe budget line word")
	gen := func(r *rand.Rand, n int) string {
		ws := make([]string, n)
		for i := range ws {
			ws[i] = vocab[r.Intn(len(vocab))]
			if r.Intn(9) == 0 {
				ws[i] += "\n"
			}
		}
		return strings.Join(ws, " ")
	}
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		shared := gen(r, 30+r.Intn(20))
		x := gen(r, r.Intn(30)) + "\n" + shared + "\n" + gen(r, r.Intn(30))
		y := gen(r, r.Intn(30)) + "\n" + shared + "\n" + gen(r, r.Intn(30))
		spansOf := func(first, second string) map[string]bool {
			a, b := file("/a.md", first), file("/b.md", second)
			out := map[string]bool{}
			for _, f := range byKind(Compute(Input{Files: files(a, b)}), model.FindDuplicate) {
				var parts []string
				for _, s := range f.Spans {
					parts = append(parts, fmt.Sprintf("%s:%d-%d", s.FileID, s.StartLine, s.EndLine))
				}
				out[strings.Join(sortedCopy(parts), ",")] = true
			}
			return out
		}
		p, q := spansOf(x, y), spansOf(y, x)
		// Swapping the texts swaps the file names in every span.
		swapped := map[string]bool{}
		for k := range q {
			k = strings.NewReplacer("/a.md", "/b.md", "/b.md", "/a.md").Replace(k)
			parts := strings.Split(k, ",")
			swapped[strings.Join(sortedCopy(parts), ",")] = true
		}
		if len(p) == 0 || len(p) != len(swapped) {
			return false
		}
		for k := range p {
			if !swapped[k] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

func sortedCopy(xs []string) []string {
	out := append([]string(nil), xs...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestDangling(t *testing.T) {
	link := &model.File{ID: "/r/CLAUDE.md", Path: "/r/CLAUDE.md", Display: "/r/CLAUDE.md", Kind: model.KindInstruction, IsLink: true, Missing: true, LinkTarget: "/r/AGENTS.md"}
	doc := file("/r/AGENTS2.md", "see docs\n")
	fs := Compute(Input{Files: files(link, doc), Refs: []model.Ref{
		{From: link.ID, To: "/r/AGENTS.md", Kind: model.RefSymlink, Dangling: true},
		{From: doc.ID, To: "/r/docs/old.md", Kind: model.RefMention, Sub: "path", Line: 1, Text: "docs/old.md", Dangling: true},
		{From: doc.ID, To: "/r/docs/gone.md", Kind: model.RefBuild, Sub: "import", Line: 1, Text: "@docs/gone.md", Dangling: true},
		{From: doc.ID, To: "skill:nope", Kind: model.RefMention, Sub: "skill", Line: 1, Text: "nope", Dangling: true},
	}})
	d := byKind(fs, model.FindDangling)
	if len(d) != 4 {
		t.Fatalf("got %+v", d)
	}
	sev := map[string]string{}
	for _, f := range d {
		sev[f.Summary] = f.Severity
	}
	want := map[string]string{
		"/r/CLAUDE.md links to a file that does not exist: /r/AGENTS.md":                     model.Problem,
		"/r/AGENTS2.md points to docs/old.md, which does not exist":                          model.Hint,
		"/r/AGENTS2.md imports @docs/gone.md, which does not exist, so that text is dropped": model.Problem,
		"/r/AGENTS2.md mentions the nope skill, which no harness here offers":                model.Hint,
	}
	for s, v := range want {
		if sev[s] != v {
			t.Errorf("%q: severity %q, want %q (have %v)", s, sev[s], v, sev)
		}
	}
}

func TestBudgets(t *testing.T) {
	infra := file("/code/infra/AGENTS.md", strings.Repeat("line of text\n", 3000))
	big := file("/code/big/CLAUDE.md", strings.Repeat("x", ClaudeCharLimit+1))
	small := file("/code/small/AGENTS.md", "short\n")
	codexCtx := model.Context{ID: "codex:/code/infra", Harness: "codex", Entries: []model.ContextEntry{
		{FileID: infra.ID, Bytes: 20000, Truncated: true, LostBytes: infra.Size - 20000},
	}}
	codexSub := model.Context{ID: "codex:/code/infra/sub", Harness: "codex", Entries: []model.ContextEntry{
		{FileID: infra.ID, Bytes: 20000, Truncated: true, LostBytes: infra.Size - 20000},
	}}
	fs := Compute(Input{Files: files(infra, big, small), CodexMaxBytes: 32768, FileBudget: 30000,
		Contexts: []model.Context{codexCtx, codexSub, ctx("claude:/code/big", big.ID)}})
	b := byKind(fs, model.FindBudget)
	var summaries []string
	for _, f := range b {
		summaries = append(summaries, f.Summary)
		if f.Severity != model.Problem {
			t.Errorf("budget findings are problems: %+v", f)
		}
	}
	joined := strings.Join(summaries, "\n")
	for _, want := range []string{
		fmt.Sprintf("Codex cuts the last %d bytes of /code/infra/AGENTS.md at its 32768-byte project-doc budget", infra.Size-20000),
		"/code/big/CLAUDE.md has 40001 characters; Claude Code warns above 40000",
		fmt.Sprintf("/code/infra/AGENTS.md is %d bytes, over your 30000-byte budget", infra.Size),
		"/code/big/CLAUDE.md is 40001 bytes, over your 30000-byte budget",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	for _, f := range b {
		if strings.HasPrefix(f.Summary, "Codex cuts") {
			if len(f.Contexts) != 2 || f.Spans[0].StartLine != 20000/len("line of text\n")+1 {
				t.Errorf("one cut finding covers both contexts and starts at the cut line: %+v", f)
			}
		}
	}
	if len(b) != 4 {
		t.Errorf("want 4 budget findings, got %d:\n%s", len(b), joined)
	}
}

func TestNotLoaded(t *testing.T) {
	agents := file("/code/web/AGENTS.md", "# web\n")
	leaf := file("/code/web/deep/AGENTS.md", "# deep\n")
	c := model.Context{ID: "claude:/code/web", Harness: "claude", Display: "~/code/web", Skipped: []model.ContextEntry{
		{FileID: agents.ID, Reason: "Claude Code skips AGENTS.md when a CLAUDE.md exists further up"},
	}}
	x := model.Context{ID: "codex:/code/web/deep", Harness: "codex", Skipped: []model.ContextEntry{
		{FileID: leaf.ID, Reason: "Codex's 32768-byte project-doc budget ran out before this file"},
	}}
	n := byKind(Compute(Input{Files: files(agents, leaf), Contexts: []model.Context{c, x}}), model.FindNotLoaded)
	if len(n) != 1 || n[0].Severity != model.Hint || n[0].Summary != "Claude Code does not load /code/web/AGENTS.md in ~/code/web" {
		t.Fatalf("got %+v", n)
	}
}

func TestSortPutsProblemsFirst(t *testing.T) {
	fs := []model.Finding{
		{Kind: model.FindNotLoaded, Severity: model.Hint},
		{Kind: model.FindDangling, Severity: model.Problem},
		{Kind: model.FindContradiction, Severity: model.Problem},
		{Kind: model.FindDuplicate, Severity: model.Hint},
	}
	Sort(fs)
	var got []string
	for _, f := range fs {
		got = append(got, f.Severity+"/"+f.Kind)
	}
	want := "problem/contradiction problem/dangling hint/duplicate hint/not-loaded"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %v", got)
	}
}

func TestUnrelatedShortRepeatIsNotReported(t *testing.T) {
	a := file("/code/a/AGENTS.md", "# A\n"+rule+"\n")
	b := file("/code/b/AGENTS.md", "# B\n"+rule+"\n")
	if d := byKind(Compute(Input{Files: files(a, b)}), model.FindDuplicate); len(d) != 0 {
		t.Errorf("a %d-word repeat between files that never co-load is below the hint length: %+v", len(strings.Fields(rule)), d)
	}
}

func TestCodePluginsAndDocPairsAreLeftOut(t *testing.T) {
	fence := "```sh\n" + paragraph + "\n```\n"
	a := file("/code/a/AGENTS.md", "# A\n"+fence)
	b := file("/code/b/AGENTS.md", "# B\n"+fence)
	p1 := file("/h/.claude/plugins/x/skills/s/SKILL.md", paragraph+"\n")
	p1.Scope = model.ScopePlugin
	p2 := file("/code/c/AGENTS.md", paragraph+"\n")
	d1 := file("/code/docs/one.md", paragraph+"\n")
	d1.Kind = model.KindDoc
	d2 := file("/code/docs/two.md", "intro\n"+paragraph+"\n")
	d2.Kind = model.KindDoc
	got := byKind(Compute(Input{Files: files(a, b, p1, p2, d1, d2)}), model.FindDuplicate)
	for _, f := range got {
		for _, s := range f.Spans {
			switch s.FileID {
			case a.ID, b.ID:
				t.Errorf("text inside fenced code is not compared: %+v", f)
			case p1.ID:
				t.Errorf("plugin files are not compared: %+v", f)
			}
		}
		if len(f.Spans) == 2 && f.Spans[0].FileID == d1.ID && f.Spans[1].FileID == d2.ID {
			t.Errorf("two referenced docs are not compared with each other: %+v", f)
		}
	}
	// The instruction file still matches the docs that repeat it.
	if len(got) != 2 {
		t.Errorf("want c/AGENTS.md against each doc, got %+v", got)
	}
}

func TestPassagesPerPairAreCapped(t *testing.T) {
	var x, y strings.Builder
	for i := 0; i < 6; i++ {
		p := fmt.Sprintf("Section %d says ", i) + strings.Repeat(fmt.Sprintf("topic%d detail ", i), 20)
		x.WriteString(p + "\nfiller line one for x only\n")
		y.WriteString("different filler for y only\n" + p + "\n")
	}
	a, b := file("/code/a/AGENTS.md", x.String()), file("/code/b/AGENTS.md", y.String())
	d := byKind(Compute(Input{Files: files(a, b)}), model.FindDuplicate)
	if len(d) != MaxPassagesPerPair || !strings.Contains(d[0].Detail, "3 more passages") {
		t.Errorf("want %d passages and a note about the rest, got %d: %+v", MaxPassagesPerPair, len(d), d)
	}
}
