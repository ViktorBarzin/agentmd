package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViktorBarzin/agentmd/internal/model"
)

// fake answers with a fixed JSON document and records what it was sent.
type fake struct {
	answer      string
	err         error
	material    string
	instruction string
}

func (f *fake) Name() string { return "fake" }
func (f *fake) Run(_ context.Context, material, instruction string, _ []byte) ([]byte, error) {
	f.material, f.instruction = material, instruction
	return []byte(f.answer), f.err
}

var docs = []Doc{
	{ID: "/u/AGENTS.md", Display: "~/AGENTS.md", Content: "# User\nAlways ask the user before committing.\nKeep replies short.\n", Editable: true},
	{ID: "/r/AGENTS.md", Display: "~/r/AGENTS.md", Content: "# Repo\n\nThe agent does **all** git work itself\nand never asks the user to commit.\nPrefer short answers.\n", Editable: true},
}

func TestAnalyseKeepsOnlyQuotesThatExist(t *testing.T) {
	r := &fake{answer: `{"findings":[
	  {"kind":"contradiction","summary":"Ask before committing versus never ask.","spans":[
	    {"file":"F1","start_line":9,"end_line":9,"quote":"Always ask the user before committing."},
	    {"file":"F2","start_line":3,"end_line":3,"quote":"The agent does all git work itself and never asks the user to commit."}]},
	  {"kind":"reworded","summary":"Invented.","spans":[
	    {"file":"F1","start_line":3,"end_line":3,"quote":"Keep replies short."},
	    {"file":"F2","start_line":5,"end_line":5,"quote":"Use bullet points everywhere."}]},
	  {"kind":"style","summary":"Not a kind we report.","spans":[]}
	]}`}
	fs, err := Analyse(context.Background(), r, "claude:/r", docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want only the finding whose quotes exist, got %+v", fs)
	}
	f := fs[0]
	if f.Kind != model.FindContradiction || f.Severity != model.Problem || f.Source != "analysis" || f.Contexts[0] != "claude:/r" {
		t.Errorf("finding = %+v", f)
	}
	// Line numbers are corrected from the quote, and a quote across two lines
	// with markdown emphasis dropped still matches.
	if f.Spans[0].FileID != "/u/AGENTS.md" || f.Spans[0].StartLine != 2 || f.Spans[0].EndLine != 2 {
		t.Errorf("span 0 = %+v", f.Spans[0])
	}
	if f.Spans[1].FileID != "/r/AGENTS.md" || f.Spans[1].StartLine != 3 || f.Spans[1].EndLine != 4 {
		t.Errorf("span 1 = %+v", f.Spans[1])
	}
	if !strings.Contains(r.material, "=== F1: ~/AGENTS.md ===") || !strings.Contains(r.material, "    2| Always ask the user") {
		t.Errorf("material = %s", r.material)
	}
	if strings.Contains(r.instruction, "Always ask") {
		t.Error("the file text goes in the material, not the instruction")
	}
}

func TestAnalyseErrors(t *testing.T) {
	if _, err := Analyse(context.Background(), &fake{err: errors.New("boom")}, "c", docs); err == nil {
		t.Error("runner errors surface")
	}
	if _, err := Analyse(context.Background(), &fake{answer: "not json"}, "c", docs); err == nil {
		t.Error("a malformed answer is an error")
	}
	fs, err := Analyse(context.Background(), &fake{}, "c", nil)
	if err != nil || len(fs) != 0 {
		t.Errorf("no docs, no run: %v %v", fs, err)
	}
}

func TestLocatePrefersTheNearestOccurrence(t *testing.T) {
	content := "a rule\nx\ny\nz\na rule\n"
	if s, e, ok := locate(content, "a rule", 5); !ok || s != 5 || e != 5 {
		t.Errorf("got %d-%d %v", s, e, ok)
	}
	if _, _, ok := locate(content, "missing", 1); ok {
		t.Error("a missing quote is not found")
	}
}

func TestPropose(t *testing.T) {
	f := model.Finding{ID: "x", Kind: model.FindReworded, Summary: "Short replies twice.", Spans: []model.Span{
		{FileID: "/u/AGENTS.md", StartLine: 3, EndLine: 3, Quote: "Keep replies short."},
		{FileID: "/r/AGENTS.md", StartLine: 5, EndLine: 5, Quote: "Prefer short answers."},
	}}
	r := &fake{answer: `{"rationale":"The user file covers every repo, so the repo copy goes.","edits":[{"file":"F2","old":"Prefer short answers.\n","new":""}]}`}
	p, err := Propose(context.Background(), r, f, docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 1 || p.Edits[0].FileID != "/r/AGENTS.md" || strings.Contains(p.Edits[0].After, "Prefer short") || p.Edits[0].Before != docs[1].Content {
		t.Errorf("proposal = %+v", p)
	}
	if !strings.Contains(r.material, "F2 lines 5-5") {
		t.Errorf("the finding is described with handles: %s", r.material)
	}

	for _, bad := range []string{
		`{"rationale":"r","edits":[{"file":"F9","old":"x","new":""}]}`,
		`{"rationale":"r","edits":[{"file":"F2","old":"not in the file","new":""}]}`,
		`{"rationale":"r","edits":[{"file":"F2","old":"","new":"x"}]}`,
	} {
		if _, err := Propose(context.Background(), &fake{answer: bad}, f, docs); err == nil {
			t.Errorf("want an error for %s", bad)
		}
	}
	ro := []Doc{docs[0], {ID: docs[1].ID, Display: docs[1].Display, Content: docs[1].Content, Editable: false, Note: "read-only"}}
	if _, err := Propose(context.Background(), &fake{answer: `{"rationale":"r","edits":[{"file":"F2","old":"Prefer short answers.","new":""}]}`}, f, ro); err == nil {
		t.Error("editing a read-only file is refused")
	}
}

const fakeClaudeCLI = `#!/bin/sh
# Answers like claude -p --output-format json, after checking the flags.
case "$*" in *"--setting-sources  --settings"*|*"--setting-sources --settings"*) ;; esac
for a in "$@"; do
  if [ "$prev" = "--system-prompt-file" ]; then grep -q "=== F1" "$a" || { echo '{"is_error":true,"result":"no material"}'; exit 1; }; fi
  prev="$a"
done
echo '{"is_error":false,"structured_output":{"findings":[]}}'
`

func TestClaudeRunnerPassesMaterialAsAFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(fakeClaudeCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Claude{Bin: bin, Env: os.Environ(), WorkDir: filepath.Join(dir, "work")}
	fs, err := Analyse(context.Background(), c, "c", docs)
	if err != nil || len(fs) != 0 {
		t.Fatalf("got %v, %v", fs, err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "work"))
	if len(entries) != 0 {
		t.Errorf("the material file is removed afterwards: %v", entries)
	}
}
