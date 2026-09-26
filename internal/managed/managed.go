// Package managed marks agent files that another tool maintains: chezmoi,
// the skills installer, Claude Code plugins and Codex's own skills. Each gets
// a note on where edits belong, and upstream copies become read-only.
package managed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// Env says where the tools keep their files.
type Env struct {
	Home            string
	CodexHome       string
	ClaudeConfigDir string
	// Chezmoi is the chezmoi binary, empty when it is not installed.
	Chezmoi string
	// Lockfiles are skills-installer lockfiles to read.
	Lockfiles []string
}

// Marker applies the notes, caching chezmoi's answers between scans.
type Marker struct {
	Env Env
	TTL time.Duration

	mu      sync.Mutex
	at      time.Time
	managed map[string]bool
	sources map[string]string
}

// Apply sets Access.Managed (and, for upstream copies, read-only) on files.
func (m *Marker) Apply(files map[string]*model.File) {
	plugins := filepath.Join(m.Env.ClaudeConfigDir, "plugins")
	codexSystem := filepath.Join(m.Env.CodexHome, "skills", ".system")
	locked := m.lockedSkills()
	var chezCandidates []string
	for _, f := range files {
		if f.Field != "" {
			continue
		}
		p := f.RealPath
		switch {
		case discover.Within(p, plugins):
			f.Access.Managed = &model.Managed{Tool: "plugin", Warn: true,
				Note: "Part of a Claude Code plugin. The next plugin update replaces edits made here."}
			f.Access.Writable = false
			f.Access.Reason = "part of a Claude Code plugin"
			continue
		case discover.Within(p, codexSystem):
			f.Access.Managed = &model.Managed{Tool: "codex", Warn: true,
				Note: "Installed by Codex, which replaces it when it updates."}
			f.Access.Writable = false
			f.Access.Reason = "installed by Codex"
			continue
		}
		if src, ok := lockedFor(locked, p); ok {
			f.Access.Managed = &model.Managed{Tool: "skills", Source: src, Warn: true,
				Note: "Installed from " + src + " by the skills CLI. Its next update replaces edits made here; keep changes as a patch instead."}
			continue
		}
		chezCandidates = append(chezCandidates, p)
	}
	if m.Env.Chezmoi == "" || len(chezCandidates) == 0 {
		return
	}
	managedSet, sources := m.chezmoi(chezCandidates)
	for _, f := range files {
		if f.Field != "" || f.Access.Managed != nil || !managedSet[f.RealPath] {
			continue
		}
		f.Access.Managed = &model.Managed{Tool: "chezmoi", Source: sources[f.RealPath],
			Note: "Managed by chezmoi. Edit here and save it back with your sync tool or `chezmoi re-add`."}
	}
}

// lockedSkill is one entry of a skills lockfile: the folder it installed and
// where it came from.
type lockedSkill struct {
	dir    string
	source string
}

func (m *Marker) lockedSkills() []lockedSkill {
	var out []lockedSkill
	for _, lf := range m.Env.Lockfiles {
		data, err := os.ReadFile(lf)
		if err != nil {
			continue
		}
		var lock struct {
			Skills map[string]struct {
				Source    string `json:"source"`
				SourceURL string `json:"sourceUrl"`
			} `json:"skills"`
		}
		if json.Unmarshal(data, &lock) != nil {
			continue
		}
		base := filepath.Join(filepath.Dir(lf), "skills")
		for name, s := range lock.Skills {
			src := s.Source
			if src == "" {
				src = s.SourceURL
			}
			out = append(out, lockedSkill{dir: filepath.Join(base, name), source: src})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

func lockedFor(locked []lockedSkill, p string) (string, bool) {
	for _, l := range locked {
		if discover.Within(p, l.dir) {
			return l.source, true
		}
	}
	return "", false
}

// chezmoi asks chezmoi which of the paths it manages and where their sources
// are, reusing the answer for TTL.
func (m *Marker) chezmoi(paths []string) (map[string]bool, map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ttl := m.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	if m.managed != nil && time.Since(m.at) < ttl {
		missing := false
		for _, p := range paths {
			if m.managed[p] {
				if _, ok := m.sources[p]; !ok {
					missing = true
				}
			}
		}
		if !missing {
			return m.managed, m.sources
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, m.Env.Chezmoi, "managed", "--include=files", "--path-style=absolute").Output()
	set := map[string]bool{}
	if err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			set[strings.TrimSpace(sc.Text())] = true
		}
	}
	var mine []string
	for _, p := range paths {
		if set[p] {
			mine = append(mine, p)
		}
	}
	sources := map[string]string{}
	if len(mine) > 0 {
		args := append([]string{"source-path"}, mine...)
		if out, err := exec.CommandContext(ctx, m.Env.Chezmoi, args...).Output(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) == len(mine) {
				for i, p := range mine {
					sources[p] = strings.TrimSpace(lines[i])
				}
			}
		}
	}
	m.managed, m.sources, m.at = set, sources, time.Now()
	return set, sources
}
