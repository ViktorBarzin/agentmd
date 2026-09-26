// Package harness turns probe captures and static rules into runtime
// contexts, and fingerprints what could change a probe's result.
package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/probe"
)

// Env describes the machine, as for discovery.
type Env struct {
	Home            string
	Etc             string
	CodexHome       string
	ClaudeConfigDir string
	GOOS            string
	Roots           []string
	Config          config.Config
	Codex           discover.CodexSettings
}

// ManagedDir is where Claude Code reads org policy.
func (e Env) ManagedDir() string {
	if e.GOOS == "darwin" {
		return "/Library/Application Support/ClaudeCode"
	}
	return filepath.Join(e.Etc, "claude-code")
}

// ID is a context's identifier.
func ID(harness, dir string) string { return harness + ":" + dir }

// Dirs returns the directories that get contexts: home, every repository
// root and every directory holding a project instruction file, minus those
// matching a skip pattern.
func Dirs(home string, res *discover.Result, skip []string) []string {
	set := map[string]bool{home: true}
	for _, d := range res.Repos {
		set[d] = true
	}
	for _, d := range res.InstructionDirs {
		set[d] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		if !Skipped(d, skip) {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// Skipped reports whether dir matches a pattern: a glob for the whole path,
// or a prefix when the pattern ends in "/**".
func Skipped(dir string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "/**") && discover.Within(dir, strings.TrimSuffix(p, "/**")) {
			return true
		}
		if ok, _ := filepath.Match(p, dir); ok {
			return true
		}
	}
	return false
}

// RepoRoot returns the nearest ancestor of dir (or dir) holding .git.
func RepoRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			return ""
		}
	}
}

