// Package discover finds agent files: the known locations of each harness plus
// the project files under the discovery roots.
package discover

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// MaxFileBytes is the largest file agentmd reads. Larger files are listed
// without content.
const MaxFileBytes = 4 << 20

// DefaultExclude lists directory names the walk never enters. Hidden
// directories are skipped too, except the harness folders.
var DefaultExclude = []string{
	"node_modules", "vendor", "dist", "build", "target", "out", "venv",
	"__pycache__", "site-packages", "bower_components", "Pods",
}

// Env describes the machine being scanned. Tests point it at a temp tree.
type Env struct {
	Home      string
	Etc       string
	CodexHome string
	GOOS      string
	Roots     []string
	Exclude   []string
	Config    config.Config
	Codex     CodexSettings
}

// CodexSettings are the parts of Codex's config.toml that change what it loads.
type CodexSettings struct {
	FallbackFilenames []string
	MaxBytes          int
}

// DefaultCodexMaxBytes is Codex's project_doc_max_bytes default.
const DefaultCodexMaxBytes = 32768

// LoadCodexSettings reads $CODEX_HOME/config.toml. A missing or unreadable
// file gives the defaults.
func LoadCodexSettings(codexHome string) CodexSettings {
	s := CodexSettings{MaxBytes: DefaultCodexMaxBytes}
	var raw struct {
		ProjectDocMaxBytes          *int     `toml:"project_doc_max_bytes"`
		ProjectDocFallbackFilenames []string `toml:"project_doc_fallback_filenames"`
	}
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return s
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return s
	}
	if raw.ProjectDocMaxBytes != nil {
		s.MaxBytes = *raw.ProjectDocMaxBytes
	}
	s.FallbackFilenames = raw.ProjectDocFallbackFilenames
	return s
}

// Result is what a scan found.
type Result struct {
	Files map[string]*model.File
	// Repos are repository roots under the discovery roots.
	Repos []string
	// InstructionDirs hold at least one project instruction file.
	InstructionDirs []string

	s *scanner
}

// Add records a file found after the scan, such as a referenced doc or an
// imported file, with the same link handling as discovery. It returns nil
// when nothing exists at path.
func (r *Result) Add(path string, kind model.Kind, harnesses []string) *model.File {
	f := r.s.addFile(path, kind, harnesses)
	if f != nil && f.IsLink {
		r.s.inheritHarnesses()
	}
	return f
}

