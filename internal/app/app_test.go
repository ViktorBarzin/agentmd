package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

// fakeClaude answers --version and, for a probe, posts a request naming the
// user file (through the throwaway config dir) and the folder's CLAUDE.md.
const fakeClaude = `#!/usr/bin/env python3
import json, os, sys, urllib.request
if sys.argv[1:] == ["--version"]:
    print("9.9.9 (Claude Code)"); sys.exit(0)
open(os.environ["FAKE_COUNT"], "a").write("claude\n")
args = sys.argv[1:]
settings = json.loads(args[args.index("--settings") + 1])
cfg = os.environ["CLAUDE_CONFIG_DIR"]
parts = []
user = os.path.join(cfg, "CLAUDE.md")
if os.path.exists(user):
    parts.append("Contents of " + user + " (user's private global instructions for all projects):\n\n" + open(user).read())
proj = os.path.join(os.getcwd(), "CLAUDE.md")
if os.path.exists(proj):
    parts.append("Contents of " + proj + " (project instructions, checked into the codebase):\n\n" + open(proj).read())
text = "<system-reminder>\n" + "\n\n".join(parts) + "\n</system-reminder>"
body = {"messages": [{"role": "user", "content": [{"type": "text", "text": text}]}]}
req = urllib.request.Request(settings["env"]["ANTHROPIC_BASE_URL"] + "/v1/messages", data=json.dumps(body).encode(), headers={"content-type": "application/json"})
try:
    urllib.request.urlopen(req)
except Exception:
    pass
sys.exit(1)
`

// fakeCodex joins the user file and the folder's AGENTS.md the way Codex does.
const fakeCodex = `#!/usr/bin/env python3
import json, os, sys
if sys.argv[1:] == ["--version"]:
    print("codex-cli 9.9.9"); sys.exit(0)
open(os.environ["FAKE_COUNT"], "a").write("codex\n")
home = os.environ["CODEX_HOME"]
user = open(os.path.join(home, "AGENTS.md")).read().strip() if os.path.exists(os.path.join(home, "AGENTS.md")) else ""
proj = open("AGENTS.md").read() if os.path.exists("AGENTS.md") else ""
text = user + ("\n\n--- project-doc ---\n\n" + proj if proj else "")
print(json.dumps([{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "# AGENTS.md instructions for " + os.getcwd() + "\n\n<INSTRUCTIONS>\n" + text + "\n</INSTRUCTIONS>"}]}]))
`

type fixture struct {
	tr    *testutil.Tree
	app   *App
	count string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed for the fake CLIs")
	}
	tr := testutil.New(t)
	bin := tr.Dir("fakebin")
	os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeClaude), 0o755)
	os.WriteFile(filepath.Join(bin, "codex"), []byte(fakeCodex), 0o755)
	shared := "Never commit secrets and always run the full test suite before you push anything to the shared branch."
	tr.File(filepath.Join(tr.Home, ".agents/AGENTS.md"), "# User\n"+shared+"\n")
	tr.Link(filepath.Join(tr.Home, ".claude/CLAUDE.md"), "../.agents/AGENTS.md")
	tr.Link(filepath.Join(tr.Home, ".codex/AGENTS.md"), "../.agents/AGENTS.md")
	for _, r := range []string{"app", "lib"} {
		d := tr.Repo(filepath.Join(tr.Code, r))
		tr.File(filepath.Join(d, "AGENTS.md"), "# "+r+"\n"+shared+"\n")
		tr.Link(filepath.Join(d, "CLAUDE.md"), "AGENTS.md")
	}
	count := filepath.Join(tr.Root, "count")
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + tr.Home, "CODEX_HOME=" + filepath.Join(tr.Home, ".codex"), "FAKE_COUNT=" + count}
	a, err := New(Options{Home: tr.Home, Etc: tr.Etc, CodexHome: filepath.Join(tr.Home, ".codex"), Roots: []string{tr.Code},
		ConfigPaths: []string{}, CacheDir: filepath.Join(tr.Root, "cache"), Env: env, Owner: "alex"})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{tr: tr, app: a, count: count}
}

