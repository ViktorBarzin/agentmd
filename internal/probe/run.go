package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultTimeout bounds one probe.
const DefaultTimeout = 90 * time.Second

// Prompt is the message a probe sends. It shows up in session telemetry.
const Prompt = "agentmd probe"

// stateEntries are the parts of the Claude config dir a probe must not write
// through to: session history, snapshots and other per-run state.
var stateEntries = map[string]bool{
	"projects": true, "shell-snapshots": true, "todos": true, "statsig": true,
	"session-env": true, "debug": true, "backups": true, "file-history": true,
	"history.jsonl": true, "ide": true, "logs": true, "telemetry": true,
	"paste-cache": true, ".credentials.json": true, ".claude.json": true,
	"jobs": true, "daemon": true, "daemon.log": true,
}

// Claude probes Claude Code.
type Claude struct {
	Bin string
	// ConfigDir is the owner's Claude config dir ($CLAUDE_CONFIG_DIR or ~/.claude).
	ConfigDir string
	// GlobalConfig is the owner's global state file (~/.claude.json).
	GlobalConfig string
	// ScratchRoot holds the throwaway config dirs.
	ScratchRoot string
	// ManagedSettings is the org policy settings file, checked before probing.
	ManagedSettings string
	Env             []string
	Timeout         time.Duration
}

// ClaudeResult is a capture plus the scratch dir prefix to map back.
type ClaudeResult struct {
	*ClaudeCapture
	ScratchDir string
}

// Probe runs Claude Code in dir against a stand-in API and parses what it sent.
func (c *Claude) Probe(ctx context.Context, dir string) (*ClaudeResult, error) {
	if err := c.checkPolicy(); err != nil {
		return nil, err
	}
	stand, err := newStandIn()
	if err != nil {
		return nil, err
	}
	defer stand.Close()

	scratch, err := c.scratchConfig()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)

	settings, err := json.Marshal(map[string]any{
		"disableAllHooks":     true,
		"apiKeyHelper":        "",
		"otelHeadersHelper":   "",
		"awsAuthRefresh":      "",
		"awsCredentialExport": "",
		"env": map[string]string{
			"ANTHROPIC_BASE_URL":      stand.URL,
			"ANTHROPIC_API_KEY":       "agentmd-probe",
			"ANTHROPIC_AUTH_TOKEN":    "",
			"CLAUDE_CODE_USE_BEDROCK": "0",
			"CLAUDE_CODE_USE_VERTEX":  "0",
			"CLAUDE_CODE_USE_FOUNDRY": "0",
		},
	})
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, "-p", Prompt, "--output-format", "json",
		"--no-session-persistence", "--strict-mcp-config", "--settings", string(settings))
	cmd.Dir = dir
	cmd.Env = append(cleanEnv(c.Env, "TMUX", "TMUX_PANE", "ANTHROPIC_", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"),
		"CLAUDE_CONFIG_DIR="+scratch,
		"ANTHROPIC_BASE_URL="+stand.URL,
		"ANTHROPIC_API_KEY=agentmd-probe",
	)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	body := stand.Best()
	if body == nil {
		msg := strings.TrimSpace(tail(stderr.String(), 400))
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: timed out after %s", ErrNoRequest, timeout)
		}
		if runErr != nil && msg != "" {
			return nil, fmt.Errorf("%w: %s", ErrNoRequest, msg)
		}
		return nil, ErrNoRequest
	}
	capture, err := ParseClaudeRequest(body)
	if err != nil {
		return nil, fmt.Errorf("parse the recorded request: %w", err)
	}
	return &ClaudeResult{ClaudeCapture: capture, ScratchDir: scratch}, nil
}