// Sorted returns the files ordered by display path.
func (r *Result) Sorted() []*model.File {
	out := make([]*model.File, 0, len(r.Files))
	for _, f := range r.Files {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type scanner struct {
	env      Env
	res      *Result
	owners   map[uint32]string
	repoSet  map[string]bool
	instrSet map[string]bool
	exclude  map[string]bool
	patterns []string
}

// Scan runs discovery.
func Scan(env Env) (*Result, error) {
	s := &scanner{
		env:      env,
		res:      &Result{Files: map[string]*model.File{}},
		owners:   map[uint32]string{},
		repoSet:  map[string]bool{},
		instrSet: map[string]bool{},
		exclude:  map[string]bool{},
	}
	s.res.s = s
	for _, e := range DefaultExclude {
		s.exclude[e] = true
	}
	for _, e := range env.Exclude {
		if strings.ContainsAny(e, "/*?[") {
			s.patterns = append(s.patterns, e)
		} else {
			s.exclude[e] = true
		}
	}
	s.harnessLocations()
	for _, root := range env.Roots {
		if err := s.walkRoot(root); err != nil {
			return nil, err
		}
	}
	s.inheritHarnesses()
	for d := range s.repoSet {
		s.res.Repos = append(s.res.Repos, d)
	}
	for d := range s.instrSet {
		s.res.InstructionDirs = append(s.res.InstructionDirs, d)
	}
	sort.Strings(s.res.Repos)
	sort.Strings(s.res.InstructionDirs)
	return s.res, nil
}

// managedDir is where Claude Code reads org policy on this OS.
func (s *scanner) managedDir() string {
	if s.env.GOOS == "darwin" {
		return "/Library/Application Support/ClaudeCode"
	}
	return filepath.Join(s.env.Etc, "claude-code")
}

func (s *scanner) harnessLocations() {
	home := s.env.Home
	claude := []string{model.HarnessClaude}
	codex := []string{model.HarnessCodex}

	md := s.managedDir()
	s.addFile(filepath.Join(md, "CLAUDE.md"), model.KindInstruction, claude)
	s.addEmbedded(filepath.Join(md, "managed-settings.json"), "claudeMd", claude)
	s.addTree(filepath.Join(md, ".claude", "rules"), model.KindInstruction, claude)
	s.addSkills(filepath.Join(md, ".claude", "skills"), claude)

	ch := filepath.Join(home, ".claude")
	s.addFile(filepath.Join(ch, "CLAUDE.md"), model.KindInstruction, claude)
	s.addTree(filepath.Join(ch, "rules"), model.KindInstruction, claude)
	s.addSkills(filepath.Join(ch, "skills"), claude)
	s.addTree(filepath.Join(ch, "agents"), model.KindSubagent, claude)
	s.addTree(filepath.Join(ch, "commands"), model.KindCommand, claude)
	s.addPlugins(filepath.Join(ch, "plugins"))

	s.addEmbedded(filepath.Join(s.env.Etc, "codex", "requirements.toml"), "additional_developer_instructions", codex)
	s.addFile(filepath.Join(s.env.CodexHome, "AGENTS.override.md"), model.KindInstruction, codex)
	s.addFile(filepath.Join(s.env.CodexHome, "AGENTS.md"), model.KindInstruction, codex)
	s.addTree(filepath.Join(s.env.CodexHome, "prompts"), model.KindCommand, codex)
	s.addSkillTree(filepath.Join(s.env.CodexHome, "skills"), codex)
	s.addSkills(filepath.Join(home, ".agents", "skills"), codex)

	for _, h := range s.env.Config.Harnesses {
		hs := []string{h.Name}
		for _, f := range h.UserFiles {
			s.addFile(f, model.KindInstruction, hs)
		}
		for _, d := range h.SkillDirs {
			s.addSkills(d, hs)
		}
	}
	for _, b := range s.env.Config.Builds {
		s.addFile(b.Output, model.KindInstruction, nil)
		for _, p := range b.Parts {
			s.addFile(p, model.KindInstruction, nil)
		}
	}
	for _, o := range s.env.Config.Origins {
		if src := FirstExisting(o.Source); src != "" {
			if o.Field != "" {
				s.addEmbedded(src, o.Field, nil)
			} else {
				s.addFile(src, model.KindInstruction, nil)
			}
		}
	}
}

// FirstExisting returns the first path that exists, or "".
func FirstExisting(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// addTree adds every .md file under dir, following a symlinked dir itself.
func (s *scanner) addTree(dir string, kind model.Kind, harnesses []string) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".md") {
			s.addFile(p, kind, harnesses)
		}
		return nil
	})
}

// addSkills adds <dir>/<name>/SKILL.md for each entry, following symlinked
// skill directories.
func (s *scanner) addSkills(dir string, harnesses []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(p); err == nil {
			s.addFile(p, model.KindSkill, harnesses)
		} else if e.Type()&fs.ModeSymlink != 0 {
			// A skill link whose target is gone: keep the link visible.
			s.addFile(filepath.Join(dir, e.Name()), model.KindSkill, harnesses)
		}
	}
}

// addSkillTree finds SKILL.md at any depth, including hidden system folders.
func (s *scanner) addSkillTree(dir string, harnesses []string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			s.addFile(p, model.KindSkill, harnesses)
		}
		return nil
	})
}

// addPlugins finds plugin skills, subagents and commands.
func (s *scanner) addPlugins(dir string) {
	claude := []string{model.HarnessClaude}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "node_modules" || name == ".git" || name == "cache" && filepath.Dir(p) == dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		parent := filepath.Base(filepath.Dir(p))
		switch {
		case name == "SKILL.md" && filepath.Base(filepath.Dir(filepath.Dir(p))) == "skills":
			s.addFile(p, model.KindSkill, claude)
		case parent == "agents":
			s.addFile(p, model.KindSubagent, claude)
		case parent == "commands":
			s.addFile(p, model.KindCommand, claude)
		}
		return nil
	})
}

