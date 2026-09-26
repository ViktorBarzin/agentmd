// Package refs finds the references between agent files: symlinks, build
// joins (build headers, config entries, installed copies, Claude imports) and
// text mentions. Load-order references come from runtime contexts instead.
package refs

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ViktorBarzin/agentmd/internal/config"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// Input is what Build needs.
type Input struct {
	Res    *discover.Result
	Home   string
	Roots  []string
	Config config.Config
}

// MaxImportDepth matches Claude Code's limit on nested imports.
const MaxImportDepth = 5

type builder struct {
	in     Input
	refs   []model.Ref
	seen   map[string]bool
	skills map[string][]*model.File
}

// Build returns every symlink, build and mention reference, adding imported
// files and referenced docs to in.Res as it goes.
func Build(in Input) []model.Ref {
	b := &builder{in: in, seen: map[string]bool{}}
	b.symlinks()
	b.imports()
	b.headers()
	b.configBuilds()
	b.origins()
	b.indexSkills()
	b.mentions(false)
	b.mentions(true)
	b.propagateAccess()
	sort.SliceStable(b.refs, func(i, j int) bool {
		if b.refs[i].From != b.refs[j].From {
			return b.refs[i].From < b.refs[j].From
		}
		return b.refs[i].Line < b.refs[j].Line
	})
	return b.refs
}

func (b *builder) add(r model.Ref) {
	if r.From == r.To {
		return
	}
	k := r.From + "\x00" + r.To + "\x00" + string(r.Kind) + "\x00" + r.Sub
	if b.seen[k] {
		return
	}
	b.seen[k] = true
	b.refs = append(b.refs, r)
}

// sorted returns the current files in a stable order.
func (b *builder) sorted() []*model.File { return b.in.Res.Sorted() }

func (b *builder) symlinks() {
	for _, f := range b.sorted() {
		if !f.IsLink {
			continue
		}
		sub := "dir"
		if st, err := os.Lstat(f.Path); err == nil && st.Mode()&os.ModeSymlink != 0 {
			sub = "link"
		}
		_, exists := b.in.Res.Files[f.LinkTarget]
		b.add(model.Ref{From: f.ID, To: f.LinkTarget, Kind: model.RefSymlink, Sub: sub, Dangling: f.Missing || !exists})
	}
}

var importRe = regexp.MustCompile(`(^|[\s(])@([~./A-Za-z0-9_-][^\s()<>\x60]*)`)

// imports follows Claude @path imports from Claude instruction files.
func (b *builder) imports() {
	type item struct {
		f     *model.File
		depth int
	}
	var queue []item
	for _, f := range b.sorted() {
		if f.IsLink || f.Kind != model.KindInstruction || !has(f.Harnesses, model.HarnessClaude) {
			continue
		}
		queue = append(queue, item{f, 1})
	}
	done := map[string]bool{}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if done[it.f.ID] || it.depth > MaxImportDepth {
			continue
		}
		done[it.f.ID] = true
		for _, m := range scanOutsideCode(it.f.Content, importRe) {
			token := trimToken(m.text)
			if token == "" || !(strings.Contains(token, "/") || hasMarkdownExt(token)) {
				continue
			}
			target := b.resolveImport(it.f, token)
			if target == "" {
				continue
			}
			if st, err := os.Stat(target); err != nil || st.IsDir() {
				if err != nil && looksLikeImport(token) {
					b.add(model.Ref{From: it.f.ID, To: target, Kind: model.RefBuild, Sub: "import", Line: m.line, Text: "@" + token, Dangling: true})
				}
				continue
			}
			t := b.in.Res.Add(target, model.KindInstruction, []string{model.HarnessClaude})
			if t == nil {
				continue
			}
			b.add(model.Ref{From: it.f.ID, To: t.ID, Kind: model.RefBuild, Sub: "import", Line: m.line, Text: "@" + token})
			real := t
			if t.IsLink {
				if r, ok := b.in.Res.Files[t.LinkTarget]; ok {
					real = r
				}
			}
			queue = append(queue, item{real, it.depth + 1})
		}
	}
}

// looksLikeImport tells an import that went missing from a package name or a
// handle such as @xyflow/react: it names a markdown file or starts like a path.
func looksLikeImport(token string) bool {
	if hasMarkdownExt(token) {
		return true
	}
	for _, p := range []string{"./", "../", "~/", "/"} {
		if strings.HasPrefix(token, p) {
			return true
		}
	}
	return false
}

