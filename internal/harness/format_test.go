package harness

import (
	"path/filepath"
	"testing"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

// A new agentmd that reads probes differently must probe again rather than
// keep what the old code read, so the probe format is part of the fingerprint.
func TestFingerprintCoversTheProbeFormat(t *testing.T) {
	tr := testutil.New(t)
	app := tr.Repo(filepath.Join(tr.Code, "app"))
	tr.File(filepath.Join(app, "AGENTS.md"), "# app\n")
	env := Env{Home: tr.Home, Etc: tr.Etc, CodexHome: filepath.Join(tr.Home, ".codex"),
		ClaudeConfigDir: filepath.Join(tr.Home, ".claude"), GOOS: "linux", Roots: []string{tr.Code}}
	res, err := discover.Scan(discover.Env{Home: tr.Home, Etc: tr.Etc, CodexHome: env.CodexHome, GOOS: "linux", Roots: env.Roots})
	if err != nil {
		t.Fatal(err)
	}
	fp := func() string { return Fingerprint(env, res, NewStats(), "claude", "1.0", app, nil, ClaudeState{}) }
	before := fp()
	old := probeFormat
	probeFormat = old + "-next"
	defer func() { probeFormat = old }()
	if fp() == before {
		t.Error("a new probe format changes the fingerprint")
	}
}
