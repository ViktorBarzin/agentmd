// Package app assembles discovery, references, runtime contexts and findings
// into one State, and runs probes. The CLI and the HTTP server both use it.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/cache"
	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/findings"
	"github.com/ViktorBarzin/agentmd/internal/harness"
	"github.com/ViktorBarzin/agentmd/internal/managed"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/probe"
	"github.com/ViktorBarzin/agentmd/internal/refs"
)

// Parallel is how many probes run at once.
const Parallel = 4

// Options says where things are. Zero values fall back to the defaults for
// the current user.
type Options struct {
	Home            string
	Etc             string
	CodexHome       string
	ClaudeConfigDir string
	GlobalConfig    string
	GOOS            string
	Roots           []string
	ConfigPaths     []string
	CacheDir        string
	Env             []string
	Owner           string
	// NoCLIs skips looking for harness CLIs (tests, and machines without them).
	NoCLIs bool
}

// Defaults fills unset options from the environment.
func Defaults(o Options) (Options, error) {
	if o.Home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return o, discover.ErrNoHome
		}
		o.Home = h
	}
	if o.Etc == "" {
		o.Etc = "/etc"
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	if o.CodexHome == "" {
		o.CodexHome = os.Getenv("CODEX_HOME")
		if o.CodexHome == "" {
			o.CodexHome = filepath.Join(o.Home, ".codex")
		}
	}
	if o.ClaudeConfigDir == "" {
		o.ClaudeConfigDir = os.Getenv("CLAUDE_CONFIG_DIR")
		if o.ClaudeConfigDir == "" {
			o.ClaudeConfigDir = filepath.Join(o.Home, ".claude")
			if o.GlobalConfig == "" {
				o.GlobalConfig = filepath.Join(o.Home, ".claude.json")
			}
		}
	}
	if o.GlobalConfig == "" {
		o.GlobalConfig = filepath.Join(o.ClaudeConfigDir, ".claude.json")
	}
	if o.ConfigPaths == nil {
		o.ConfigPaths = config.DefaultPaths(o.Etc, o.Home)
	}
	if o.CacheDir == "" {
		o.CacheDir = cache.DefaultDir(o.Home)
	}
	if o.Owner == "" {
		if u, err := user.Current(); err == nil {
			o.Owner = u.Username
		}
	}
	return o, nil
}

// App holds the current state and the tools to refresh it.
type App struct {
	opts   Options
	cfg    config.Config
	store  cache.Store
	claude *probe.Claude
	codex  *probe.Codex
	cver   string
	xver   string

	marker *managed.Marker

	mu    sync.Mutex
	res   *discover.Result
	state *model.State
	// analysisCLI is the CLI used for analysis, empty when none.
	analysisCLI string
	analysisBin string
}

// New loads the config and finds the harness CLIs.
func New(opts Options) (*App, error) {
	opts, err := Defaults(opts)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(opts.Home, opts.ConfigPaths...)
	if err != nil {
		return nil, err
	}
	if len(opts.Roots) == 0 {
		opts.Roots = cfg.Roots
	}
	if len(opts.Roots) == 0 {
		if st, err := os.Stat(filepath.Join(opts.Home, "code")); err == nil && st.IsDir() {
			opts.Roots = []string{filepath.Join(opts.Home, "code")}
		}
	}
	a := &App{opts: opts, cfg: cfg, store: cache.Store{Dir: opts.CacheDir}}
	a.marker = &managed.Marker{Env: managed.Env{Home: opts.Home, CodexHome: opts.CodexHome, ClaudeConfigDir: opts.ClaudeConfigDir}}
	if !opts.NoCLIs {
		a.marker.Env.Chezmoi = probe.FindBinary("chezmoi", opts.Home)
	}
	if !opts.NoCLIs {
		ctx := context.Background()
		if bin := probe.FindBinary("claude", opts.Home); bin != "" {
			if v, err := probe.Version(ctx, bin); err == nil {
				a.cver = v
				a.claude = &probe.Claude{Bin: bin, ConfigDir: opts.ClaudeConfigDir, GlobalConfig: opts.GlobalConfig,
					ScratchRoot:     filepath.Join(opts.CacheDir, "claude-config"),
					ManagedSettings: filepath.Join(a.harnessEnv().ManagedDir(), "managed-settings.json"), Env: opts.Env}
			}
		}
		if bin := probe.FindBinary("codex", opts.Home); bin != "" {
			if v, err := probe.Version(ctx, bin); err == nil {
				a.xver = v
				a.codex = &probe.Codex{Bin: bin, CodexHome: opts.CodexHome, Env: opts.Env}
			}
		}
	}
	a.pickAnalysisCLI()
	return a, nil
}

