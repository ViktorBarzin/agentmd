// Package findings computes the deterministic findings: duplicates, dangling
// references, files over a budget and files a harness skips.
package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"

	"github.com/ViktorBarzin/agentmd/internal/model"
)

// Window is the number of words in a compared run.
const Window = 8

// MinProblemWords and MinHintWords are the shortest shared passages reported
// between co-loading files and between other files. Shorter matches are
// mostly shared commands, headings and link lists.
const (
	MinProblemWords = 12
	MinHintWords    = 25
)

// MaxPassagesPerPair caps the passages reported for one pair of files.
const MaxPassagesPerPair = 3

// ClaudeCharLimit is where Claude Code warns about an instruction file.
const ClaudeCharLimit = 40000

// Input is what Compute needs.
type Input struct {
	Files    map[string]*model.File
	Refs     []model.Ref
	Contexts []model.Context
	// CodexMaxBytes is Codex's project-doc budget.
	CodexMaxBytes int
	// FileBudget is the owner's own limit per instruction file; 0 means none.
	FileBudget int
}

// Compute returns the findings, problems first.
func Compute(in Input) []model.Finding {
	c := newColoading(in)
	var out []model.Finding
	out = append(out, duplicates(in, c)...)
	out = append(out, dangling(in, c)...)
	out = append(out, budgets(in)...)
	out = append(out, notLoaded(in)...)
	Sort(out)
	return out
}

var kindOrder = map[string]int{
	model.FindContradiction: 0, model.FindDuplicate: 1, model.FindReworded: 2,
	model.FindBudget: 3, model.FindDangling: 4, model.FindNotLoaded: 5,
}

// Sort orders findings: problems first, then by kind, then by first file.
func Sort(fs []model.Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if (a.Severity == model.Problem) != (b.Severity == model.Problem) {
			return a.Severity == model.Problem
		}
		if kindOrder[a.Kind] != kindOrder[b.Kind] {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		return firstFile(a) < firstFile(b)
	})
}

func firstFile(f model.Finding) string {
	if len(f.Spans) == 0 {
		return ""
	}
	return f.Spans[0].FileID
}