func (s *scanner) skipDir(p, name string) bool {
	if s.exclude[name] {
		return true
	}
	if strings.HasPrefix(name, ".") {
		switch name {
		case ".claude", ".agents", ".codex":
			return false
		}
		return true
	}
	for _, pat := range s.patterns {
		if ok, _ := filepath.Match(pat, p); ok {
			return true
		}
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}

func (s *scanner) walkRoot(root string) error {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && s.skipDir(p, name) {
				return filepath.SkipDir
			}
			if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
				s.repoSet[p] = true
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				s.linkedDir(p)
				return nil
			}
		}
		kind, hs, ok := s.classify(p)
		if !ok {
			return nil
		}
		s.addFile(p, kind, hs)
		if kind == model.KindInstruction {
			s.instrSet[instructionDir(p)] = true
		}
		return nil
	})
}

// linkedDir handles a symlinked directory met during the walk. Only symlinked
// skill directories matter; the walk does not follow other directory links.
func (s *scanner) linkedDir(p string) {
	parent := filepath.Dir(p)
	if filepath.Base(parent) != "skills" {
		return
	}
	switch filepath.Base(filepath.Dir(parent)) {
	case ".claude":
		s.addSkillIn(p, []string{model.HarnessClaude})
	case ".agents":
		s.addSkillIn(p, []string{model.HarnessCodex})
	}
}

func (s *scanner) addSkillIn(dir string, hs []string) {
	skill := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skill); err == nil {
		s.addFile(skill, model.KindSkill, hs)
	}
}

// instructionDir is the directory an instruction file belongs to: the parent
// of .claude for .claude/CLAUDE.md and rules.
func instructionDir(p string) string {
	d := filepath.Dir(p)
	for {
		base := filepath.Base(d)
		if base == ".claude" {
			return filepath.Dir(d)
		}
		if base == "rules" && filepath.Base(filepath.Dir(d)) == ".claude" {
			return filepath.Dir(filepath.Dir(d))
		}
		if base != "rules" {
			return d
		}
		d = filepath.Dir(d)
	}
}

// classify decides whether a project file is an agent file and for whom.
func (s *scanner) classify(p string) (model.Kind, []string, bool) {
	name := filepath.Base(p)
	claude := []string{model.HarnessClaude}
	if i := strings.LastIndex(p, string(filepath.Separator)+".claude"+string(filepath.Separator)); i >= 0 {
		rem := filepath.ToSlash(p[i+len("/.claude/"):])
		switch {
		case rem == "CLAUDE.md":
			return model.KindInstruction, claude, true
		case strings.HasPrefix(rem, "rules/") && strings.HasSuffix(rem, ".md"):
			return model.KindInstruction, claude, true
		case strings.HasPrefix(rem, "skills/") && name == "SKILL.md" && strings.Count(rem, "/") == 2:
			return model.KindSkill, claude, true
		case strings.HasPrefix(rem, "agents/") && strings.HasSuffix(rem, ".md"):
			return model.KindSubagent, claude, true
		case strings.HasPrefix(rem, "commands/") && strings.HasSuffix(rem, ".md"):
			return model.KindCommand, claude, true
		}
		return "", nil, false
	}
	if i := strings.LastIndex(p, string(filepath.Separator)+".agents"+string(filepath.Separator)); i >= 0 {
		rem := filepath.ToSlash(p[i+len("/.agents/"):])
		if strings.HasPrefix(rem, "skills/") && name == "SKILL.md" && strings.Count(rem, "/") == 2 {
			return model.KindSkill, []string{model.HarnessCodex}, true
		}
		return "", nil, false
	}
	var hs []string
	switch name {
	case "AGENTS.md":
		hs = []string{model.HarnessCodex, model.HarnessAgentsMD}
	case "AGENTS.override.md":
		hs = []string{model.HarnessCodex}
	case "CLAUDE.md", "CLAUDE.local.md":
		hs = claude
	}
	for _, fb := range s.env.Codex.FallbackFilenames {
		if name == fb && !contains(hs, model.HarnessCodex) {
			hs = append(hs, model.HarnessCodex)
		}
	}
	for _, h := range s.env.Config.Harnesses {
		for _, pf := range h.ProjectFiles {
			if name == pf && !contains(hs, h.Name) {
				hs = append(hs, h.Name)
			}
		}
	}
	if len(hs) == 0 {
		return "", nil, false
	}
	return model.KindInstruction, hs, true
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// addFile records a file and, when it is reached through a symlink, its
// target. It returns the file, or nil when nothing exists at p.
func (s *scanner) addFile(p string, kind model.Kind, harnesses []string) *model.File {
	p = filepath.Clean(p)
	if f, ok := s.res.Files[p]; ok {
		f.Harnesses = union(f.Harnesses, harnesses)
		return f
	}
	lst, err := os.Lstat(p)
	if err != nil {
		return nil
	}
	f := &model.File{
		ID:        p,
		Path:      p,
		RealPath:  p,
		Display:   Display(s.env.Home, p),
		Kind:      kind,
		Harnesses: union(nil, harnesses),
	}
	s.res.Files[p] = f
	f.Scope = s.scopeOf(p)
	f.Repo = s.repoOf(p)

	if lst.Mode()&fs.ModeSymlink != 0 {
		f.IsLink = true
		target, err := os.Readlink(p)
		if err == nil && !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(p), target)
		}
		f.LinkTarget = filepath.Clean(target)
		st, err := os.Stat(p)
		if err != nil {
			f.Missing = true
			return f
		}
		if st.IsDir() {
			// A dangling skill link that points at a directory without SKILL.md.
			f.Missing = true
			return f
		}
		real, _ := filepath.EvalSymlinks(p)
		f.RealPath = real
		t := s.addFile(f.LinkTarget, kind, harnesses)
		if t != nil {
			s.copyFrom(f, t)
		}
		return f
	}
	if real, err := filepath.EvalSymlinks(p); err == nil && real != p {
		// Reached through a symlinked parent directory.
		f.IsLink = true
		f.LinkTarget = real
		f.RealPath = real
		if t := s.addFile(real, kind, harnesses); t != nil {
			s.copyFrom(f, t)
		}
		return f
	}
	s.load(f, lst)
	return f
}