func (a *App) pickAnalysisCLI() {
	want := a.cfg.Analysis.CLI
	switch {
	case (want == "" || want == "claude") && a.claude != nil:
		a.analysisCLI, a.analysisBin = "claude", a.claude.Bin
	case (want == "" || want == "codex") && a.codex != nil:
		a.analysisCLI, a.analysisBin = "codex", a.codex.Bin
	}
}

// Options returns the resolved options.
func (a *App) Options() Options { return a.opts }

// Config returns the loaded config.
func (a *App) Config() config.Config { return a.cfg }

func (a *App) codexSettings() discover.CodexSettings {
	return discover.LoadCodexSettings(a.opts.CodexHome)
}

func (a *App) discoverEnv() discover.Env {
	return discover.Env{Home: a.opts.Home, Etc: a.opts.Etc, CodexHome: a.opts.CodexHome, GOOS: a.opts.GOOS,
		Roots: a.opts.Roots, Exclude: a.cfg.Exclude, Config: a.cfg, Codex: a.codexSettings()}
}

func (a *App) harnessEnv() harness.Env {
	return harness.Env{Home: a.opts.Home, Etc: a.opts.Etc, CodexHome: a.opts.CodexHome, ClaudeConfigDir: a.opts.ClaudeConfigDir,
		GOOS: a.opts.GOOS, Roots: a.opts.Roots, Config: a.cfg, Codex: a.codexSettings()}
}

type probeEntry struct {
	FP      string        `json:"fp"`
	Loaded  []string      `json:"loaded"`
	Context model.Context `json:"context"`
}

func probeKey(h, dir string) string { return cache.Key("probe", h, dir) }

func (a *App) probeHarnesses() []string {
	var hs []string
	if a.claude != nil {
		hs = append(hs, model.HarnessClaude)
	}
	if a.codex != nil {
		hs = append(hs, model.HarnessCodex)
	}
	return hs
}

func (a *App) version(h string) string {
	if h == model.HarnessClaude {
		return a.cver
	}
	return a.xver
}

