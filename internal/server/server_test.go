package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/app"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/testutil"
)

func newApp(t *testing.T, tr *testutil.Tree) *app.App {
	t.Helper()
	a, err := app.New(app.Options{Home: tr.Home, Etc: tr.Etc, Roots: []string{tr.Code}, ConfigPaths: []string{},
		CacheDir: filepath.Join(tr.Root, "cache"), NoCLIs: true, Owner: "alex"})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

var ui = fstest.MapFS{"index.html": {Data: []byte("<html>agentmd</html>")}, "assets/app.js": {Data: []byte("x")}}

func newServer(t *testing.T, a *app.App, opts Options) *Server {
	t.Helper()
	opts.UI = ui
	s, err := New(a, opts, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type call struct {
	method, path, host, body string
	headers                  map[string]string
}

func do(t *testing.T, h http.Handler, c call) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	req.Host = c.host
	if req.Host == "" {
		req.Host = "127.0.0.1:7390"
	}
	if strings.HasPrefix(c.path, "/api/") {
		req.Header.Set(RequestHeader, "1")
	}
	if c.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoopbackGuards(t *testing.T) {
	tr := testutil.New(t)
	h := newServer(t, newApp(t, tr), Options{}).Handler()
	cases := []struct {
		name string
		c    call
		want int
	}{
		{"state", call{method: "GET", path: "/api/state"}, 200},
		{"localhost name", call{method: "GET", path: "/api/state", host: "localhost:7390"}, 200},
		{"ipv6 loopback", call{method: "GET", path: "/api/state", host: "[::1]:7390"}, 200},
		{"rebinding host", call{method: "GET", path: "/api/state", host: "evil.example:7390"}, http.StatusMisdirectedRequest},
		{"no request header", call{method: "GET", path: "/api/state", headers: map[string]string{RequestHeader: ""}}, 403},
		{"form post", call{method: "POST", path: "/api/scan", body: "a=b", headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}}, 415},
		{"preflight", call{method: "OPTIONS", path: "/api/state"}, 405},
		{"ui", call{method: "GET", path: "/"}, 200},
		{"spa route", call{method: "GET", path: "/findings/x"}, 200},
		{"unknown api", call{method: "GET", path: "/api/nope"}, 404},
	}
	for _, tc := range cases {
		rec := do(t, h, tc.c)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: sent a CORS header", tc.name)
		}
		if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: missing security headers", tc.name)
		}
	}
	if rec := do(t, h, call{method: "GET", path: "/findings/x"}); !strings.Contains(rec.Body.String(), "agentmd") {
		t.Error("unknown UI routes serve index.html")
	}
}

func TestProxyMode(t *testing.T) {
	tr := testutil.New(t)
	secretFile := filepath.Join(tr.Root, "secret")
	os.WriteFile(secretFile, []byte("0123456789abcdef0123456789abcdef\n"), 0o600)
	s := newServer(t, newApp(t, tr), Options{ProxySecretFile: secretFile, IdentityHeader: "X-Authentik-Username", AllowIdentities: []string{"alex-sso"}})
	h := s.Handler()
	ok := map[string]string{SecretHeader: "0123456789abcdef0123456789abcdef", "X-Authentik-Username": "alex-sso"}
	if rec := do(t, h, call{method: "GET", path: "/api/state", host: "agentmd.example.com", headers: ok}); rec.Code != 200 {
		t.Errorf("a proxied request with the secret and identity: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, call{method: "GET", path: "/", host: "agentmd.example.com"}); rec.Code != 401 {
		t.Errorf("no secret: %d", rec.Code)
	}
	bad := map[string]string{SecretHeader: "0123456789abcdef0123456789abcdeX", "X-Authentik-Username": "alex-sso"}
	if rec := do(t, h, call{method: "GET", path: "/api/state", host: "x", headers: bad}); rec.Code != 401 {
		t.Errorf("wrong secret: %d", rec.Code)
	}
	other := map[string]string{SecretHeader: "0123456789abcdef0123456789abcdef", "X-Authentik-Username": "mallory"}
	if rec := do(t, h, call{method: "GET", path: "/api/state", host: "x", headers: other}); rec.Code != 403 {
		t.Errorf("another identity: %d", rec.Code)
	}
	if _, err := New(newApp(t, tr), Options{ProxySecretFile: filepath.Join(tr.Root, "missing")}, io.Discard); err == nil {
		t.Error("a missing secret file is an error")
	}
	short := filepath.Join(tr.Root, "short")
	os.WriteFile(short, []byte("abc"), 0o600)
	if _, err := New(newApp(t, tr), Options{ProxySecretFile: short}, io.Discard); err == nil {
		t.Error("a short secret is refused")
	}
}

func TestListenRefusesOpenAddressWithoutSecret(t *testing.T) {
	tr := testutil.New(t)
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skip(err)
	}
	s := newServer(t, newApp(t, tr), Options{Listener: ln})
	if _, err := s.Listen(); err == nil || !strings.Contains(err.Error(), "proxy-secret-file") {
		t.Fatalf("want a refusal, got %v", err)
	}
}