// copyFrom gives a link the content facts of what it points at.
func (s *scanner) copyFrom(link, target *model.File) {
	link.Size, link.Lines, link.Hash = target.Size, target.Lines, target.Hash
	link.Content = target.Content
	link.Name, link.Description = target.Name, target.Description
	link.Access = target.Access
	link.RealPath = target.RealPath
}

func (s *scanner) load(f *model.File, st fs.FileInfo) {
	f.Access = s.access(f.RealPath, st)
	if st.Size() > MaxFileBytes {
		f.Size = int(st.Size())
		return
	}
	data, err := os.ReadFile(f.RealPath)
	if err != nil {
		f.Access.Writable = false
		f.Access.Reason = "not readable"
		return
	}
	setContent(f, string(data))
	if f.Kind == model.KindSkill || f.Kind == model.KindSubagent || f.Kind == model.KindCommand {
		fm := ParseFrontmatter(f.Content)
		f.Name, f.Description = fm["name"], fm["description"]
		if f.Name == "" && f.Kind == model.KindSkill {
			f.Name = filepath.Base(filepath.Dir(f.Path))
		}
		if f.Name == "" {
			f.Name = strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
		}
	}
}

func setContent(f *model.File, content string) {
	f.Content = content
	f.Size = len(content)
	f.Lines = CountLines(content)
	f.Hash = Hash(content)
}