// chain lists the directories from root down to dir, inclusive. With an empty
// root it is just dir.
func chain(root, dir string) []string {
	if root == "" || !discover.Within(dir, root) {
		return []string{dir}
	}
	var out []string
	for d := dir; ; d = filepath.Dir(d) {
		out = append(out, d)
		if d == root || d == filepath.Dir(d) {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func newContext(env Env, harness, dir, source string) model.Context {
	return model.Context{
		ID: ID(harness, dir), Harness: harness, Dir: dir,
		Display: discover.Display(env.Home, dir), Source: source,
		Entries: []model.ContextEntry{}, Skills: []model.ContextItem{}, Subagents: []model.ContextItem{},
	}
}

func addEntry(c *model.Context, f *model.File, label string, bytes int) {
	c.Entries = append(c.Entries, model.ContextEntry{FileID: f.ID, Label: label, Bytes: bytes})
	c.Bytes += bytes
}

// AgentsMD builds the static context of the AGENTS.md standard: every
// AGENTS.md from the repository root down to dir. It reports false outside a
// repository.
func AgentsMD(env Env, res *discover.Result, dir string) (model.Context, bool) {
	root := RepoRoot(dir)
	if root == "" {
		return model.Context{}, false
	}
	c := newContext(env, model.HarnessAgentsMD, dir, "static")
	for _, d := range chain(root, dir) {
		if f, ok := res.Files[filepath.Join(d, "AGENTS.md")]; ok && !f.Missing {
			addEntry(&c, f, "", f.Size)
		}
	}
	return c, len(c.Entries) > 0
}

// Static builds a context for a harness declared in the config.
func Static(env Env, res *discover.Result, h config.StaticHarness, dir string) model.Context {
	c := newContext(env, h.Name, dir, "static")
	for _, p := range h.UserFiles {
		if f, ok := res.Files[p]; ok && !f.Missing {
			addEntry(&c, f, "", f.Size)
		}
	}
	for _, d := range chain(RepoRoot(dir), dir) {
		for _, name := range h.ProjectFiles {
			if f, ok := res.Files[filepath.Join(d, name)]; ok && !f.Missing {
				addEntry(&c, f, "", f.Size)
			}
		}
	}
	for _, sd := range h.SkillDirs {
		for _, f := range res.Sorted() {
			if f.Kind == model.KindSkill && discover.Within(f.Path, sd) {
				c.Skills = append(c.Skills, model.ContextItem{Name: f.Name, FileID: f.ID})
			}
		}
	}
	return c
}

// FromClaude maps a Claude Code capture to a context.
func FromClaude(env Env, res *discover.Result, dir, version string, cap *probe.ClaudeResult, at time.Time) model.Context {
	c := newContext(env, model.HarnessClaude, dir, "probe")
	c.Version, c.ProbedAt = version, &at
	loaded := map[string]bool{}
	for _, l := range cap.Files {
		path := l.Label
		switch {
		case l.Label == "<managed-settings>":
			path = filepath.Join(env.ManagedDir(), "managed-settings.json") + "#claudeMd"
		case cap.ScratchDir != "" && discover.Within(l.Label, cap.ScratchDir):
			path = filepath.Join(env.ClaudeConfigDir, strings.TrimPrefix(l.Label, cap.ScratchDir))
		}
		f := res.Files[path]
		if f == nil && !strings.HasPrefix(path, "<") && !strings.Contains(path, "#") {
			f = res.Add(path, model.KindInstruction, []string{model.HarnessClaude})
		}
		if f == nil {
			c.Entries = append(c.Entries, model.ContextEntry{Label: l.Label, Bytes: len(l.Content)})
			c.Bytes += len(l.Content)
			continue
		}
		loaded[f.ID] = true
		if f.IsLink {
			loaded[f.LinkTarget] = true
		}
		addEntry(&c, f, l.Label, len(l.Content))
	}
	for _, s := range cap.Skills {
		c.Skills = append(c.Skills, model.ContextItem{Name: s.Name, FileID: findItem(env, res, dir, model.KindSkill, s.Name)})
	}
	for _, s := range cap.Subagents {
		c.Subagents = append(c.Subagents, model.ContextItem{Name: s.Name, FileID: findItem(env, res, dir, model.KindSubagent, s.Name)})
	}
	c.Skipped = claudeSkipped(res, dir, loaded)
	return c
}

// claudeSkipped explains instruction files in dir that Claude Code did not load.
func claudeSkipped(res *discover.Result, dir string, loaded map[string]bool) []model.ContextEntry {
	var out []model.ContextEntry
	agents := res.Files[filepath.Join(dir, "AGENTS.md")]
	if agents == nil || agents.Missing || loaded[agents.ID] || loaded[agents.RealPath] {
		return nil
	}
	reason := "Claude Code reads CLAUDE.md, not AGENTS.md; a CLAUDE.md link to it would load it"
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "CLAUDE.md")); err == nil {
			if d == dir {
				reason = "this folder's CLAUDE.md is not a link to AGENTS.md, so AGENTS.md is skipped"
			} else {
				reason = "Claude Code skips AGENTS.md when a CLAUDE.md exists further up (" + filepath.Join(d, "CLAUDE.md") + ")"
			}
			break
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	out = append(out, model.ContextEntry{FileID: agents.ID, Bytes: agents.Size, Reason: reason})
	return out
}

// findItem finds the file behind a skill or subagent name, preferring the
// project's own, then the user's, then plugins.
func findItem(env Env, res *discover.Result, dir string, kind model.Kind, name string) string {
	short := name
	if i := strings.LastIndex(name, ":"); i >= 0 {
		short = name[i+1:]
	}
	best, bestRank := "", 99
	for _, f := range res.Files {
		if f.Kind != kind || (f.Name != name && f.Name != short) {
			continue
		}
		rank := 5
		switch {
		case discover.Within(f.Path, filepath.Join(dir, ".claude")):
			rank = 0
		case discover.Within(f.Path, env.ClaudeConfigDir) && !discover.Within(f.Path, filepath.Join(env.ClaudeConfigDir, "plugins")):
			rank = 1
		case discover.Within(f.Path, filepath.Join(env.ClaudeConfigDir, "plugins")):
			rank = 2
		case f.Scope == model.ScopeProject:
			rank = 3
		}
		if rank < bestRank || rank == bestRank && f.ID < best {
			best, bestRank = f.ID, rank
		}
	}
	return best
}

const projectDocSep = "\n\n--- project-doc ---\n\n"

// FromCodex maps a Codex capture to a context, attributing the joined
// instruction text to candidate files by matching their contents in order.
func FromCodex(env Env, res *discover.Result, dir, version string, cap *probe.CodexCapture, at time.Time) model.Context {
	c := newContext(env, model.HarnessCodex, dir, "probe")
	c.Version, c.ProbedAt = version, &at
	text := cap.Instructions
	pos := 0

	if user := codexUserFile(env, res); user != nil {
		want := strings.TrimSpace(user.Content)
		if want != "" && strings.HasPrefix(text, want) {
			addEntry(&c, user, "", len(want))
			pos = len(want)
			if strings.HasPrefix(text[pos:], projectDocSep) {
				pos += len(projectDocSep)
			}
		} else if want != "" {
			c.Skipped = append(c.Skipped, model.ContextEntry{FileID: user.ID, Bytes: user.Size, Reason: "its text is not in what Codex sent"})
		}
	}

	budgetGone := false
	for _, f := range codexProjectDocs(env, res, dir) {
		if budgetGone {
			c.Skipped = append(c.Skipped, model.ContextEntry{FileID: f.ID, Bytes: f.Size,
				Reason: fmt.Sprintf("Codex's %d-byte project-doc budget ran out before this file", env.Codex.MaxBytes)})
			continue
		}
		rest := text[pos:]
		content := f.Content
		switch {
		case content != "" && strings.HasPrefix(rest, content):
			addEntry(&c, f, "", len(content))
			pos += len(content)
		case strings.TrimRight(content, "\n") != "" && strings.HasPrefix(rest, strings.TrimRight(content, "\n")):
			n := len(strings.TrimRight(content, "\n"))
			addEntry(&c, f, "", n)
			pos += n
		case rest != "" && strings.HasPrefix(content, rest):
			addEntry(&c, f, "", len(rest))
			last := &c.Entries[len(c.Entries)-1]
			last.Truncated, last.LostBytes = true, len(content)-len(rest)
			pos = len(text)
			budgetGone = true
			continue
		case rest == "" && pos > 0:
			budgetGone = true
			c.Skipped = append(c.Skipped, model.ContextEntry{FileID: f.ID, Bytes: f.Size,
				Reason: fmt.Sprintf("Codex's %d-byte project-doc budget ran out before this file", env.Codex.MaxBytes)})
			continue
		default:
			c.Skipped = append(c.Skipped, model.ContextEntry{FileID: f.ID, Bytes: f.Size, Reason: "its text is not in what Codex sent"})
			continue
		}
		if strings.HasPrefix(text[pos:], "\n\n") {
			pos += 2
		}
	}

	if cap.OrgPolicy != "" {
		id := filepath.Join(env.Etc, "codex", "requirements.toml") + "#additional_developer_instructions"
		if f, ok := res.Files[id]; ok {
			addEntry(&c, f, "", len(cap.OrgPolicy))
		} else {
			c.Entries = append(c.Entries, model.ContextEntry{Label: "<managed_developer_instructions>", Bytes: len(cap.OrgPolicy)})
			c.Bytes += len(cap.OrgPolicy)
		}
	}
	for _, s := range cap.Skills {
		id := ""
		if s.Path != "" {
			if f, ok := res.Files[s.Path]; ok {
				id = f.ID
			} else if f := res.Add(s.Path, model.KindSkill, []string{model.HarnessCodex}); f != nil {
				id = f.ID
			}
		}
		c.Skills = append(c.Skills, model.ContextItem{Name: s.Name, FileID: id})
	}
	return c
}

func codexUserFile(env Env, res *discover.Result) *model.File {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		if f, ok := res.Files[filepath.Join(env.CodexHome, name)]; ok && !f.Missing && strings.TrimSpace(f.Content) != "" {
			return f
		}
	}
	return nil
}

// codexProjectDocs lists, from the git root down to dir, the one project doc
// Codex reads in each directory.
func codexProjectDocs(env Env, res *discover.Result, dir string) []*model.File {
	names := append([]string{"AGENTS.override.md", "AGENTS.md"}, env.Codex.FallbackFilenames...)
	var out []*model.File
	for _, d := range chain(RepoRoot(dir), dir) {
		for _, n := range names {
			p := filepath.Join(d, n)
			f, ok := res.Files[p]
			if !ok {
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					f = res.Add(p, model.KindInstruction, []string{model.HarnessCodex})
				}
			}
			if f != nil && !f.Missing && strings.TrimSpace(f.Content) != "" {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// ClaudeState holds the parts of Claude's global state file that change what
// a probe sees: per-folder trust and import approvals, and cached feature
// flags.
type ClaudeState struct {
	Projects map[string]string
	Flags    string
}

// LoadClaudeState reads the global state file. A missing file gives an empty
// state.
func LoadClaudeState(path string) ClaudeState {
	st := ClaudeState{Projects: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(data, &top) != nil {
		return st
	}
	var flagKeys []string
	for k := range top {
		lk := strings.ToLower(k)
		// Timestamps such as cachedGrowthBookFeaturesAt change on every
		// refresh without changing any flag, so they are left out.
		if strings.HasSuffix(k, "At") || strings.HasSuffix(lk, "time") || strings.HasSuffix(lk, "timestamp") {
			continue
		}
		if strings.Contains(lk, "growthbook") || strings.Contains(lk, "statsig") || strings.Contains(lk, "feature") || strings.Contains(lk, "gate") {
			flagKeys = append(flagKeys, k)
		}
	}
	sort.Strings(flagKeys)
	h := sha256.New()
	for _, k := range flagKeys {
		h.Write([]byte(k))
		h.Write(canonical(top[k]))
	}
	st.Flags = hex.EncodeToString(h.Sum(nil))
	var projects map[string]map[string]json.RawMessage
	if raw, ok := top["projects"]; ok && json.Unmarshal(raw, &projects) == nil {
		for dir, p := range projects {
			var parts []string
			for _, k := range []string{"hasTrustDialogAccepted", "hasClaudeMdExternalIncludesApproved", "hasClaudeMdExternalIncludesWarningShown", "enabledMcpjsonServers", "disabledMcpjsonServers"} {
				if v, ok := p[k]; ok {
					parts = append(parts, k+"="+string(v))
				}
			}
			st.Projects[dir] = strings.Join(parts, ";")
		}
	}
	return st
}

// canonical re-encodes JSON with sorted keys, so a rewrite that only
// reorders keys does not change the hash.
func canonical(raw json.RawMessage) []byte {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

// Stats caches file stats for one scan.
type Stats map[string]string

func (s Stats) of(p string) string {
	if v, ok := s[p]; ok {
		return v
	}
	v := "-"
	if st, err := os.Stat(p); err == nil {
		v = fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
	}
	s[p] = v
	return v
}

// Fingerprint hashes everything that could change a probe of harness in dir:
// the harness version, the org and user files, the settings files, every
// instruction file up the tree (present or not), the files the last probe
// loaded, and the relevant parts of Claude's global state.
func Fingerprint(env Env, res *discover.Result, stats Stats, harness, version, dir string, loaded []string, cs ClaudeState) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	add("harness " + harness)
	add("version " + version)
	add("dir " + dir)
	for _, f := range res.Files {
		real := f.RealPath
		if f.Field != "" {
			real = f.Path
		}
		switch {
		case f.Scope == model.ScopeOrg || f.Scope == model.ScopeUser || f.Scope == model.ScopePlugin:
		case discover.Within(dir, filepath.Dir(f.Path)) && f.Kind == model.KindInstruction:
		case discover.Within(f.Path, filepath.Join(dir, ".claude"), filepath.Join(dir, ".agents"), filepath.Join(dir, ".codex")):
		default:
			continue
		}
		add("file " + f.ID + " " + stats.of(real) + " " + f.LinkTarget)
	}
	for d := dir; ; d = filepath.Dir(d) {
		for _, n := range []string{"CLAUDE.md", "CLAUDE.local.md", ".claude/CLAUDE.md", "AGENTS.md", "AGENTS.override.md",
			".claude/settings.json", ".claude/settings.local.json", ".codex/config.toml"} {
			p := filepath.Join(d, n)
			add("up " + p + " " + stats.of(p))
		}
		if harness == model.HarnessClaude {
			if v, ok := cs.Projects[d]; ok {
				add("trust " + d + " " + v)
			}
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	for _, p := range []string{
		filepath.Join(env.ClaudeConfigDir, "settings.json"), filepath.Join(env.ClaudeConfigDir, "settings.local.json"),
		filepath.Join(env.ManagedDir(), "managed-settings.json"), filepath.Join(env.CodexHome, "config.toml"),
		filepath.Join(env.Etc, "codex", "requirements.toml"),
	} {
		add("settings " + p + " " + stats.of(p))
	}
	for _, p := range loaded {
		add("loaded " + p + " " + stats.of(p))
	}
	if harness == model.HarnessClaude {
		add("flags " + cs.Flags)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
}

// LoadedPaths lists the real paths a context loaded, for Fingerprint.
func LoadedPaths(res *discover.Result, c model.Context) []string {
	var out []string
	for _, e := range c.Entries {
		if f, ok := res.Files[e.FileID]; ok {
			if f.Field != "" {
				out = append(out, f.Path)
			} else {
				out = append(out, f.RealPath)
			}
		}
	}
	sort.Strings(out)
	return out
}
