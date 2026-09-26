package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/analysis"
	"github.com/ViktorBarzin/agentmd/internal/cache"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// ErrNoAnalysisCLI means neither claude nor codex is installed.
var ErrNoAnalysisCLI = errors.New("no agent CLI (claude or codex) was found, so analysis is not available")

// analysisRecord is the last analysis of one context.
type analysisRecord struct {
	Key      string          `json:"key"`
	At       time.Time       `json:"at"`
	CLI      string          `json:"cli"`
	Findings []model.Finding `json:"findings"`
}

func recordKey(contextID string) string { return cache.Key("analysis-record", contextID) }

// runner returns the analysis runner for the chosen CLI.
func (a *App) runner() (analysis.Runner, error) {
	work := filepath.Join(a.opts.CacheDir, "analysis")
	switch a.analysisCLI {
	case "claude":
		return &analysis.Claude{Bin: a.analysisBin, Env: a.opts.Env, WorkDir: work}, nil
	case "codex":
		return &analysis.Codex{Bin: a.analysisBin, CodexHome: a.opts.CodexHome, Env: a.opts.Env, WorkDir: work}, nil
	}
	return nil, ErrNoAnalysisCLI
}

// docsFor lists the files a context loads, with built files replaced by
// their parts when every part appears in the built text.
func docsFor(res *discover.Result, c model.Context) []analysis.Doc {
	var out []analysis.Doc
	seen := map[string]bool{}
	add := func(f *model.File) {
		if seen[f.ID] || f.Content == "" {
			return
		}
		seen[f.ID] = true
		note := ""
		if !f.Access.Writable {
			note = "read-only"
		}
		out = append(out, analysis.Doc{ID: f.ID, Display: f.Display, Content: f.Content, Editable: f.Access.Writable, Note: note})
	}
	for _, e := range c.Entries {
		f, ok := res.Files[e.FileID]
		if !ok {
			continue
		}
		if f.IsLink {
			if t, ok := res.Files[f.LinkTarget]; ok {
				f = t
			}
		}
		if parts := partsIn(res, f); parts != nil {
			for _, p := range parts {
				add(p)
			}
			continue
		}
		add(f)
	}
	return out
}

// partsIn returns a built file's parts when each one is found verbatim in it.
func partsIn(res *discover.Result, f *model.File) []*model.File {
	if len(f.Access.BuiltFrom) == 0 {
		return nil
	}
	var parts []*model.File
	for _, id := range f.Access.BuiltFrom {
		p, ok := res.Files[id]
		if !ok || !strings.Contains(f.Content, strings.TrimSpace(p.Content)) {
			return nil
		}
		parts = append(parts, p)
	}
	return parts
}

func analysisKey(cli string, docs []analysis.Doc) string {
	parts := []string{analysis.PromptVersion, cli}
	for _, d := range docs {
		parts = append(parts, d.ID, discover.Hash(d.Content))
	}
	return cache.Key(parts...)
}

// analysisFor marks each context with its last analysis and returns the
// findings of analyses that still match the files.
func (a *App) analysisFor(res *discover.Result, contexts []model.Context) []model.Finding {
	var out []model.Finding
	for i := range contexts {
		c := &contexts[i]
		var rec analysisRecord
		if !a.store.Get("analysis", recordKey(c.ID), &rec) {
			continue
		}
		stale := rec.Key != analysisKey(rec.CLI, docsFor(res, *c))
		c.Analysis = &model.AnalysisInfo{At: rec.At, CLI: rec.CLI, Stale: stale, Findings: len(rec.Findings)}
		if !stale {
			out = append(out, rec.Findings...)
		}
	}
	return out
}

// Analyse runs the analysis of one context and caches it. A cached answer for
// the same file contents is reused.
func (a *App) Analyse(ctx context.Context, contextID string) ([]model.Finding, error) {
	st, err := a.State()
	if err != nil {
		return nil, err
	}
	var c *model.Context
	for i := range st.Contexts {
		if st.Contexts[i].ID == contextID {
			c = &st.Contexts[i]
		}
	}
	if c == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownContext, contextID)
	}
	r, err := a.runner()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	docs := docsFor(a.res, *c)
	a.mu.Unlock()
	key := analysisKey(r.Name(), docs)
	var rec analysisRecord
	if a.store.Get("analysis", recordKey(contextID), &rec) && rec.Key == key {
		return rec.Findings, nil
	}
	fs, err := analysis.Analyse(ctx, r, contextID, docs)
	if err != nil {
		return nil, err
	}
	rec = analysisRecord{Key: key, At: time.Now(), CLI: r.Name(), Findings: fs}
	if err := a.store.Put("analysis", recordKey(contextID), rec); err != nil {
		return nil, err
	}
	if _, err := a.Scan(); err != nil {
		return nil, err
	}
	return fs, nil
}

// ErrUnknownFinding is returned for a finding id not in the current state.
var ErrUnknownFinding = errors.New("unknown finding")

// Propose asks the analysis CLI for a fix to one finding. A read-only copy is
// replaced by its origin, so the edit lands where it belongs.
func (a *App) Propose(ctx context.Context, findingID string) (*model.Proposal, error) {
	st, err := a.State()
	if err != nil {
		return nil, err
	}
	var f *model.Finding
	for i := range st.Findings {
		if st.Findings[i].ID == findingID {
			f = &st.Findings[i]
		}
	}
	if f == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownFinding, findingID)
	}
	r, err := a.runner()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	docs, finding := fixDocs(a.res, *f)
	a.mu.Unlock()
	if len(docs) == 0 {
		return nil, errors.New("none of this finding's files has text to edit")
	}
	return analysis.Propose(ctx, r, finding, docs)
}

// fixDocs returns the docs for a fix proposal and the finding with its spans
// pointed at those docs.
func fixDocs(res *discover.Result, f model.Finding) ([]analysis.Doc, model.Finding) {
	var docs []analysis.Doc
	index := map[string]string{}
	seen := map[string]bool{}
	for _, s := range f.Spans {
		file, ok := res.Files[s.FileID]
		if !ok {
			continue
		}
		target := file
		if file.IsLink {
			if t, ok := res.Files[file.LinkTarget]; ok {
				target = t
			}
		}
		if !target.Access.Writable && target.Access.Origin != nil {
			if o, ok := res.Files[target.Access.Origin.FileID]; ok && o.Content == target.Content {
				target = o
			}
		}
		index[s.FileID] = target.ID
		if seen[target.ID] {
			continue
		}
		seen[target.ID] = true
		note := ""
		if !target.Access.Writable {
			note = "read-only"
		}
		docs = append(docs, analysis.Doc{ID: target.ID, Display: target.Display, Content: target.Content, Editable: target.Access.Writable, Note: note})
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Editable && !docs[j].Editable })
	out := f
	out.Spans = nil
	for _, s := range f.Spans {
		if id, ok := index[s.FileID]; ok {
			s.FileID = id
		}
		out.Spans = append(out.Spans, s)
	}
	return docs, out
}