// ID derives a stable finding id from its kind and spans.
func ID(kind string, spans []model.Span, extra ...string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	for _, s := range spans {
		fmt.Fprintf(h, "|%s:%d-%d", s.FileID, s.StartLine, s.EndLine)
	}
	for _, e := range extra {
		h.Write([]byte("|" + e))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// coloading knows which contexts load each file, counting a built file's parts
// as loaded wherever the built file is.
type coloading struct {
	byFile map[string][]string // file key -> context ids
}

// key identifies the content behind a file id: the real path, or the id for
// an embedded file.
func key(files map[string]*model.File, id string) string {
	f, ok := files[id]
	if !ok {
		return id
	}
	if f.Field != "" {
		return f.ID
	}
	return f.RealPath
}

func newColoading(in Input) *coloading {
	c := &coloading{byFile: map[string][]string{}}
	for _, ctx := range in.Contexts {
		seen := map[string]bool{}
		var mark func(id string, depth int)
		mark = func(id string, depth int) {
			k := key(in.Files, id)
			if seen[k] || depth > 5 {
				return
			}
			seen[k] = true
			c.byFile[k] = append(c.byFile[k], ctx.ID)
			f, ok := in.Files[id]
			if !ok {
				return
			}
			target := f
			if f.IsLink {
				if t, ok := in.Files[f.LinkTarget]; ok {
					target = t
				}
			}
			for _, p := range target.Access.BuiltFrom {
				mark(p, depth+1)
			}
		}
		for _, e := range ctx.Entries {
			if e.FileID != "" {
				mark(e.FileID, 0)
			}
		}
	}
	return c
}

// shared returns the contexts that load both files.
func (c *coloading) shared(a, b string) []string {
	in := map[string]bool{}
	for _, id := range c.byFile[a] {
		in[id] = true
	}
	var out []string
	for _, id := range c.byFile[b] {
		if in[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (c *coloading) loaded(k string) bool { return len(c.byFile[k]) > 0 }

// word is one normalised word and the line it came from.
type word struct {
	text string
	line int
}

// words normalises text for comparison: lower case, letters and digits only,
// leaving out fenced code blocks and HTML comments, where repeats are expected.
func words(s string) []word {
	return wordsOf(proseOnly(s))
}

// proseOnly blanks fenced code and HTML comments, keeping line numbers.
func proseOnly(s string) string {
	lines := strings.Split(s, "\n")
	inFence, inComment := false, false
	var fence string
	for i, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case inComment:
			if strings.Contains(t, "-->") {
				inComment = false
			}
			lines[i] = ""
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			if !inFence {
				inFence, fence = true, t[:3]
			} else if t[:3] == fence {
				inFence = false
			}
			lines[i] = ""
		case inFence:
			lines[i] = ""
		case strings.HasPrefix(t, "<!--"):
			if !strings.Contains(t, "-->") {
				inComment = true
			}
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func wordsOf(s string) []word {
	// Lower-case once and slice words out of it, so a word costs no
	// allocation of its own.
	lower := strings.ToLower(s)
	out := make([]word, 0, len(lower)/6)
	line, start := 1, -1
	for i, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, word{lower[start:i], line})
			start = -1
		}
		if r == '\n' {
			line++
		}
	}
	if start >= 0 {
		out = append(out, word{lower[start:], line})
	}
	return out
}

type doc struct {
	id    string // file id used in spans
	key   string
	words []word
	file  *model.File
}

// comparable lists the files whose text is compared: real files and embedded
// files with content, one per content key, skipping built files whose parts
// are all known (the parts stand in for them).
func comparable(in Input) []*doc {
	var ids []string
	for id := range in.Files {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	seen := map[string]bool{}
	var out []*doc
	for _, id := range ids {
		f := in.Files[id]
		if f.IsLink || f.Missing || f.Content == "" || f.Scope == model.ScopePlugin {
			continue
		}
		if len(f.Access.BuiltFrom) > 0 {
			all := true
			for _, p := range f.Access.BuiltFrom {
				if _, ok := in.Files[p]; !ok {
					all = false
				}
			}
			if all {
				continue
			}
		}
		k := key(in.Files, id)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, &doc{id: id, key: k, words: words(f.Content), file: f})
	}
	return out
}

// related reports pairs that repeat each other by design: a built file and
// its parts, an installed copy and its origin, two copies of one origin.
func related(a, b *model.File) bool {
	for _, p := range a.Access.BuiltFrom {
		if p == b.ID {
			return true
		}
	}
	for _, p := range b.Access.BuiltFrom {
		if p == a.ID {
			return true
		}
	}
	oa, ob := a.Access.Origin, b.Access.Origin
	switch {
	case oa != nil && oa.FileID == b.ID, ob != nil && ob.FileID == a.ID:
		return true
	case oa != nil && ob != nil && oa.FileID == ob.FileID:
		return true
	}
	return false
}

type pos struct{ doc, at int }

func duplicates(in Input, c *coloading) []model.Finding {
	docs := comparable(in)
	var out []model.Finding

	// Identical files first, as one finding per group, so vendored copies do
	// not produce a passage per paragraph.
	byHash := map[string][]int{}
	for i, d := range docs {
		byHash[d.file.Hash] = append(byHash[d.file.Hash], i)
	}
	skip := map[int]bool{}
	var hashes []string
	for h := range byHash {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	for _, h := range hashes {
		group := byHash[h]
		if len(group) < 2 || len(docs[group[0]].words) < Window {
			continue
		}
		var spans []model.Span
		var ctxs []string
		problem := false
		for gi, i := range group {
			d := docs[i]
			spans = append(spans, model.Span{FileID: d.id, StartLine: 1, EndLine: d.file.Lines})
			if gi > 0 {
				skip[i] = true
			}
			for _, j := range group[:gi] {
				if s := c.shared(docs[j].key, d.key); len(s) > 0 && !related(docs[j].file, d.file) {
					problem = true
					ctxs = append(ctxs, s...)
				}
			}
		}
		allRelated := true
		for gi := 1; gi < len(group); gi++ {
			if !related(docs[group[0]].file, docs[group[gi]].file) {
				allRelated = false
			}
		}
		if allRelated {
			continue
		}
		sev := model.Hint
		if problem {
			sev = model.Problem
		}
		out = append(out, model.Finding{
			ID: ID(model.FindDuplicate, spans), Kind: model.FindDuplicate, Severity: sev, Source: "rule",
			Summary:  fmt.Sprintf("%d identical copies of %s", len(group), docs[group[0]].file.Display),
			Spans:    spans,
			Contexts: uniq(ctxs),
		})
	}

	// Shared runs of Window words between two files.
	grams := map[uint64][]pos{}
	for i, d := range docs {
		if skip[i] {
			continue
		}
		for at := 0; at+Window <= len(d.words); at++ {
			h := gramHash(d.words[at : at+Window])
			grams[h] = append(grams[h], pos{i, at})
		}
	}
	type pair struct{ a, b int }
	hits := map[pair][][2]int{}
	for _, ps := range grams {
		if len(ps) < 2 || len(ps) > 60 {
			continue
		}
		for x := 0; x < len(ps); x++ {
			for y := x + 1; y < len(ps); y++ {
				p, q := ps[x], ps[y]
				if p.doc == q.doc {
					continue
				}
				if p.doc > q.doc {
					p, q = q, p
				}
				hits[pair{p.doc, q.doc}] = append(hits[pair{p.doc, q.doc}], [2]int{p.at, q.at})
			}
		}
	}
	var pairs []pair
	for p := range hits {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})
	for _, p := range pairs {
		da, db := docs[p.a], docs[p.b]
		if related(da.file, db.file) || (da.file.Kind == model.KindDoc && db.file.Kind == model.KindDoc) {
			continue
		}
		shared := c.shared(da.key, db.key)
		min := MinHintWords
		sev := model.Hint
		if len(shared) > 0 {
			min, sev = MinProblemWords, model.Problem
		}
		var found []model.Finding
		for _, run := range mergeRuns(hits[p]) {
			n := run.n + Window - 1
			if n < min {
				continue
			}
			wa := da.words[run.a : run.a+n]
			wb := db.words[run.b : run.b+n]
			spans := []model.Span{
				{FileID: da.id, StartLine: wa[0].line, EndLine: wa[len(wa)-1].line, Quote: excerpt(wa)},
				{FileID: db.id, StartLine: wb[0].line, EndLine: wb[len(wb)-1].line, Quote: excerpt(wb)},
			}
			found = append(found, model.Finding{
				ID: ID(model.FindDuplicate, spans), Kind: model.FindDuplicate, Severity: sev, Source: "rule",
				Summary:  fmt.Sprintf("The same %d words appear in %s and %s", n, da.file.Display, db.file.Display),
				Spans:    spans,
				Contexts: shared,
			})
		}
		if len(found) > MaxPassagesPerPair {
			sort.SliceStable(found, func(i, j int) bool { return passageLen(found[i]) > passageLen(found[j]) })
			more := len(found) - MaxPassagesPerPair
			found = found[:MaxPassagesPerPair]
			for i := range found {
				found[i].Detail = fmt.Sprintf("These files share %d more passages; the %d longest are listed.", more, MaxPassagesPerPair)
			}
		}
		out = append(out, found...)
	}
	return out
}

func passageLen(f model.Finding) int {
	var n int
	fmt.Sscanf(f.Summary, "The same %d words", &n)
	return n
}

func gramHash(ws []word) uint64 {
	h := fnv.New64a()
	for _, w := range ws {
		h.Write([]byte(w.text))
		h.Write([]byte{' '})
	}
	return h.Sum64()
}

type run struct{ a, b, n int }

// mergeRuns joins matching windows that continue each other in both files
// into runs of n windows.
func mergeRuns(hs [][2]int) []run {
	sort.Slice(hs, func(i, j int) bool {
		di, dj := hs[i][0]-hs[i][1], hs[j][0]-hs[j][1]
		if di != dj {
			return di < dj
		}
		return hs[i][0] < hs[j][0]
	})
	var out []run
	for i := 0; i < len(hs); {
		j := i
		for j+1 < len(hs) && hs[j+1][0]-hs[j+1][1] == hs[i][0]-hs[i][1] && hs[j+1][0] == hs[j][0]+1 {
			j++
		}
		out = append(out, run{a: hs[i][0], b: hs[i][1], n: j - i + 1})
		i = j + 1
	}
	// Drop runs that sit inside a longer run on the same side (the same
	// passage matched at a second place).
	sort.Slice(out, func(i, j int) bool { return out[i].n > out[j].n })
	var kept []run
	for _, r := range out {
		inside := false
		for _, k := range kept {
			if r.a >= k.a && r.a+r.n <= k.a+k.n && r.b >= k.b && r.b+r.n <= k.b+k.n {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, r)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].a < kept[j].a })
	return kept
}

func excerpt(ws []word) string {
	n := len(ws)
	if n > 24 {
		n = 24
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = ws[i].text
	}
	s := strings.Join(parts, " ")
	if len(ws) > n {
		s += " ..."
	}
	return s
}

func uniq(xs []string) []string {
	sort.Strings(xs)
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func dangling(in Input, c *coloading) []model.Finding {
	var out []model.Finding
	for _, r := range in.Refs {
		if !r.Dangling {
			continue
		}
		from, ok := in.Files[r.From]
		if !ok {
			continue
		}
		sev := model.Hint
		var summary string
		switch {
		case r.Kind == model.RefSymlink:
			summary = fmt.Sprintf("%s links to a file that does not exist: %s", from.Display, r.To)
			if c.loaded(key(in.Files, r.From)) || from.Kind == model.KindInstruction || from.Kind == model.KindSkill {
				sev = model.Problem
			}
		case r.Sub == "import":
			summary = fmt.Sprintf("%s imports %s, which does not exist, so that text is dropped", from.Display, r.Text)
			sev = model.Problem
		case r.Sub == "skill":
			summary = fmt.Sprintf("%s mentions the %s skill, which no harness here offers", from.Display, r.Text)
		case r.Kind == model.RefBuild:
			summary = fmt.Sprintf("%s is built from %s, which does not exist", from.Display, r.To)
			sev = model.Problem
		default:
			summary = fmt.Sprintf("%s points to %s, which does not exist", from.Display, r.Text)
		}
		line := r.Line
		spans := []model.Span{{FileID: r.From, StartLine: line, EndLine: line, Quote: r.Text}}
		out = append(out, model.Finding{
			ID: ID(model.FindDangling, spans, r.To), Kind: model.FindDangling, Severity: sev, Source: "rule",
			Summary: summary, Spans: spans, Contexts: c.byFile[key(in.Files, r.From)],
		})
	}
	return out
}

func budgets(in Input) []model.Finding {
	var out []model.Finding
	type cut struct {
		entry model.ContextEntry
		ctxs  []string
	}
	cuts := map[string]*cut{}
	claudeBig := map[string][]string{}
	for _, ctx := range in.Contexts {
		for _, e := range ctx.Entries {
			if e.Truncated && e.FileID != "" {
				k := fmt.Sprintf("%s|%d", e.FileID, e.LostBytes)
				if cuts[k] == nil {
					cuts[k] = &cut{entry: e}
				}
				cuts[k].ctxs = append(cuts[k].ctxs, ctx.ID)
			}
			if ctx.Harness == model.HarnessClaude && e.FileID != "" {
				if f, ok := in.Files[e.FileID]; ok && len([]rune(f.Content)) > ClaudeCharLimit {
					claudeBig[e.FileID] = append(claudeBig[e.FileID], ctx.ID)
				}
			}
		}
	}
	var keys []string
	for k := range cuts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ct := cuts[k]
		f := in.Files[ct.entry.FileID]
		display := ct.entry.FileID
		lines := 0
		if f != nil {
			display, lines = f.Display, f.Lines
		}
		spans := []model.Span{{FileID: ct.entry.FileID, StartLine: lineAt(f, ct.entry.Bytes), EndLine: lines}}
		out = append(out, model.Finding{
			ID: ID(model.FindBudget, spans, "codex"), Kind: model.FindBudget, Severity: model.Problem, Source: "rule",
			Summary: fmt.Sprintf("Codex cuts the last %d bytes of %s at its %d-byte project-doc budget", ct.entry.LostBytes, display, in.CodexMaxBytes),
			Detail:  "Codex joins every project doc from the repository root down and stops at the budget, in the middle of a line and without a warning.",
			Spans:   spans, Contexts: uniq(ct.ctxs),
		})
	}
	var bigIDs []string
	for id := range claudeBig {
		bigIDs = append(bigIDs, id)
	}
	sort.Strings(bigIDs)
	for _, id := range bigIDs {
		f := in.Files[id]
		spans := []model.Span{{FileID: id, StartLine: 1, EndLine: f.Lines}}
		out = append(out, model.Finding{
			ID: ID(model.FindBudget, spans, "claude"), Kind: model.FindBudget, Severity: model.Problem, Source: "rule",
			Summary: fmt.Sprintf("%s has %d characters; Claude Code warns above %d", f.Display, len([]rune(f.Content)), ClaudeCharLimit),
			Spans:   spans, Contexts: uniq(claudeBig[id]),
		})
	}
	if in.FileBudget > 0 {
		seen := map[string]bool{}
		var ids []string
		for id := range in.Files {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			f := in.Files[id]
			if f.IsLink || f.Kind != model.KindInstruction || f.Size <= in.FileBudget {
				continue
			}
			k := key(in.Files, id)
			if seen[k] {
				continue
			}
			seen[k] = true
			spans := []model.Span{{FileID: id, StartLine: lineAt(f, in.FileBudget), EndLine: f.Lines}}
			out = append(out, model.Finding{
				ID: ID(model.FindBudget, spans, "owner"), Kind: model.FindBudget, Severity: model.Problem, Source: "rule",
				Summary: fmt.Sprintf("%s is %d bytes, over your %d-byte budget", f.Display, f.Size, in.FileBudget),
				Spans:   spans,
			})
		}
	}
	return out
}

// lineAt returns the line holding byte offset n.
func lineAt(f *model.File, n int) int {
	if f == nil || f.Content == "" {
		return 1
	}
	if n > len(f.Content) {
		n = len(f.Content)
	}
	return strings.Count(f.Content[:n], "\n") + 1
}

func notLoaded(in Input) []model.Finding {
	var out []model.Finding
	seen := map[string]bool{}
	for _, ctx := range in.Contexts {
		for _, s := range ctx.Skipped {
			if s.FileID == "" || strings.Contains(s.Reason, "budget") {
				continue
			}
			k := ctx.Harness + "|" + s.FileID
			if seen[k] {
				continue
			}
			seen[k] = true
			f := in.Files[s.FileID]
			display := s.FileID
			if f != nil {
				display = f.Display
			}
			spans := []model.Span{{FileID: s.FileID, StartLine: 0, EndLine: 0}}
			out = append(out, model.Finding{
				ID: ID(model.FindNotLoaded, spans, ctx.Harness), Kind: model.FindNotLoaded, Severity: model.Hint, Source: "rule",
				Summary: fmt.Sprintf("%s does not load %s in %s", harnessLabel(ctx.Harness), display, ctx.Display),
				Detail:  s.Reason, Spans: spans, Contexts: []string{ctx.ID},
			})
		}
	}
	return out
}

func harnessLabel(h string) string {
	switch h {
	case model.HarnessClaude:
		return "Claude Code"
	case model.HarnessCodex:
		return "Codex"
	case model.HarnessAgentsMD:
		return "The AGENTS.md standard"
	}
	return h
}
