package probe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeClaude is a stand-in for the claude CLI: it checks the probe's pins,
// records what it saw, and posts a request shaped like the real one.
const fakeClaude = `#!/usr/bin/env python3
import json, os, sys, urllib.request
args = sys.argv[1:]
settings = json.loads(args[args.index("--settings") + 1])
seen = {
  "cwd": os.getcwd(),
  "tmux": os.environ.get("TMUX", ""),
  "oauth": os.environ.get("CLAUDE_CODE_OAUTH_TOKEN", ""),
  "config_dir": os.environ.get("CLAUDE_CONFIG_DIR", ""),
  "user_md_is_link": os.path.islink(os.path.join(os.environ["CLAUDE_CONFIG_DIR"], "CLAUDE.md")),
  "projects_linked": os.path.exists(os.path.join(os.environ["CLAUDE_CONFIG_DIR"], "projects")),
  "global_copied": os.path.isfile(os.path.join(os.environ["CLAUDE_CONFIG_DIR"], ".claude.json")),
  "hooks_off": settings.get("disableAllHooks"),
  "helper": settings.get("apiKeyHelper"),
  "base_url_pinned": settings["env"]["ANTHROPIC_BASE_URL"] == os.environ["ANTHROPIC_BASE_URL"],
  "args": args,
}
open(os.environ["FAKE_SEEN"], "w").write(json.dumps(seen))
if os.environ.get("FAKE_MODE") == "silent":
    sys.stderr.write("could not start\n")
    sys.exit(1)
cfg = os.environ["CLAUDE_CONFIG_DIR"]
text = ("<system-reminder>\nContents of <managed-settings> (organization-managed policy instructions):\n\n# Org\n\n"
        "Contents of " + cfg + "/CLAUDE.md (user's private global instructions for all projects):\n\n# User\n\n"
        "Contents of " + os.getcwd() + "/CLAUDE.md (project instructions, checked into the codebase):\n\n# Project\n</system-reminder>")
body = {"model": "x", "messages": [{"role": "user", "content": [{"type": "text", "text": text}]}],
        "system": [{"type": "text", "text": "The following skills are available for use with the Skill tool:\n\n- tdd\n"}]}
url = settings["env"]["ANTHROPIC_BASE_URL"] + "/v1/messages?beta=true"
req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"content-type": "application/json"})
try:
    urllib.request.urlopen(req)
except Exception:
    pass
print(json.dumps({"is_error": True}))
sys.exit(1)
`

func writeExe(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func needPython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed for the fake CLIs")
	}
}

