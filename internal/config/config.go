// Package config reads agentmd's optional TOML configuration: a machine-wide
// file and the owner's own file, merged in that order.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the merged configuration. Every path in it is absolute after Load.
type Config struct {
	Roots   []string `toml:"roots"`
	Exclude []string `toml:"exclude"`
	// SkipContexts lists directories (globs) where no session starts, such as
	// a folder of other users' instruction files, so no context is built there.
	SkipContexts []string        `toml:"skip_contexts"`
	Budget       Budget          `toml:"budget"`
	Analysis     Analysis        `toml:"analysis"`
	Origins      []OriginRule    `toml:"origin"`
	Builds       []BuildRule     `toml:"build"`
	Harnesses    []StaticHarness `toml:"harness"`
}

// Budget holds the owner's own size limit. Zero means no limit.
type Budget struct {
	FileBytes int `toml:"file_bytes"`
}

// Analysis picks the agent CLI used for analysis: "claude", "codex", or empty
// for the first one installed.
type Analysis struct {
	CLI string `toml:"cli"`
}

// OriginRule says that a file (or one field of it) is a copy installed from
// somewhere else. Source lists candidates; the first that exists wins.
type OriginRule struct {
	Path   string   `toml:"path"`
	Field  string   `toml:"field"`
	Source []string `toml:"source"`
	// SourceField is the field in the source, when it differs from Field.
	SourceField string `toml:"source_field"`
	Note        string `toml:"note"`
}

// SourceFieldName is the field to read in the source file.
func (o OriginRule) SourceFieldName() string {
	if o.SourceField != "" {
		return o.SourceField
	}
	return o.Field
}

// BuildRule says that Output is built from Parts by Command.
type BuildRule struct {
	Output  string   `toml:"output"`
	Parts   []string `toml:"parts"`
	Command string   `toml:"command"`
}

// StaticHarness is an extra harness modelled by static rules only.
type StaticHarness struct {
	Name         string   `toml:"name"`
	UserFiles    []string `toml:"user_files"`
	ProjectFiles []string `toml:"project_files"`
	SkillDirs    []string `toml:"skill_dirs"`
}

// DefaultPaths returns the files Load reads, machine-wide first.
func DefaultPaths(etc, home string) []string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return []string{
		filepath.Join(etc, "agentmd", "config.toml"),
		filepath.Join(xdg, "agentmd", "config.toml"),
	}
}

// Load reads and merges the files that exist, in order. Lists are appended,
// and scalar values from a later file replace earlier ones.
func Load(home string, paths ...string) (Config, error) {
	var merged Config
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("read %s: %w", p, err)
		}
		var c Config
		md, err := toml.Decode(string(data), &c)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", p, err)
		}
		if undecoded := md.Undecoded(); len(undecoded) > 0 {
			keys := make([]string, len(undecoded))
			for i, k := range undecoded {
				keys[i] = k.String()
			}
			return Config{}, fmt.Errorf("%s: unknown keys: %s", p, strings.Join(keys, ", "))
		}
		merged = merge(merged, c)
	}
	return merged.expand(home), nil
}

func merge(a, b Config) Config {
	a.Roots = append(a.Roots, b.Roots...)
	a.Exclude = append(a.Exclude, b.Exclude...)
	a.SkipContexts = append(a.SkipContexts, b.SkipContexts...)
	if b.Budget.FileBytes != 0 {
		a.Budget.FileBytes = b.Budget.FileBytes
	}
	if b.Analysis.CLI != "" {
		a.Analysis.CLI = b.Analysis.CLI
	}
	a.Origins = append(a.Origins, b.Origins...)
	a.Builds = append(a.Builds, b.Builds...)
	a.Harnesses = append(a.Harnesses, b.Harnesses...)
	return a
}

func (c Config) expand(home string) Config {
	c.Roots = ExpandAll(home, c.Roots)
	c.SkipContexts = ExpandAll(home, c.SkipContexts)
	for i := range c.Origins {
		c.Origins[i].Path = Expand(home, c.Origins[i].Path)
		c.Origins[i].Source = ExpandAll(home, c.Origins[i].Source)
	}
	for i := range c.Builds {
		c.Builds[i].Output = Expand(home, c.Builds[i].Output)
		c.Builds[i].Parts = ExpandAll(home, c.Builds[i].Parts)
	}
	for i := range c.Harnesses {
		c.Harnesses[i].UserFiles = ExpandAll(home, c.Harnesses[i].UserFiles)
		c.Harnesses[i].SkillDirs = ExpandAll(home, c.Harnesses[i].SkillDirs)
	}
	return c
}

// Expand replaces a leading "~" with home and cleans the path.
func Expand(home, p string) string {
	switch {
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	case p == "":
		return p
	}
	return filepath.Clean(p)
}

// ExpandAll applies Expand to every path.
func ExpandAll(home string, ps []string) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, Expand(home, p))
	}
	return out
}