// checkPolicy refuses to probe when the org policy points Claude at another
// endpoint or key source, since agentmd's pins cannot override managed settings.
func (c *Claude) checkPolicy() error {
	if c.ManagedSettings == "" {
		return nil
	}
	data, err := os.ReadFile(c.ManagedSettings)
	if err != nil {
		return nil
	}
	var m struct {
		Env          map[string]string `json:"env"`
		APIKeyHelper string            `json:"apiKeyHelper"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	var reasons []string
	for _, k := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if m.Env[k] != "" {
			reasons = append(reasons, k)
		}
	}
	for _, k := range []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
		if v := strings.ToLower(m.Env[k]); v != "" && v != "0" && v != "false" {
			reasons = append(reasons, k)
		}
	}
	if m.APIKeyHelper != "" {
		reasons = append(reasons, "apiKeyHelper")
	}
	if len(reasons) > 0 {
		return fmt.Errorf("the org policy sets %s, so a probe could reach a real model; not probing", strings.Join(reasons, ", "))
	}
	return nil
}

// scratchConfig makes a throwaway config dir that links every entry of the
// owner's config dir except per-run state, plus a copy of the global state
// file, so the probe sees the same setup and writes nothing back.
func (c *Claude) scratchConfig() (string, error) {
	if err := os.MkdirAll(c.ScratchRoot, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(c.ScratchRoot, "claude-")
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(c.ConfigDir)
	if err != nil && !os.IsNotExist(err) {
		os.RemoveAll(dir)
		return "", err
	}
	for _, e := range entries {
		if stateEntries[e.Name()] {
			continue
		}
		if err := os.Symlink(filepath.Join(c.ConfigDir, e.Name()), filepath.Join(dir, e.Name())); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	if data, err := os.ReadFile(c.GlobalConfig); err == nil {
		if err := os.WriteFile(filepath.Join(dir, ".claude.json"), data, 0o600); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

// Codex probes Codex.
type Codex struct {
	Bin       string
	CodexHome string
	Env       []string
	Timeout   time.Duration
}

// Probe runs `codex debug prompt-input` in dir with every MCP server off.
func (c *Codex) Probe(ctx context.Context, dir string) (*CodexCapture, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"debug", "prompt-input"}
	args = append(args, DisableMCPArgs(c.CodexHome, dir)...)
	args = append(args, Prompt)
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Dir = dir
	cmd.Env = cleanEnv(c.Env, "TMUX", "TMUX_PANE")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("codex timed out after %s", timeout)
		}
		return nil, fmt.Errorf("codex: %v: %s", err, strings.TrimSpace(tail(stderr.String(), 400)))
	}
	return ParseCodexPromptInput(stdout.Bytes())
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func tomlKey(k string) string {
	if bareKey.MatchString(k) {
		return k
	}
	b, _ := json.Marshal(k)
	return string(b)
}

// DisableMCPArgs returns the -c flags that switch off every MCP server Codex
// would start in dir. A server defined only in a project config also gets a
// stand-in command, because Codex rejects an override that has no transport.
func DisableMCPArgs(codexHome, dir string) []string {
	user, project := CodexMCPServers(codexHome, dir)
	var args []string
	for _, name := range user {
		args = append(args, "-c", "mcp_servers."+tomlKey(name)+".enabled=false")
	}
	for _, name := range project {
		args = append(args, "-c", "mcp_servers."+tomlKey(name)+".enabled=false",
			"-c", "mcp_servers."+tomlKey(name)+`.command="true"`)
	}
	return args
}

// CodexMCPServers lists the MCP server names in Codex's own config and its
// profiles (user), and those only in a project config from dir up (project).
func CodexMCPServers(codexHome, dir string) (user, project []string) {
	read := func(path string, into map[string]bool) {
		var cfg struct {
			MCPServers map[string]toml.Primitive `toml:"mcp_servers"`
			Profiles   map[string]struct {
				MCPServers map[string]toml.Primitive `toml:"mcp_servers"`
			} `toml:"profiles"`
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return
		}
		for n := range cfg.MCPServers {
			into[n] = true
		}
		for _, p := range cfg.Profiles {
			for n := range p.MCPServers {
				into[n] = true
			}
		}
	}
	u, pr := map[string]bool{}, map[string]bool{}
	read(filepath.Join(codexHome, "config.toml"), u)
	for d := dir; ; d = filepath.Dir(d) {
		read(filepath.Join(d, ".codex", "config.toml"), pr)
		if d == filepath.Dir(d) {
			break
		}
	}
	for n := range u {
		user = append(user, n)
	}
	for n := range pr {
		if !u[n] {
			project = append(project, n)
		}
	}
	sort.Strings(user)
	sort.Strings(project)
	return user, project
}

// cleanEnv drops variables whose names equal or start with any prefix.
func cleanEnv(env []string, prefixes ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, p := range prefixes {
			if name == p || (strings.HasSuffix(p, "_") && strings.HasPrefix(name, p)) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// standIn is a local API that records message requests and answers each with
// an error, so a probe never reaches a model.
type standIn struct {
	URL  string
	srv  *http.Server
	ln   net.Listener
	mu   sync.Mutex
	best []byte
}

func newStandIn() (*standIn, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &standIn{ln: ln, URL: "http://" + ln.Addr().String()}
	s.srv = &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *standIn) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v1/messages") || strings.Contains(r.URL.Path, "count_tokens") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	s.mu.Lock()
	if len(body) > len(s.best) && bytes.Contains(body, []byte(`"messages"`)) {
		s.best = body
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"agentmd probe: no model behind this endpoint"}}`))
}

// Best returns the largest message request seen, which is the one carrying
// the session context.
func (s *standIn) Best() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.best
}

func (s *standIn) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

// Version runs "<bin> --version" and returns the first line.
func Version(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line, nil
}

// FindBinary looks for a harness CLI on PATH and in the usual install places.
func FindBinary(name, home string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(home, ".claude", "local", name),
		filepath.Join(home, ".npm-global", "bin", name),
		filepath.Join(home, ".bun", "bin", name),
		"/usr/local/bin/" + name,
		"/opt/homebrew/bin/" + name,
		"/usr/bin/" + name,
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}