func TestClaudeProbe(t *testing.T) {
	needPython(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".claude")
	for _, d := range []string{filepath.Join(config, "projects"), filepath.Join(root, "repo")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(config, "CLAUDE.md"), []byte("# User\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"projects":{}}`), 0o600)
	seen := filepath.Join(root, "seen.json")
	c := &Claude{
		Bin:          writeExe(t, filepath.Join(root, "claude"), fakeClaude),
		ConfigDir:    config,
		GlobalConfig: filepath.Join(home, ".claude.json"),
		ScratchRoot:  filepath.Join(root, "cache"),
		Env:          append(os.Environ(), "TMUX=/tmp/tmux-1/default,1,0", "CLAUDE_CODE_OAUTH_TOKEN=real-token", "FAKE_SEEN="+seen),
	}
	res, err := c.Probe(context.Background(), filepath.Join(root, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, f := range res.Files {
		labels = append(labels, f.Label)
	}
	want := []string{"<managed-settings>", res.ScratchDir + "/CLAUDE.md", filepath.Join(root, "repo") + "/CLAUDE.md"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "tdd" {
		t.Errorf("skills = %v", res.Skills)
	}
	data, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{`"tmux": ""`, `"oauth": ""`, `"user_md_is_link": true`, `"projects_linked": false`,
		`"global_copied": true`, `"hooks_off": true`, `"helper": ""`, `"base_url_pinned": true`, `"--no-session-persistence"`, `"--strict-mcp-config"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the fake CLI saw %s, want it to contain %s", s, want)
		}
	}
	if _, err := os.Stat(res.ScratchDir); !os.IsNotExist(err) {
		t.Error("the throwaway config dir is removed after the probe")
	}
}

func TestClaudeProbeWithoutRequest(t *testing.T) {
	needPython(t)
	root := t.TempDir()
	c := &Claude{
		Bin:         writeExe(t, filepath.Join(root, "claude"), fakeClaude),
		ConfigDir:   filepath.Join(root, "none"),
		ScratchRoot: filepath.Join(root, "cache"),
		Env:         append(os.Environ(), "FAKE_MODE=silent", "FAKE_SEEN="+filepath.Join(root, "seen.json")),
	}
	_, err := c.Probe(context.Background(), root)
	if !errors.Is(err, ErrNoRequest) || !strings.Contains(err.Error(), "could not start") {
		t.Fatalf("want ErrNoRequest with the CLI's stderr, got %v", err)
	}
}

func TestClaudeProbeRefusesRoutedPolicy(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed-settings.json")
	os.WriteFile(managed, []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://gateway.example"},"apiKeyHelper":"/bin/key"}`), 0o644)
	c := &Claude{Bin: "/bin/false", ManagedSettings: managed, ScratchRoot: root}
	_, err := c.Probe(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_BASE_URL") || !strings.Contains(err.Error(), "apiKeyHelper") {
		t.Fatalf("want a refusal naming both settings, got %v", err)
	}
	os.WriteFile(managed, []byte(`{"env":{"CLAUDE_CODE_ENABLE_TELEMETRY":"1","CLAUDE_CODE_USE_BEDROCK":"0"}}`), 0o644)
	if err := c.checkPolicy(); err != nil {
		t.Errorf("telemetry and a disabled provider are fine, got %v", err)
	}
}

const fakeCodex = `#!/usr/bin/env python3
import json, os, sys
open(os.environ["FAKE_SEEN"], "w").write(json.dumps({"args": sys.argv[1:], "tmux": os.environ.get("TMUX", "")}))
print(json.dumps([
  {"type": "message", "role": "user", "content": [{"type": "input_text", "text": "# AGENTS.md instructions for " + os.getcwd() + "\n\n<INSTRUCTIONS>\n# User\n\n--- project-doc ---\n\n# Project\n</INSTRUCTIONS>"}]},
]))
`

func TestCodexProbeTurnsMCPServersOff(t *testing.T) {
	needPython(t)
	root := t.TempDir()
	codexHome := filepath.Join(root, ".codex")
	os.MkdirAll(codexHome, 0o755)
	os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(`
model = "x"
[mcp_servers.ha]
command = "ha-mcp"
[mcp_servers."paperless.mcp"]
command = "p"
[profiles.work.mcp_servers.jira]
command = "j"
`), 0o644)
	repo := filepath.Join(root, "repo")
	os.MkdirAll(filepath.Join(repo, ".codex"), 0o755)
	os.WriteFile(filepath.Join(repo, ".codex", "config.toml"), []byte("[mcp_servers.local]\ncommand = \"l\"\n"), 0o644)
	seen := filepath.Join(root, "seen.json")
	c := &Codex{Bin: writeExe(t, filepath.Join(root, "codex"), fakeCodex), CodexHome: codexHome,
		Env: append(os.Environ(), "TMUX=x", "FAKE_SEEN="+seen)}
	cap, err := c.Probe(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Instructions != "# User\n\n--- project-doc ---\n\n# Project" {
		t.Errorf("instructions = %q", cap.Instructions)
	}
	data, _ := os.ReadFile(seen)
	s := string(data)
	for _, want := range []string{`mcp_servers.ha.enabled=false`, `mcp_servers.jira.enabled=false`, `mcp_servers.local.enabled=false`,
		`mcp_servers.local.command=\"true\"`, `mcp_servers.\"paperless.mcp\".enabled=false`, `"tmux": ""`, `"debug", "prompt-input"`} {
		if !strings.Contains(s, want) {
			t.Errorf("codex saw %s, want %s", s, want)
		}
	}
}

func TestProjectServersGetAStandInCommandOnly(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	os.MkdirAll(home, 0o755)
	os.WriteFile(filepath.Join(home, "config.toml"), []byte("[mcp_servers.ha]\nurl = \"http://x\"\n"), 0o644)
	repo := filepath.Join(root, "repo")
	os.MkdirAll(filepath.Join(repo, ".codex"), 0o755)
	os.WriteFile(filepath.Join(repo, ".codex", "config.toml"), []byte("[mcp_servers.ha]\nurl = \"http://y\"\n[mcp_servers.xcode]\ncommand = \"npx\"\n"), 0o644)
	got := strings.Join(DisableMCPArgs(home, repo), " ")
	want := `-c mcp_servers.ha.enabled=false -c mcp_servers.xcode.enabled=false -c mcp_servers.xcode.command="true"`
	if got != want {
		t.Errorf("args = %s\nwant   %s", got, want)
	}
}

func TestCleanEnv(t *testing.T) {
	got := cleanEnv([]string{"TMUX=1", "TMUX_PANE=%1", "TMUXX=keep", "ANTHROPIC_API_KEY=k", "ANTHROPIC_BASE_URL=u", "HOME=/h"}, "TMUX", "TMUX_PANE", "ANTHROPIC_")
	if !reflect.DeepEqual(got, []string{"TMUXX=keep", "HOME=/h"}) {
		t.Errorf("cleanEnv = %v", got)
	}
}
