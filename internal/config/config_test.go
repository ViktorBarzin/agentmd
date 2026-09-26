package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergesMachineThenUser(t *testing.T) {
	dir := t.TempDir()
	home := "/home/alex"
	machine := filepath.Join(dir, "etc.toml")
	user := filepath.Join(dir, "user.toml")
	write(t, machine, `
roots = ["/srv/repos"]
[budget]
file_bytes = 1000
[[origin]]
path = "/etc/claude-code/managed-settings.json"
field = "claudeMd"
source = ["~/code/infra/managed-settings.json", "~/code/managed-settings.json"]
note = "installed hourly"
`)
	write(t, user, `
roots = ["~/code"]
[budget]
file_bytes = 32768
[analysis]
cli = "codex"
[[build]]
output = "~/.agents/AGENTS.md"
parts = ["~/.agents/core.md", "~/.agents/personal.md"]
command = "agents-sync"
[[harness]]
name = "pi"
user_files = ["~/.pi/agent/AGENTS.md"]
project_files = ["AGENTS.md"]
skill_dirs = ["~/.agents/skills"]
`)
	c, err := Load(home, machine, filepath.Join(dir, "missing.toml"), user)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/srv/repos", "/home/alex/code"}; !reflect.DeepEqual(c.Roots, want) {
		t.Errorf("roots = %v, want %v", c.Roots, want)
	}
	if c.Budget.FileBytes != 32768 {
		t.Errorf("budget = %d, want the user's 32768", c.Budget.FileBytes)
	}
	if c.Analysis.CLI != "codex" {
		t.Errorf("cli = %q", c.Analysis.CLI)
	}
	if got := c.Origins[0].Source; !reflect.DeepEqual(got, []string{"/home/alex/code/infra/managed-settings.json", "/home/alex/code/managed-settings.json"}) {
		t.Errorf("origin sources = %v", got)
	}
	if got := c.Builds[0]; got.Output != "/home/alex/.agents/AGENTS.md" || got.Parts[1] != "/home/alex/.agents/personal.md" || got.Command != "agents-sync" {
		t.Errorf("build = %+v", got)
	}
	if got := c.Harnesses[0]; got.Name != "pi" || got.UserFiles[0] != "/home/alex/.pi/agent/AGENTS.md" || got.ProjectFiles[0] != "AGENTS.md" {
		t.Errorf("harness = %+v", got)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.toml")
	write(t, p, "rots = [\"~/code\"]\n")
	_, err := Load("/home/alex", p)
	if err == nil || !strings.Contains(err.Error(), "rots") {
		t.Fatalf("want an error naming the unknown key, got %v", err)
	}
}

func TestLoadWithNoFilesIsEmpty(t *testing.T) {
	c, err := Load("/home/alex", filepath.Join(t.TempDir(), "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Roots) != 0 || c.Budget.FileBytes != 0 {
		t.Errorf("want an empty config, got %+v", c)
	}
}

func TestExpand(t *testing.T) {
	cases := map[string]string{
		"~":           "/home/alex",
		"~/code":      "/home/alex/code",
		"/etc/x/../y": "/etc/y",
		"":            "",
		"rel/path":    "rel/path",
	}
	for in, want := range cases {
		if got := Expand("/home/alex", in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}