func (b *builder) resolveImport(f *model.File, token string) string {
	switch {
	case strings.HasPrefix(token, "~/"):
		return filepath.Join(b.in.Home, token[2:])
	case filepath.IsAbs(token):
		return filepath.Clean(token)
	}
	return filepath.Join(filepath.Dir(f.Path), token)
}

var headerRe = regexp.MustCompile(`(?i)<!--\s*(?:built|generated|assembled)\s+(?:by\s+(\S+)\s+)?from\s+(.+?)\s*(?:[;.]\s*(?:edit|do not|don't)[^>]*)?-->`)

// headers reads "built from" comments at the top of instruction files.
func (b *builder) headers() {
	for _, f := range b.sorted() {
		if f.IsLink || f.Kind != model.KindInstruction || f.Content == "" {
			continue
		}
		head := f.Content
		if lines := strings.SplitN(head, "\n", 6); len(lines) > 5 {
			head = strings.Join(lines[:5], "\n")
		}
		m := headerRe.FindStringSubmatch(head)
		if m == nil {
			continue
		}
		var parts []string
		for _, p := range splitParts(m[2]) {
			parts = append(parts, b.resolvePart(f, p))
		}
		b.markBuilt(f, parts, m[1], "header")
	}
}

func (b *builder) configBuilds() {
	for _, rule := range b.in.Config.Builds {
		f, ok := b.in.Res.Files[rule.Output]
		if !ok {
			continue
		}
		if f.IsLink {
			if t, ok := b.in.Res.Files[f.LinkTarget]; ok {
				f = t
			}
		}
		b.markBuilt(f, rule.Parts, rule.Command, "config")
	}
}

func (b *builder) markBuilt(f *model.File, parts []string, by, sub string) {
	var ids []string
	for _, p := range parts {
		t := b.in.Res.Add(p, model.KindInstruction, nil)
		if t == nil {
			b.add(model.Ref{From: f.ID, To: p, Kind: model.RefBuild, Sub: sub, Dangling: true})
			continue
		}
		ids = append(ids, t.ID)
		b.add(model.Ref{From: f.ID, To: t.ID, Kind: model.RefBuild, Sub: sub})
	}
	f.Access.BuiltFrom = ids
	f.Access.BuiltBy = by
	f.Access.Writable = false
	if by != "" {
		f.Access.Reason = "built by " + by + " from its parts; edit the parts"
	} else {
		f.Access.Reason = "built from its parts; edit the parts"
	}
}

func (b *builder) resolvePart(f *model.File, p string) string {
	switch {
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(b.in.Home, p[2:])
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	}
	return filepath.Join(filepath.Dir(f.Path), p)
}

// splitParts splits "a.md, b.md and ~/x/{c,d}.md" into paths, expanding braces.
func splitParts(s string) []string {
	var out []string
	for _, tok := range strings.Fields(s) {
		tok = strings.Trim(tok, ",;")
		if tok == "" || strings.EqualFold(tok, "and") || strings.EqualFold(tok, "plus") {
			continue
		}
		out = append(out, ExpandBraces(tok)...)
	}
	return out
}

// ExpandBraces expands shell-style {a,b} groups.
func ExpandBraces(s string) []string {
	open := strings.Index(s, "{")
	if open < 0 {
		return []string{s}
	}
	close := strings.Index(s[open:], "}")
	if close < 0 {
		return []string{s}
	}
	close += open
	var out []string
	for _, alt := range strings.Split(s[open+1:close], ",") {
		out = append(out, ExpandBraces(s[:open]+alt+s[close+1:])...)
	}
	return out
}

func (b *builder) origins() {
	for _, rule := range b.in.Config.Origins {
		copyID := rule.Path
		if rule.Field != "" {
			copyID += "#" + rule.Field
		}
		c, ok := b.in.Res.Files[copyID]
		if !ok {
			continue
		}
		src := discover.FirstExisting(rule.Source)
		if src == "" {
			continue
		}
		srcID := src
		if f := rule.SourceFieldName(); f != "" {
			srcID += "#" + f
		}
		if _, ok := b.in.Res.Files[srcID]; !ok {
			if rule.SourceFieldName() != "" {
				continue
			}
			if b.in.Res.Add(src, model.KindInstruction, nil) == nil {
				continue
			}
		}
		b.add(model.Ref{From: copyID, To: srcID, Kind: model.RefBuild, Sub: "origin"})
		c.Access.Origin = &model.Origin{FileID: srcID, Path: src, Field: rule.SourceFieldName(), Note: rule.Note}
	}
}