// Scan rebuilds the state from disk and the probe cache. It never probes.
func (a *App) Scan() (*model.State, error) {
	res, err := discover.Scan(a.discoverEnv())
	if err != nil {
		return nil, err
	}
	a.marker.Env.Lockfiles = lockfiles(a.opts.Home, res.Repos)
	a.marker.Apply(res.Files)
	rs := refs.Build(refs.Input{Res: res, Home: a.opts.Home, Roots: a.opts.Roots, Config: a.cfg})
	henv := a.harnessEnv()
	stats := harness.Stats{}
	cs := harness.LoadClaudeState(a.opts.GlobalConfig)

	var contexts []model.Context
	var unprobed []model.Candidate
	dirs := harness.Dirs(a.opts.Home, res, a.cfg.SkipContexts)
	for _, dir := range dirs {
		for _, h := range a.probeHarnesses() {
			var e probeEntry
			if !a.store.Get("probe", probeKey(h, dir), &e) {
				unprobed = append(unprobed, model.Candidate{Harness: h, Dir: dir, Display: discover.Display(a.opts.Home, dir)})
				continue
			}
			fp := harness.Fingerprint(henv, res, stats, h, a.version(h), dir, e.Loaded, cs)
			c := e.Context
			c.Stale = fp != e.FP
			reattach(res, &c)
			contexts = append(contexts, c)
		}
		if c, ok := harness.AgentsMD(henv, res, dir); ok {
			contexts = append(contexts, c)
		}
		for _, sh := range a.cfg.Harnesses {
			if c := harness.Static(henv, res, sh, dir); len(c.Entries) > 0 {
				contexts = append(contexts, c)
			}
		}
	}
	rs = dropOfferedSkills(rs, contexts)
	analysed := a.analysisFor(res, contexts)

	fs := findings.Compute(findings.Input{Files: res.Files, Refs: rs, Contexts: contexts,
		CodexMaxBytes: henv.Codex.MaxBytes, FileBudget: a.cfg.Budget.FileBytes})
	fs = append(fs, analysed...)
	findings.Sort(fs)

	st := &model.State{
		Owner: a.opts.Owner, Home: a.opts.Home, Roots: a.opts.Roots, ScannedAt: time.Now(),
		Harnesses: a.harnessInfo(), Refs: rs, Contexts: contexts, Findings: fs, Unprobed: unprobed,
		Analysis: a.analysisCLI,
	}
	for _, f := range res.Sorted() {
		st.Files = append(st.Files, *f)
	}
	if st.Refs == nil {
		st.Refs = []model.Ref{}
	}
	if st.Contexts == nil {
		st.Contexts = []model.Context{}
	}
	if st.Findings == nil {
		st.Findings = []model.Finding{}
	}
	a.mu.Lock()
	a.res, a.state = res, st
	a.mu.Unlock()
	return st, nil
}

// reattach makes sure every file a cached context names is in res, so a file
// outside the discovery roots (an import, say) still appears.
func reattach(res *discover.Result, c *model.Context) {
	for _, e := range c.Entries {
		if e.FileID == "" || strings.Contains(e.FileID, "#") {
			continue
		}
		if _, ok := res.Files[e.FileID]; !ok {
			res.Add(e.FileID, model.KindInstruction, []string{c.Harness})
		}
	}
}

func (a *App) harnessInfo() []model.HarnessInfo {
	out := []model.HarnessInfo{
		{Name: model.HarnessClaude, Label: "Claude Code", Available: a.claude != nil, Version: a.cver, Probe: true},
		{Name: model.HarnessCodex, Label: "Codex", Available: a.codex != nil, Version: a.xver, Probe: true},
		{Name: model.HarnessAgentsMD, Label: "AGENTS.md", Available: true},
	}
	for _, h := range a.cfg.Harnesses {
		out = append(out, model.HarnessInfo{Name: h.Name, Label: h.Name, Available: true})
	}
	return out
}

// State returns the last scanned state, scanning first when there is none.
func (a *App) State() (*model.State, error) {
	a.mu.Lock()
	st := a.state
	a.mu.Unlock()
	if st != nil {
		return st, nil
	}
	return a.Scan()
}

// Result returns the last discovery result.
func (a *App) Result() *discover.Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.res
}