func TestOwnerListenerAcceptsTheOwner(t *testing.T) {
	if !peerCheckAvailable {
		t.Skip("peer check is Linux only")
	}
	tr := testutil.New(t)
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t, newApp(t, tr), Options{Listener: raw})
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ln.(*ownerListener); !ok {
		t.Fatalf("loopback without a secret wraps the listener, got %T", ln)
	}
	srv := &http.Server{Handler: s.Handler()}
	go srv.Serve(ln)
	defer srv.Close()
	req, _ := http.NewRequest("GET", "http://"+raw.Addr().String()+"/api/state", nil)
	req.Header.Set(RequestHeader, "1")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("the owner's own connection: %d", resp.StatusCode)
	}
}

func TestProcAddr(t *testing.T) {
	if !peerCheckAvailable {
		t.Skip("Linux only")
	}
	a := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	if got := procAddr(a, false); got != "0100007F:1F90" {
		t.Errorf("ipv4 = %s", got)
	}
	if got := procAddr(&net.TCPAddr{IP: net.ParseIP("::1"), Port: 8080}, true); got != "00000000000000000000000001000000:1F90" {
		t.Errorf("ipv6 = %s", got)
	}
	if got := procAddr(a, true); got != "0000000000000000FFFF00000100007F:1F90" {
		t.Errorf("mapped = %s", got)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

func body(v any) string {
	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(v)
	return b.String()
}

func TestEditCommitPushFlow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	tr := testutil.New(t)
	origin := filepath.Join(tr.Root, "origin.git")
	git(t, tr.Root, "init", "-q", "--bare", "-b", "master", origin)
	app1 := filepath.Join(tr.Code, "app")
	git(t, tr.Code, "clone", "-q", origin, app1)
	// agentmd commits with the owner's identity; the test machine may have none.
	git(t, app1, "config", "user.name", "T")
	git(t, app1, "config", "user.email", "t@example.com")
	p := tr.File(filepath.Join(app1, "AGENTS.md"), "# App\nOld rule.\n")
	tr.Link(filepath.Join(app1, "CLAUDE.md"), "AGENTS.md")
	git(t, app1, "add", "AGENTS.md", "CLAUDE.md")
	git(t, app1, "commit", "-q", "-m", "init")
	git(t, app1, "push", "-q", "origin", "HEAD:master")
	git(t, app1, "branch", "-q", "--set-upstream-to=origin/master")

	a := newApp(t, tr)
	h := newServer(t, a, Options{}).Handler()

	st := decode[model.State](t, do(t, h, call{method: "GET", path: "/api/state"}))
	if len(st.Files) == 0 || st.Owner != "alex" {
		t.Fatalf("state = %+v", st)
	}
	link := filepath.Join(app1, "CLAUDE.md")
	fr := decode[fileResponse](t, do(t, h, call{method: "GET", path: "/api/file?id=" + link}))
	if fr.Content != "# App\nOld rule.\n" || fr.File.Hash != discover.Hash(fr.Content) {
		t.Fatalf("file = %+v", fr)
	}

	// Save through the link.
	rec := do(t, h, call{method: "PUT", path: "/api/file", body: body(saveRequest{ID: link, Content: "# App\nNew rule.\n", BaseHash: fr.File.Hash})})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	sr := decode[saveResponse](t, rec)
	if sr.Repo != app1 || !strings.Contains(sr.Diff, "+New rule.") || sr.File.Hash != discover.Hash("# App\nNew rule.\n") {
		t.Errorf("save response = %+v", sr)
	}
	if data, _ := os.ReadFile(p); string(data) != "# App\nNew rule.\n" {
		t.Errorf("disk = %q", data)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("CLAUDE.md must still be a link")
	}

	// A stale save conflicts and carries the disk version.
	rec = do(t, h, call{method: "PUT", path: "/api/file", body: body(saveRequest{ID: link, Content: "x", BaseHash: fr.File.Hash})})
	if rec.Code != 409 {
		t.Fatalf("stale save: %d", rec.Code)
	}
	if c := decode[conflictResponse](t, rec); c.Current != "# App\nNew rule.\n" {
		t.Errorf("conflict = %+v", c)
	}

	gr := decode[gitResponse](t, do(t, h, call{method: "GET", path: "/api/git?id=" + link}))
	if !gr.Dirty || gr.Branch != "master" || gr.Upstream != "origin/master" {
		t.Errorf("git = %+v", gr)
	}
	rec = do(t, h, call{method: "POST", path: "/api/commit", body: body(commitRequest{IDs: []string{link}, Message: "Change the rule"})})
	if rec.Code != 200 {
		t.Fatalf("commit: %d %s", rec.Code, rec.Body)
	}
	cr := decode[commitResponse](t, rec)
	if len(cr.Commit) != 40 {
		t.Errorf("commit = %+v", cr)
	}
	gr = decode[gitResponse](t, do(t, h, call{method: "GET", path: "/api/git?id=" + link}))
	if gr.Ahead != 1 || gr.Dirty {
		t.Errorf("after commit = %+v", gr)
	}
	rec = do(t, h, call{method: "POST", path: "/api/push", body: body(pushRequest{ID: link})})
	if rec.Code != 200 {
		t.Fatalf("push: %d %s", rec.Code, rec.Body)
	}

	// Apply checks every base hash first.
	cur := "# App\nNew rule.\n"
	rec = do(t, h, call{method: "POST", path: "/api/apply", body: body(applyRequest{Edits: []saveRequest{{ID: p, Content: "# App\n", BaseHash: "stale"}}})})
	if rec.Code != 409 {
		t.Errorf("apply with a stale hash: %d", rec.Code)
	}
	rec = do(t, h, call{method: "POST", path: "/api/apply", body: body(applyRequest{Edits: []saveRequest{{ID: p, Content: "# App\n", BaseHash: discover.Hash(cur)}}})})
	if rec.Code != 200 {
		t.Errorf("apply: %d %s", rec.Code, rec.Body)
	}
}

func TestJobs(t *testing.T) {
	tr := testutil.New(t)
	h := newServer(t, newApp(t, tr), Options{}).Handler()
	rec := do(t, h, call{method: "POST", path: "/api/probe", body: `{"contexts":["claude:` + tr.Code + `"]}`})
	if rec.Code != 202 {
		t.Fatalf("probe: %d %s", rec.Code, rec.Body)
	}
	j := decode[model.Job](t, rec)
	deadline := time.Now().Add(5 * time.Second)
	for j.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		j = decode[model.Job](t, do(t, h, call{method: "GET", path: "/api/jobs/" + j.ID}))
	}
	if j.Status != "error" || !strings.Contains(j.Error, "not installed") {
		t.Errorf("probing without claude installed: %+v", j)
	}
	rec = do(t, h, call{method: "POST", path: "/api/analyse", body: `{"context":"agents-md:/nowhere"}`})
	j = decode[model.Job](t, rec)
	for j.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		j = decode[model.Job](t, do(t, h, call{method: "GET", path: "/api/jobs/" + j.ID}))
	}
	if j.Status != "error" {
		t.Errorf("analysing without a CLI: %+v", j)
	}
	if rec := do(t, h, call{method: "GET", path: "/api/jobs/nope"}); rec.Code != 404 {
		t.Errorf("unknown job: %d", rec.Code)
	}
}