func (b *builder) indexSkills() {
	b.skills = map[string][]*model.File{}
	for _, f := range b.sorted() {
		if f.Kind != model.KindSkill || f.Name == "" {
			continue
		}
		b.skills[f.Name] = append(b.skills[f.Name], f)
	}
}

// skill finds the file for a skill name, preferring a real file over a link.
func (b *builder) skill(name string) *model.File {
	cands := b.skills[name]
	if len(cands) == 0 {
		if i := strings.LastIndex(name, ":"); i >= 0 {
			cands = b.skills[name[i+1:]]
		}
	}
	for _, f := range cands {
		if !f.IsLink {
			return f
		}
	}
	if len(cands) > 0 {
		return cands[0]
	}
	return nil
}

var (
	linkRe      = regexp.MustCompile(`\[[^\]\n]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	codeSpanRe  = regexp.MustCompile("`([^`\n]+)`")
	bareRe      = regexp.MustCompile("(^|[\\s(])([^\\s()\\[\\]\"'`]*/[^\\s()\\[\\]\"'`]*)")
	skillAfter  = regexp.MustCompile("(?i)`([a-z0-9][a-z0-9:_-]*)`\\s+skill")
	skillBefore = regexp.MustCompile("(?i)skill\\s+`([a-z0-9][a-z0-9:_-]*)`")
	slashRe     = regexp.MustCompile(`(^|\s)/([a-z0-9][a-z0-9-]*)($|[\s.,;:!?)])`)
)

// mentions scans text for paths and skill names. With docsOnly false it scans
// agent files and may add referenced docs; with docsOnly true it scans those
// docs and only links to files already known.
func (b *builder) mentions(docsOnly bool) {
	for _, f := range b.sorted() {
		if f.IsLink || f.Content == "" || (f.Kind == model.KindDoc) != docsOnly {
			continue
		}
		b.scanFile(f, !docsOnly)
	}
}

func (b *builder) scanFile(f *model.File, addDocs bool) {
	lines := strings.Split(f.Content, "\n")
	inFence := false
	var fence string
	for i, line := range lines {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence = false
			}
			continue
		}
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			b.pathMention(f, m[1], "link", n, inFence, addDocs, true)
		}
		for _, m := range codeSpanRe.FindAllStringSubmatch(line, -1) {
			b.pathMention(f, m[1], "path", n, inFence, addDocs, true)
		}
		stripped := codeSpanRe.ReplaceAllString(linkRe.ReplaceAllString(line, " "), " ")
		for _, m := range bareRe.FindAllStringSubmatch(stripped, -1) {
			b.pathMention(f, m[2], "path", n, inFence, addDocs, false)
		}
		if inFence {
			continue
		}
		for _, re := range []*regexp.Regexp{skillAfter, skillBefore} {
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				b.skillMention(f, m[1], n, true)
			}
		}
		for _, m := range slashRe.FindAllStringSubmatch(stripped, -1) {
			b.skillMention(f, m[2], n, false)
		}
	}
}

func (b *builder) skillMention(f *model.File, name string, line int, flagMissing bool) {
	s := b.skill(name)
	if s == nil {
		if flagMissing && !upstream(f) {
			b.add(model.Ref{From: f.ID, To: "skill:" + name, Kind: model.RefMention, Sub: "skill", Line: line, Text: name, Dangling: true})
		}
		return
	}
	if s.RealPath == f.RealPath {
		return
	}
	b.add(model.Ref{From: f.ID, To: s.ID, Kind: model.RefMention, Sub: "skill", Line: line, Text: name})
}