// Hash is the content hash agentmd uses everywhere.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// CountLines counts lines the way an editor shows them.
func CountLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// addEmbedded records one string field of a JSON or TOML settings file as an
// embedded file.
func (s *scanner) addEmbedded(path, field string, harnesses []string) *model.File {
	id := path + "#" + field
	if f, ok := s.res.Files[id]; ok {
		f.Harnesses = union(f.Harnesses, harnesses)
		return f
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text, ok := EmbeddedField(path, data, field)
	if !ok {
		return nil
	}
	real, _ := filepath.EvalSymlinks(path)
	if real == "" {
		real = path
	}
	f := &model.File{
		ID:        id,
		Path:      path,
		RealPath:  real,
		Display:   Display(s.env.Home, path) + "#" + field,
		Field:     field,
		Kind:      model.KindInstruction,
		Harnesses: union(nil, harnesses),
	}
	f.Scope = s.scopeOf(path)
	f.Repo = s.repoOf(path)
	f.Access = s.access(real, st)
	if strings.HasSuffix(path, ".toml") && f.Access.Writable {
		f.Access.Writable = false
		f.Access.Reason = "editing a TOML field is not supported; edit the file itself"
	}
	setContent(f, text)
	s.res.Files[id] = f
	return f
}

// EmbeddedField extracts a top-level string field from JSON or TOML.
func EmbeddedField(path string, data []byte, field string) (string, bool) {
	if strings.HasSuffix(path, ".toml") {
		var m map[string]any
		if _, err := toml.Decode(string(data), &m); err != nil {
			return "", false
		}
		v, ok := m[field].(string)
		return v, ok && v != ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &m); err != nil {
		return "", false
	}
	raw, ok := m[field]
	if !ok {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, v != ""
}

func (s *scanner) access(real string, st fs.FileInfo) model.Access {
	a := model.Access{Writable: true}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		a.Owner = s.ownerName(sys.Uid)
	}
	if err := syscall.Access(real, 2); err != nil {
		a.Writable = false
		if a.Owner == "root" {
			a.Reason = "owned by root"
		} else if a.Owner != "" {
			a.Reason = "not writable (owned by " + a.Owner + ")"
		} else {
			a.Reason = "not writable"
		}
	}
	return a
}

func (s *scanner) ownerName(uid uint32) string {
	if n, ok := s.owners[uid]; ok {
		return n
	}
	name := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	s.owners[uid] = name
	return name
}

func (s *scanner) scopeOf(p string) model.Scope {
	home := s.env.Home
	switch {
	case within(p, s.env.Etc), within(p, s.managedDir()):
		return model.ScopeOrg
	case within(p, filepath.Join(home, ".claude", "plugins")):
		return model.ScopePlugin
	case within(p, filepath.Join(home, ".claude")), within(p, s.env.CodexHome),
		within(p, filepath.Join(home, ".agents")), within(p, filepath.Join(home, ".pi")),
		within(p, filepath.Join(home, ".config")):
		return model.ScopeUser
	}
	for _, r := range s.env.Roots {
		if within(p, r) {
			return model.ScopeProject
		}
	}
	return model.ScopeOther
}

// repoOf returns the nearest ancestor holding a .git entry, if any.
func (s *scanner) repoOf(p string) string {
	d := filepath.Dir(p)
	for {
		if s.repoSet[d] {
			return d
		}
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			s.repoSet[d] = within(d, s.env.Roots...)
			if !s.repoSet[d] {
				delete(s.repoSet, d)
			}
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// inheritHarnesses gives a link target the harnesses of every link to it.
func (s *scanner) inheritHarnesses() {
	for changed := true; changed; {
		changed = false
		for _, f := range s.res.Files {
			if !f.IsLink {
				continue
			}
			t, ok := s.res.Files[f.LinkTarget]
			if !ok {
				continue
			}
			u := union(t.Harnesses, f.Harnesses)
			if len(u) != len(t.Harnesses) {
				t.Harnesses = u
				changed = true
			}
		}
	}
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, x := range b {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// within reports whether p is dir or inside one of dirs.
func within(p string, dirs ...string) bool {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if p == d || strings.HasPrefix(p, strings.TrimSuffix(d, string(filepath.Separator))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Within is the exported form of within for other packages.
func Within(p string, dirs ...string) bool { return within(p, dirs...) }

// Display shortens a path under home to "~/...".
func Display(home, p string) string {
	if home != "" && within(p, home) {
		if p == home {
			return "~"
		}
		return "~" + p[len(strings.TrimSuffix(home, "/")):]
	}
	return p
}

// ParseFrontmatter reads simple "key: value" pairs from YAML front matter,
// including folded (">") and literal ("|") blocks.
func ParseFrontmatter(content string) map[string]string {
	out := map[string]string{}
	if !strings.HasPrefix(content, "---") {
		return out
	}
	lines := strings.Split(content, "\n")
	var key string
	var block []string
	flush := func() {
		if key != "" && block != nil {
			out[key] = strings.TrimSpace(strings.Join(block, " "))
		}
		key, block = "", nil
	}
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "---" {
			break
		}
		if block != nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || line == "") {
			block = append(block, strings.TrimSpace(line))
			continue
		}
		flush()
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(k, " ") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch v {
		case ">", "|", ">-", "|-", ">+", "|+":
			key, block = k, []string{}
			continue
		}
		out[k] = unquote(v)
	}
	flush()
	return out
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		if v[0] == '"' {
			if u, err := strconv.Unquote(v); err == nil {
				return u
			}
		}
		return v[1 : len(v)-1]
	}
	return v
}

// ErrNoHome is returned when the owner's home directory is unknown.
var ErrNoHome = errors.New("home directory unknown")
