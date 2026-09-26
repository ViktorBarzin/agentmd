// Package analysis asks the owner's agent CLI to find contradictions and
// reworded duplicates in one runtime context, and to propose fixes. Every
// quoted passage is checked against the file before a finding is kept.
package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/findings"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/probe"
)

// PromptVersion changes when the prompts change, so old cached answers are
// not reused.
const PromptVersion = "1"

// Timeout bounds one CLI run.
const Timeout = 10 * time.Minute

// Runner runs one structured request through an agent CLI.
type Runner interface {
	Name() string
	// Run sends the material (kept out of the prompt where the CLI allows)
	// and a short instruction, and returns JSON matching schema.
	Run(ctx context.Context, material, instruction string, schema []byte) ([]byte, error)
}

// Claude runs `claude -p` with none of the owner's own instruction files.
type Claude struct {
	Bin     string
	Env     []string
	WorkDir string
	Timeout time.Duration
}

func (c *Claude) Name() string { return "claude" }

// Run passes the material as a system-prompt file so it stays out of prompt
// telemetry, and loads no user or project settings.
func (c *Claude) Run(ctx context.Context, material, instruction string, schema []byte) ([]byte, error) {
	if err := os.MkdirAll(c.WorkDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(c.WorkDir, "material-*.md")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(material); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	timeout := c.Timeout
	if timeout == 0 {
		timeout = Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, "-p", instruction, "--output-format", "json", "--json-schema", string(schema),
		"--setting-sources", "", "--settings", `{"disableAllHooks":true}`, "--no-session-persistence",
		"--strict-mcp-config", "--tools", "", "--disable-slash-commands", "--system-prompt-file", f.Name())
	cmd.Dir = c.WorkDir
	cmd.Env = cleanEnv(c.Env)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	var res struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("claude timed out after %s", timeout)
		}
		return nil, fmt.Errorf("claude did not answer with JSON: %v %s", runErr, tail(errb.String()+out.String(), 500))
	}
	if res.IsError {
		return nil, fmt.Errorf("claude: %s", tail(res.Result, 500))
	}
	if len(res.StructuredOutput) == 0 || string(res.StructuredOutput) == "null" {
		return nil, errors.New("claude returned no structured answer")
	}
	return res.StructuredOutput, nil
}

// Codex runs `codex exec` read-only and ephemeral, with MCP servers off.
type Codex struct {
	Bin       string
	CodexHome string
	Env       []string
	WorkDir   string
	Timeout   time.Duration
}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) Run(ctx context.Context, material, instruction string, schema []byte) ([]byte, error) {
	if err := os.MkdirAll(c.WorkDir, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(c.WorkDir, "codex-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	schemaFile := filepath.Join(dir, "schema.json")
	outFile := filepath.Join(dir, "answer.json")
	if err := os.WriteFile(schemaFile, schema, 0o600); err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only",
		"--output-schema", schemaFile, "-o", outFile}
	args = append(args, probe.DisableMCPArgs(c.CodexHome, dir)...)
	args = append(args, "-")
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Dir = dir
	cmd.Env = cleanEnv(c.Env)
	cmd.Stdin = strings.NewReader(instruction + "\n\n" + material)
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &errb, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("codex timed out after %s", timeout)
		}
		return nil, fmt.Errorf("codex: %v: %s", err, tail(errb.String(), 500))
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		return nil, fmt.Errorf("codex wrote no answer: %s", tail(errb.String(), 500))
	}
	return bytes.TrimSpace(data), nil
}

func cleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "TMUX" || name == "TMUX_PANE" || name == "CLAUDECODE" || name == "CLAUDE_CODE_ENTRYPOINT" {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// Doc is one file in the material.
type Doc struct {
	ID       string
	Display  string
	Content  string
	Editable bool
	// Note explains why a doc is not editable, or where edits go instead.
	Note string
}

// handle is the short name a doc gets in the material.
func handle(i int) string { return fmt.Sprintf("F%d", i+1) }

// Material renders docs with numbered lines under short handles.
func Material(header string, docs []Doc) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	for i, d := range docs {
		fmt.Fprintf(&b, "=== %s: %s", handle(i), d.Display)
		if d.Note != "" {
			fmt.Fprintf(&b, " (%s)", d.Note)
		}
		b.WriteString(" ===\n")
		for n, line := range strings.Split(strings.TrimRight(d.Content, "\n"), "\n") {
			fmt.Fprintf(&b, "%5d| %s\n", n+1, line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

const analyseHeader = `You review the instruction files that a coding agent loads together at the start of one session. They appear below, each under a handle (F1, F2, ...) with numbered lines.

Report two kinds of finding:
- contradiction: two statements that an agent cannot both follow.
- reworded: the same instruction stated twice in different words. Skip verbatim copies (already found elsewhere), headings, links to other files, and a general rule followed by a specific example of it.

Only report what would change how an agent behaves. For each finding give a one-sentence summary in plain English and two spans. A span names the file handle, the first and last line, and a quote copied exactly from those lines (at most two sentences). The two spans may be in the same file. If there is nothing to report, return an empty list.`

const analyseInstruction = "Review the instruction files in your system prompt and answer with JSON that matches the schema."

var analyseSchema = []byte(`{"type":"object","additionalProperties":false,"required":["findings"],"properties":{"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["kind","summary","spans"],"properties":{"kind":{"type":"string","enum":["contradiction","reworded"]},"summary":{"type":"string"},"spans":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"object","additionalProperties":false,"required":["file","start_line","end_line","quote"],"properties":{"file":{"type":"string"},"start_line":{"type":"integer"},"end_line":{"type":"integer"},"quote":{"type":"string"}}}}}}}}}`)

type rawSpan struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Quote     string `json:"quote"`
}

// Analyse returns the checked findings for one context.
func Analyse(ctx context.Context, r Runner, contextID string, docs []Doc) ([]model.Finding, error) {
	if len(docs) == 0 {
		return []model.Finding{}, nil
	}
	out, err := r.Run(ctx, Material(analyseHeader, docs), analyseInstruction, analyseSchema)
	if err != nil {
		return nil, err
	}
	var ans struct {
		Findings []struct {
			Kind    string    `json:"kind"`
			Summary string    `json:"summary"`
			Spans   []rawSpan `json:"spans"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out, &ans); err != nil {
		return nil, fmt.Errorf("the answer does not match the schema: %w", err)
	}
	var fs []model.Finding
	for _, a := range ans.Findings {
		if a.Kind != model.FindContradiction && a.Kind != model.FindReworded {
			continue
		}
		var spans []model.Span
		for _, s := range a.Spans {
			if sp, ok := check(docs, s); ok {
				spans = append(spans, sp)
			}
		}
		if len(spans) < 2 {
			continue
		}
		fs = append(fs, model.Finding{
			ID: findings.ID(a.Kind, spans, "analysis"), Kind: a.Kind, Severity: model.Problem, Source: "analysis",
			Summary: strings.TrimSpace(a.Summary), Spans: spans, Contexts: []string{contextID},
		})
	}
	if fs == nil {
		fs = []model.Finding{}
	}
	return fs, nil
}

// check finds a span's quote in its file and returns the span with corrected
// lines, or false when the quote is not there.
func check(docs []Doc, s rawSpan) (model.Span, bool) {
	var d *Doc
	for i := range docs {
		if handle(i) == strings.TrimSpace(s.File) {
			d = &docs[i]
		}
	}
	if d == nil || strings.TrimSpace(s.Quote) == "" {
		return model.Span{}, false
	}
	start, end, ok := locate(d.Content, s.Quote, s.StartLine)
	if !ok {
		return model.Span{}, false
	}
	return model.Span{FileID: d.ID, StartLine: start, EndLine: end, Quote: s.Quote}, true
}

// locate finds quote in content, ignoring differences in whitespace and the
// markdown markers a model tends to drop, and returns its line range. When
// the quote occurs more than once, the occurrence nearest near wins.
func locate(content, quote string, near int) (int, int, bool) {
	normContent, lineOf := normalise(content)
	normQuote, _ := normalise(quote)
	if normQuote == "" {
		return 0, 0, false
	}
	best, bestDist := -1, 1<<30
	for from := 0; ; {
		i := strings.Index(normContent[from:], normQuote)
		if i < 0 {
			break
		}
		at := from + i
		d := lineOf[at] - near
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			best, bestDist = at, d
		}
		from = at + 1
	}
	if best < 0 {
		return 0, 0, false
	}
	return lineOf[best], lineOf[best+len(normQuote)-1], true
}

// normalise lowers case, drops markdown emphasis and code markers, and folds
// runs of whitespace to one space. lineOf maps each output byte to its line.
func normalise(s string) (string, []int) {
	var b strings.Builder
	var lineOf []int
	line := 1
	space := false
	for _, r := range s {
		if r == '\n' {
			line++
		}
		switch {
		case r == '*' || r == '_' || r == '`' || r == '>':
			continue
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if b.Len() > 0 && !space {
				b.WriteByte(' ')
				lineOf = append(lineOf, line)
				space = true
			}
			continue
		}
		space = false
		w := strings.ToLower(string(r))
		b.WriteString(w)
		for range w {
			lineOf = append(lineOf, line)
		}
	}
	out := strings.TrimRight(b.String(), " ")
	return out, lineOf[:len(out)]
}

const fixHeader = `An owner is cleaning up the instruction files their coding agents read. One finding is described below, followed by the files involved, each under a handle (F1, F2, ...) with numbered lines. Files marked "read-only" cannot be edited.

Propose the smallest edit that resolves the finding. For a duplicate, keep the text in the file that loads in every place it is needed (usually the more general one) and remove it from the other. For a contradiction, keep the rule that the more specific file states and change or remove the other, unless the text makes the owner's intent clear. Do not add new rules or rewrite unrelated text.

Answer with a short rationale and a list of edits. Each edit names an editable file handle, an exact passage copied from that file (without the line numbers, long enough to appear only once) and its replacement (an empty string removes it).`

const fixInstruction = "Propose a fix for the finding in your system prompt and answer with JSON that matches the schema."

var fixSchema = []byte(`{"type":"object","additionalProperties":false,"required":["rationale","edits"],"properties":{"rationale":{"type":"string"},"edits":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["file","old","new"],"properties":{"file":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}}}}}}`)

// FindingText describes a finding for the fix prompt.
func FindingText(f model.Finding, docs []Doc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Finding (%s): %s\n", f.Kind, f.Summary)
	if f.Detail != "" {
		fmt.Fprintf(&b, "%s\n", f.Detail)
	}
	for _, s := range f.Spans {
		for i, d := range docs {
			if d.ID == s.FileID {
				fmt.Fprintf(&b, "- %s lines %d-%d", handle(i), s.StartLine, s.EndLine)
				if s.Quote != "" {
					fmt.Fprintf(&b, ": %q", s.Quote)
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// Propose asks for a fix and turns it into whole-file before and after texts.
func Propose(ctx context.Context, r Runner, f model.Finding, docs []Doc) (*model.Proposal, error) {
	header := fixHeader + "\n\n" + FindingText(f, docs)
	out, err := r.Run(ctx, Material(header, docs), fixInstruction, fixSchema)
	if err != nil {
		return nil, err
	}
	var ans struct {
		Rationale string `json:"rationale"`
		Edits     []struct {
			File string `json:"file"`
			Old  string `json:"old"`
			New  string `json:"new"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(out, &ans); err != nil {
		return nil, fmt.Errorf("the answer does not match the schema: %w", err)
	}
	after := map[int]string{}
	for _, e := range ans.Edits {
		idx := -1
		for i := range docs {
			if handle(i) == strings.TrimSpace(e.File) {
				idx = i
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("the proposal edits %q, which is not one of the files", e.File)
		}
		if !docs[idx].Editable {
			return nil, fmt.Errorf("the proposal edits %s, which is read-only", docs[idx].Display)
		}
		cur, ok := after[idx]
		if !ok {
			cur = docs[idx].Content
		}
		if e.Old == "" {
			return nil, fmt.Errorf("the proposal has an empty search text for %s", docs[idx].Display)
		}
		n := strings.Count(cur, e.Old)
		if n != 1 {
			return nil, fmt.Errorf("the proposal's search text appears %d times in %s, so it cannot be applied safely", n, docs[idx].Display)
		}
		after[idx] = strings.Replace(cur, e.Old, e.New, 1)
	}
	p := &model.Proposal{FindingID: f.ID, Rationale: strings.TrimSpace(ans.Rationale), Edits: []model.ProposalEdit{}}
	var idxs []int
	for i := range after {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		p.Edits = append(p.Edits, model.ProposalEdit{FileID: docs[i].ID, Display: docs[i].Display,
			BaseHash: discover.Hash(docs[i].Content), Before: docs[i].Content, After: tidy(after[i])})
	}
	return p, nil
}

// tidy collapses the runs of blank lines a removal leaves behind.
func tidy(s string) string {
	for strings.Contains(s, "\n\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n\n", "\n\n\n")
	}
	return s
}