// File returns a file with its content, from the last scan.
func (a *App) File(id string) (*model.File, error) {
	if _, err := a.State(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.res.Files[id]
	if !ok {
		return nil, fmt.Errorf("no agent file %q", id)
	}
	return f, nil
}

// ErrUnknownContext is returned for a context id that is not a probe candidate.
var ErrUnknownContext = errors.New("unknown context")

// Progress reports probe progress.
type Progress func(done, total int, id string, err error)

// Probe runs the probes for the given context ids, or for every unprobed or
// stale context when ids is empty, and returns the new state.
func (a *App) Probe(ctx context.Context, ids []string, progress Progress) (*model.State, error) {
	st, err := a.State()
	if err != nil {
		return nil, err
	}
	type job struct{ h, dir string }
	var jobs []job
	if len(ids) == 0 {
		for _, c := range st.Unprobed {
			jobs = append(jobs, job{c.Harness, c.Dir})
		}
		for _, c := range st.Contexts {
			if c.Stale && c.Source == "probe" {
				jobs = append(jobs, job{c.Harness, c.Dir})
			}
		}
	} else {
		for _, id := range ids {
			h, dir, ok := strings.Cut(id, ":")
			if !ok || (h != model.HarnessClaude && h != model.HarnessCodex) || !filepath.IsAbs(dir) {
				return nil, fmt.Errorf("%w: %s", ErrUnknownContext, id)
			}
			if (h == model.HarnessClaude && a.claude == nil) || (h == model.HarnessCodex && a.codex == nil) {
				return nil, fmt.Errorf("%s is not installed", h)
			}
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return nil, fmt.Errorf("%w: %s is not a directory", ErrUnknownContext, dir)
			}
			jobs = append(jobs, job{h, dir})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].dir+jobs[i].h < jobs[j].dir+jobs[j].h })

	var wg sync.WaitGroup
	sem := make(chan struct{}, Parallel)
	var mu sync.Mutex
	done := 0
	var firstErr error
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := a.probeOne(ctx, j.h, j.dir)
			mu.Lock()
			done++
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if progress != nil {
				progress(done, len(jobs), harness.ID(j.h, j.dir), err)
			}
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	st, err = a.Scan()
	if err != nil {
		return nil, err
	}
	if firstErr != nil && len(jobs) == 1 {
		return st, firstErr
	}
	return st, nil
}

// probeOne probes one context and caches the result. A failed probe is cached
// as a context with an error, so the UI can show why.
func (a *App) probeOne(ctx context.Context, h, dir string) error {
	res := a.Result()
	if res == nil {
		if _, err := a.Scan(); err != nil {
			return err
		}
		res = a.Result()
	}
	henv := a.harnessEnv()
	now := time.Now()
	var c model.Context
	var perr error
	switch h {
	case model.HarnessClaude:
		cap, err := a.claude.Probe(ctx, dir)
		if err != nil {
			perr = err
			break
		}
		a.mu.Lock()
		c = harness.FromClaude(henv, res, dir, a.cver, cap, now)
		a.mu.Unlock()
	case model.HarnessCodex:
		cap, err := a.codex.Probe(ctx, dir)
		if err != nil {
			perr = err
			break
		}
		a.mu.Lock()
		c = harness.FromCodex(henv, res, dir, a.xver, cap, now)
		a.mu.Unlock()
	}
	if perr != nil {
		c = model.Context{ID: harness.ID(h, dir), Harness: h, Dir: dir, Display: discover.Display(a.opts.Home, dir),
			Source: "probe", Version: a.version(h), ProbedAt: &now, Error: perr.Error(),
			Entries: []model.ContextEntry{}, Skills: []model.ContextItem{}, Subagents: []model.ContextItem{}}
	}
	a.mu.Lock()
	loaded := harness.LoadedPaths(res, c)
	fp := harness.Fingerprint(henv, res, harness.Stats{}, h, a.version(h), dir, loaded, harness.LoadClaudeState(a.opts.GlobalConfig))
	a.mu.Unlock()
	if err := a.store.Put("probe", probeKey(h, dir), probeEntry{FP: fp, Loaded: loaded, Context: c}); err != nil {
		return err
	}
	return perr
}

// lockfiles lists the skills-installer lockfiles that exist: the user's and
// any repository's own.
func lockfiles(home string, repos []string) []string {
	var out []string
	for _, d := range append([]string{home}, repos...) {
		p := filepath.Join(d, ".agents", ".skill-lock.json")
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// dropOfferedSkills clears the dangling mark on a skill mention when a probed
// harness offers a skill of that name, such as a built-in skill with no file.
func dropOfferedSkills(rs []model.Ref, contexts []model.Context) []model.Ref {
	offered := map[string]bool{}
	for _, c := range contexts {
		for _, s := range c.Skills {
			offered[s.Name] = true
			if i := strings.LastIndex(s.Name, ":"); i >= 0 {
				offered[s.Name[i+1:]] = true
			}
		}
	}
	out := rs[:0]
	for _, r := range rs {
		if r.Dangling && r.Sub == "skill" && offered[r.Text] {
			continue
		}
		out = append(out, r)
	}
	return out
}