// pathMention resolves one path-like token. allowBare lets a token without a
// slash count (inside links and code spans only).
func (b *builder) pathMention(f *model.File, raw, sub string, line int, inFence, addDocs, allowBare bool) {
	token := trimToken(raw)
	if token == "" || !hasMarkdownExt(token) {
		return
	}
	if !allowBare && !strings.Contains(token, "/") {
		return
	}
	cands := b.candidates(f, token)
	for _, c := range cands {
		st, err := os.Stat(c)
		if err != nil || st.IsDir() {
			continue
		}
		t, ok := b.in.Res.Files[c]
		if !ok {
			if !addDocs {
				return
			}
			t = b.in.Res.Add(c, model.KindDoc, nil)
			if t == nil {
				return
			}
		}
		if t.RealPath == f.RealPath {
			return
		}
		b.add(model.Ref{From: f.ID, To: t.ID, Kind: model.RefMention, Sub: sub, Line: line, Text: token})
		return
	}
	if inFence || !addDocs || upstream(f) || !strings.Contains(token, "/") {
		return
	}
	// Only a path whose folder exists near the file counts as broken; the
	// home fallback would make almost any ".claude/x.md" look broken.
	for _, c := range b.danglingCandidates(f, token) {
		if st, err := os.Stat(filepath.Dir(c)); err == nil && st.IsDir() {
			b.add(model.Ref{From: f.ID, To: c, Kind: model.RefMention, Sub: sub, Line: line, Text: token, Dangling: true})
			return
		}
	}
}

// upstream reports a file someone else maintains, whose broken links are
// theirs to fix: plugin files and installed copies that get replaced.
func upstream(f *model.File) bool {
	return f.Scope == model.ScopePlugin || (f.Access.Managed != nil && f.Access.Managed.Warn)
}

// danglingCandidates are the places a broken path most likely meant: the
// path as written when absolute or under ~ (only inside home or a discovery
// root), else next to the file or at the root of its repository.
func (b *builder) danglingCandidates(f *model.File, token string) []string {
	switch {
	case strings.HasPrefix(token, "~/"):
		return []string{filepath.Join(b.in.Home, token[2:])}
	case filepath.IsAbs(token):
		p := filepath.Clean(token)
		if !discover.Within(p, append([]string{b.in.Home}, b.in.Roots...)...) {
			return nil
		}
		return []string{p}
	}
	out := []string{filepath.Join(filepath.Dir(f.Path), token)}
	if f.Repo != "" {
		out = append(out, filepath.Join(f.Repo, token))
	}
	return out
}

// candidates lists where a mentioned path might live, most specific first.
func (b *builder) candidates(f *model.File, token string) []string {
	switch {
	case strings.HasPrefix(token, "~/"):
		return []string{filepath.Join(b.in.Home, token[2:])}
	case filepath.IsAbs(token):
		return []string{filepath.Clean(token)}
	}
	var out []string
	seen := map[string]bool{}
	push := func(base string) {
		if base == "" {
			return
		}
		p := filepath.Join(base, token)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	dir := filepath.Dir(f.Path)
	push(dir)
	push(f.Repo)
	limits := append([]string{b.in.Home}, b.in.Roots...)
	for d := filepath.Dir(dir); discover.Within(d, limits...); d = filepath.Dir(d) {
		push(d)
		if d == filepath.Dir(d) {
			break
		}
	}
	for _, r := range b.in.Roots {
		push(r)
	}
	push(b.in.Home)
	return out
}

func hasMarkdownExt(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".mdx", ".markdown":
		return true
	}
	return false
}

// trimToken strips punctuation, anchors and queries, and rejects URLs and
// placeholders.
func trimToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".,;:!?)]'\"")
	s = strings.TrimLeft(s, "(['\"")
	if s == "" || strings.Contains(s, "://") || strings.HasPrefix(s, "mailto:") || strings.HasPrefix(s, "#") {
		return ""
	}
	if i := strings.IndexAny(s, "#?"); i >= 0 {
		s = s[:i]
	}
	if strings.ContainsAny(s, "<>{}*$…|") || strings.Contains(s, "...") || strings.Contains(s, "YYYY") {
		return ""
	}
	return s
}

type match struct {
	text string
	line int
}

// scanOutsideCode returns the second capture group of re for every match
// outside fenced blocks and inline code spans.
func scanOutsideCode(content string, re *regexp.Regexp) []match {
	var out []match
	inFence := false
	var fence string
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		line = codeSpanRe.ReplaceAllString(line, " ")
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			out = append(out, match{text: m[2], line: i + 1})
		}
	}
	return out
}

// propagateAccess gives each link the access facts of its target, set above
// for built files and installed copies.
func (b *builder) propagateAccess() {
	for _, f := range b.sorted() {
		if !f.IsLink {
			continue
		}
		t, ok := b.in.Res.Files[f.LinkTarget]
		if !ok {
			continue
		}
		for t.IsLink {
			next, ok := b.in.Res.Files[t.LinkTarget]
			if !ok || next == t {
				break
			}
			t = next
		}
		f.Access = t.Access
	}
}

func has(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