func (f *fixture) runs(t *testing.T) int {
	t.Helper()
	data, _ := os.ReadFile(f.count)
	return strings.Count(string(data), "\n")
}

func contextIDs(st *model.State, source string) []string {
	var out []string
	for _, c := range st.Contexts {
		if c.Source == source {
			out = append(out, c.ID)
		}
	}
	sort.Strings(out)
	return out
}

func TestProbeEverythingThenOnlyWhatChanged(t *testing.T) {
	oldPath := os.Getenv("PATH")
	f := setup(t)
	os.Setenv("PATH", filepath.Join(f.tr.Root, "fakebin")+":"+oldPath)
	defer os.Setenv("PATH", oldPath)
	a, err := New(f.app.opts)
	if err != nil {
		t.Fatal(err)
	}
	st, err := a.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Unprobed) != 6 {
		t.Fatalf("want 6 unprobed contexts (home and two repos, two harnesses), got %d: %+v", len(st.Unprobed), st.Unprobed)
	}
	var progress atomic.Int32
	st, err = a.Probe(context.Background(), nil, func(done, total int, id string, err error) {
		progress.Add(1)
		if err != nil {
			t.Errorf("probe %s: %v", id, err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Load() != 6 || f.runs(t) != 6 || len(st.Unprobed) != 0 {
		t.Fatalf("progress %d, runs %d, unprobed %d", progress.Load(), f.runs(t), len(st.Unprobed))
	}
	probed := contextIDs(st, "probe")
	if len(probed) != 6 {
		t.Fatalf("probed = %v", probed)
	}
	var appClaude *model.Context
	for i := range st.Contexts {
		if st.Contexts[i].ID == "claude:"+filepath.Join(f.tr.Code, "app") {
			appClaude = &st.Contexts[i]
		}
	}
	if appClaude == nil || len(appClaude.Entries) != 2 || appClaude.Version != "9.9.9 (Claude Code)" {
		t.Fatalf("claude context in app = %+v", appClaude)
	}
	if appClaude.Entries[0].FileID != filepath.Join(f.tr.Home, ".claude/CLAUDE.md") {
		t.Errorf("the throwaway config path maps back to ~/.claude: %+v", appClaude.Entries[0])
	}
	// The shared sentence co-loads with each repo file: a problem, not a hint.
	var problems int
	for _, fd := range st.Findings {
		if fd.Kind == model.FindDuplicate && fd.Severity == model.Problem {
			problems++
		}
	}
	if problems < 2 {
		t.Errorf("want the user file duplicated in both repos as problems, got %d: %+v", problems, st.Findings)
	}

	// Nothing changed: probing again runs nothing.
	if _, err := a.Probe(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.runs(t) != 6 {
		t.Errorf("a fresh cache runs no probes, ran %d", f.runs(t)-6)
	}

	// Change the app repo's file: only the app contexts go stale.
	p := filepath.Join(f.tr.Code, "app", "AGENTS.md")
	os.WriteFile(p, []byte("# app\nchanged\n"), 0o644)
	later := time.Now().Add(time.Minute)
	os.Chtimes(p, later, later)
	st, err = a.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var stale []string
	for _, c := range st.Contexts {
		if c.Stale {
			stale = append(stale, c.ID)
		}
	}
	sort.Strings(stale)
	want := []string{"claude:" + filepath.Join(f.tr.Code, "app"), "codex:" + filepath.Join(f.tr.Code, "app")}
	if strings.Join(stale, ",") != strings.Join(want, ",") {
		t.Errorf("stale = %v, want %v", stale, want)
	}
	if _, err := a.Probe(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.runs(t) != 8 {
		t.Errorf("only the two stale contexts are probed again, total runs %d", f.runs(t))
	}
}

func TestProbeRejectsUnknownContexts(t *testing.T) {
	f := setup(t)
	a := f.app
	if _, err := a.Scan(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"agents-md:/x", "claude:relative/path", "nonsense"} {
		if _, err := a.Probe(context.Background(), []string{id}, nil); err == nil {
			t.Errorf("%s: want an error", id)
		}
	}
}