func TestJSONIsGzippedWhenAccepted(t *testing.T) {
	tr := testutil.New(t)
	h := newServer(t, newApp(t, tr), Options{}).Handler()
	rec := do(t, h, call{method: "GET", path: "/api/state", headers: map[string]string{"Accept-Encoding": "gzip, br"}})
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("status %d encoding %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var st model.State
	if err := json.NewDecoder(zr).Decode(&st); err != nil || st.Owner != "alex" {
		t.Errorf("decoded %+v, %v", st, err)
	}
	if rec := do(t, h, call{method: "GET", path: "/api/state"}); rec.Header().Get("Content-Encoding") != "" {
		t.Error("no gzip unless asked")
	}
}

func TestIdentityMatching(t *testing.T) {
	cases := []struct {
		allowed, header string
		want            bool
	}{
		{"vbarzin", "vbarzin", true},
		{"vbarzin", "vbarzin@gmail.com", true},
		{"vbarzin", "VBarzin@Gmail.com", true},
		{"vbarzin", "vbarzin2@gmail.com", false},
		{"vbarzin", "mallory", false},
		{"vbarzin", "", false},
		{"vbarzin", "@gmail.com", false},
		{"alice@example.com", "alice@example.com", true},
		{"alice@example.com", "Alice@Example.com", true},
		{"alice@example.com", "alice", false},
		{"alice@example.com", "alice@other.com", false},
	}
	for _, c := range cases {
		if got := identityMatches(c.allowed, c.header); got != c.want {
			t.Errorf("identityMatches(%q, %q) = %v, want %v", c.allowed, c.header, got, c.want)
		}
	}
}

func TestProxyModeNamesTheRefusedIdentity(t *testing.T) {
	tr := testutil.New(t)
	secretFile := filepath.Join(tr.Root, "secret")
	os.WriteFile(secretFile, []byte("0123456789abcdef0123456789abcdef\n"), 0o600)
	var logs bytes.Buffer
	s, err := New(newApp(t, tr), Options{ProxySecretFile: secretFile, IdentityHeader: "X-Authentik-Username",
		AllowIdentities: []string{"alex"}, UI: ui}, &logs)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	secret := "0123456789abcdef0123456789abcdef"
	ok := do(t, h, call{method: "GET", path: "/api/state", host: "x", headers: map[string]string{SecretHeader: secret, "X-Authentik-Username": "alex@example.com"}})
	if ok.Code != 200 {
		t.Errorf("the full username of an allowed local part gets in: %d %s", ok.Code, ok.Body)
	}
	no := do(t, h, call{method: "GET", path: "/", host: "x", headers: map[string]string{SecretHeader: secret, "X-Authentik-Username": "mallory@example.com"}})
	if no.Code != 403 || !strings.Contains(no.Body.String(), "mallory@example.com") {
		t.Errorf("a refusal names the identity the proxy sent: %d %s", no.Code, no.Body)
	}
	if !strings.Contains(logs.String(), "mallory@example.com") {
		t.Errorf("the refused identity is logged: %q", logs.String())
	}
}
