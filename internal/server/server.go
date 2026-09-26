// Package server is agentmd's HTTP API and UI host, with the access checks
// from ADR-0002: loopback with no token by default, a proxy secret otherwise.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ViktorBarzin/agentmd/internal/app"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/edit"
	"github.com/ViktorBarzin/agentmd/internal/gitx"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// DefaultListen is where `agentmd serve` listens unless told otherwise.
const DefaultListen = "127.0.0.1:7390"

// Header names.
const (
	RequestHeader = "X-Agentmd-Request"
	SecretHeader  = "X-Agentmd-Proxy-Secret"
)

// Options configures the server.
type Options struct {
	Listen          string
	ProxySecretFile string
	IdentityHeader  string
	AllowIdentities []string
	Open            bool
	Version         string
	// UI is the built UI to serve.
	UI fs.FS
	// Listener, when set, is used instead of listening on Listen (tests and
	// systemd socket activation).
	Listener net.Listener
}

// Server holds the app and the running jobs.
type Server struct {
	app    *app.App
	opts   Options
	secret []byte
	proxy  bool
	logger *log.Logger

	mu   sync.Mutex
	jobs map[string]*model.Job
}

// New validates the options and returns a server ready to serve.
func New(a *app.App, opts Options, logw io.Writer) (*Server, error) {
	s := &Server{app: a, opts: opts, jobs: map[string]*model.Job{}, logger: log.New(logw, "agentmd: ", 0)}
	if opts.ProxySecretFile != "" {
		data, err := os.ReadFile(opts.ProxySecretFile)
		if err != nil {
			return nil, fmt.Errorf("read the proxy secret: %w", err)
		}
		s.secret = []byte(strings.TrimSpace(string(data)))
		if len(s.secret) < 16 {
			return nil, errors.New("the proxy secret must be at least 16 characters")
		}
		s.proxy = true
		if s.opts.IdentityHeader == "" {
			s.opts.IdentityHeader = "X-Forwarded-User"
		}
		if len(s.opts.AllowIdentities) == 0 {
			s.opts.AllowIdentities = []string{a.Options().Owner}
		}
	}
	return s, nil
}

// Listen opens the listener: a socket passed by systemd, the one in Options,
// or a new one on Listen. It refuses a non-loopback address without a secret.
func (s *Server) Listen() (net.Listener, error) {
	ln := s.opts.Listener
	if ln == nil {
		var err error
		ln, err = systemdListener()
		if err != nil {
			return nil, err
		}
	}
	if ln == nil {
		addr := s.opts.Listen
		if addr == "" {
			addr = DefaultListen
		}
		var err error
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	loopback := ok && tcp.IP.IsLoopback()
	if !loopback && !s.proxy {
		ln.Close()
		return nil, fmt.Errorf("refusing to listen on %s without --proxy-secret-file: only a loopback address is safe without one", ln.Addr())
	}
	if loopback && !s.proxy && peerCheckAvailable {
		ln = &ownerListener{Listener: ln, uid: uint32(os.Getuid()), logger: s.logger}
	}
	return ln, nil
}

// systemdListener returns the socket systemd passed (LISTEN_FDS), if any.
func systemdListener() (net.Listener, error) {
	if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return nil, nil
	}
	n, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || n < 1 {
		return nil, nil
	}
	f := os.NewFile(3, "systemd-socket")
	ln, err := net.FileListener(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("use the socket from systemd: %w", err)
	}
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")
	return ln, nil
}

// ownerListener drops connections whose socket belongs to another OS user.
type ownerListener struct {
	net.Listener
	uid    uint32
	logger *log.Logger
	warned sync.Once
}

func (l *ownerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		local, ok1 := c.LocalAddr().(*net.TCPAddr)
		remote, ok2 := c.RemoteAddr().(*net.TCPAddr)
		if !ok1 || !ok2 {
			return c, nil
		}
		uid, found, err := peerUID(local, remote)
		if found && uid == l.uid {
			return c, nil
		}
		l.warned.Do(func() {
			l.logger.Printf("refused a connection from another OS user (uid %d, %v); only the owner may connect", uid, err)
		})
		c.Close()
	}
}

// Handler returns the full HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("POST /api/scan", s.scan)
	mux.HandleFunc("GET /api/file", s.getFile)
	mux.HandleFunc("PUT /api/file", s.putFile)
	mux.HandleFunc("GET /api/git", s.git)
	mux.HandleFunc("POST /api/commit", s.commit)
	mux.HandleFunc("POST /api/push", s.push)
	mux.HandleFunc("POST /api/probe", s.probe)
	mux.HandleFunc("POST /api/analyse", s.analyse)
	mux.HandleFunc("POST /api/fix", s.fix)
	mux.HandleFunc("GET /api/jobs/{id}", s.job)
	mux.HandleFunc("POST /api/apply", s.apply)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	if s.opts.UI != nil {
		mux.Handle("/", spa(s.opts.UI))
	}
	return s.guard(mux)
}

// guard applies the access checks and security headers to every request.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			writeError(w, http.StatusMethodNotAllowed, "cross-origin requests are not accepted")
			return
		}
		if s.proxy {
			got := []byte(r.Header.Get(SecretHeader))
			if subtle.ConstantTimeCompare(got, s.secret) != 1 {
				writeError(w, http.StatusUnauthorized, "missing or incorrect proxy secret")
				return
			}
			id := r.Header.Get(s.opts.IdentityHeader)
			allowed := false
			for _, a := range s.opts.AllowIdentities {
				if id != "" && id == a {
					allowed = true
				}
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "this instance belongs to another user")
				return
			}
		} else if !loopbackHost(r.Host) {
			writeError(w, http.StatusMisdirectedRequest, "agentmd answers only on localhost")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Header.Get(RequestHeader) != "1" {
				writeError(w, http.StatusForbidden, "missing the "+RequestHeader+" header")
				return
			}
			if r.Method != http.MethodGet && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeError(w, http.StatusUnsupportedMediaType, "send JSON")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost accepts localhost, 127.0.0.1 and [::1], with any port.
func loopbackHost(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// spa serves the built UI, answering unknown paths with index.html.
func spa(ui fs.FS) http.Handler {
	files := http.FileServer(http.FS(ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := ui.Open(p); err == nil {
				f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		data, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			http.Error(w, "the UI is not built into this binary", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

// Serve runs until ctx ends.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	url := "http://" + ln.Addr().String()
	mode := "loopback only"
	if s.proxy {
		mode = "behind a proxy (identity header " + s.opts.IdentityHeader + ")"
	}
	s.logger.Printf("serving %s for %s, %s", url, s.app.Options().Owner, mode)
	if s.opts.Open && !s.proxy {
		openBrowser(url)
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func openBrowser(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	_ = exec.Command(name, url).Start()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 16<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.State()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.Scan()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type fileResponse struct {
	File    model.File `json:"file"`
	Content string     `json:"content"`
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	f, err := s.app.File(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	content := f.Content
	if !f.Missing {
		if cur, err := edit.Current(f); err == nil {
			content = cur
		}
	}
	file := *f
	file.Hash = discover.Hash(content)
	writeJSON(w, http.StatusOK, fileResponse{File: file, Content: content})
}

type saveRequest struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	BaseHash string `json:"baseHash"`
}

type saveResponse struct {
	File    model.File `json:"file"`
	Content string     `json:"content"`
	Repo    string     `json:"repo"`
	Diff    string     `json:"diff"`
}

type conflictResponse struct {
	Error   string `json:"error"`
	Current string `json:"current"`
	Hash    string `json:"hash"`
}

func (s *Server) putFile(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, status, err := s.save(r.Context(), req)
	if err != nil {
		var c *edit.Conflict
		if errors.As(err, &c) {
			writeJSON(w, http.StatusConflict, conflictResponse{Error: err.Error(), Current: c.Current, Hash: c.Hash})
			return
		}
		writeError(w, status, err.Error())
		return
	}
	if _, err := s.app.Scan(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.refresh(r.Context(), res)
	writeJSON(w, http.StatusOK, res)
}

// save writes one file and returns the pieces of the response that do not
// depend on the rescan.
func (s *Server) save(ctx context.Context, req saveRequest) (*saveResponse, int, error) {
	f, err := s.app.File(req.ID)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	written, err := edit.Save(f, req.Content, req.BaseHash)
	if err != nil {
		if errors.Is(err, edit.ErrReadOnly) {
			return nil, http.StatusForbidden, err
		}
		return nil, http.StatusInternalServerError, err
	}
	resp := &saveResponse{File: *f, Content: req.Content}
	if root, err := gitx.Root(ctx, written); err == nil {
		resp.Repo = root
		if d, err := gitx.Diff(ctx, root, written); err == nil {
			resp.Diff = d
		}
	}
	return resp, http.StatusOK, nil
}

// refresh replaces the file in a save response with the rescanned one.
func (s *Server) refresh(ctx context.Context, res *saveResponse) {
	if f, err := s.app.File(res.File.ID); err == nil {
		res.File = *f
	}
	res.File.Hash = discover.Hash(res.Content)
}

type gitResponse struct {
	Repo     string `json:"repo"`
	Diff     string `json:"diff"`
	Dirty    bool   `json:"dirty"`
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
}

func (s *Server) repoOf(ctx context.Context, id string) (string, string, error) {
	f, err := s.app.File(id)
	if err != nil {
		return "", "", err
	}
	written, err := edit.Target(f)
	if err != nil {
		return "", "", err
	}
	root, err := gitx.Root(ctx, written)
	if err != nil {
		return "", written, err
	}
	return root, written, nil
}

func (s *Server) git(w http.ResponseWriter, r *http.Request) {
	root, written, err := s.repoOf(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		if errors.Is(err, gitx.ErrNotInRepo) {
			writeJSON(w, http.StatusOK, gitResponse{})
			return
		}
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	resp := gitResponse{Repo: root}
	resp.Diff, _ = gitx.Diff(r.Context(), root, written)
	if st, err := gitx.Info(r.Context(), root, written); err == nil {
		resp.Dirty, resp.Branch, resp.Upstream, resp.Ahead, resp.Behind = st.Dirty, st.Branch, st.Upstream, st.Ahead, st.Behind
	}
	writeJSON(w, http.StatusOK, resp)
}

type commitRequest struct {
	IDs     []string `json:"ids"`
	Message string   `json:"message"`
}

type commitResponse struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
	Output string `json:"output"`
}

func (s *Server) commit(w http.ResponseWriter, r *http.Request) {
	var req commitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var root string
	var paths []string
	for _, id := range req.IDs {
		rt, written, err := s.repoOf(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s: %v", id, err))
			return
		}
		if root != "" && rt != root {
			writeError(w, http.StatusBadRequest, "these files are in different repositories; commit them one repository at a time")
			return
		}
		root = rt
		paths = append(paths, written)
	}
	sha, out, err := gitx.Commit(r.Context(), root, paths, req.Message)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error() + "\n" + out})
		return
	}
	writeJSON(w, http.StatusOK, commitResponse{Repo: root, Commit: sha, Output: out})
}

type pushRequest struct {
	ID string `json:"id"`
}

type pushResponse struct {
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Output   string `json:"output"`
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	var req pushRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	root, _, err := s.repoOf(r.Context(), req.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, out, err := gitx.Push(r.Context(), root)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pushResponse{Repo: root, Branch: st.Branch, Upstream: st.Upstream, Output: out})
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// startJob runs fn in the background and returns the job right away.
func (s *Server) startJob(kind string, init func(*model.Job), fn func(ctx context.Context, j *model.Job) error) *model.Job {
	j := &model.Job{ID: newID(), Kind: kind, Status: "running", StartedAt: time.Now()}
	if init != nil {
		init(j)
	}
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.pruneJobs()
	s.mu.Unlock()
	go func() {
		err := fn(context.Background(), j)
		s.mu.Lock()
		defer s.mu.Unlock()
		now := time.Now()
		j.FinishedAt = &now
		if err != nil {
			j.Status, j.Error = "error", err.Error()
		} else {
			j.Status = "done"
		}
	}()
	return s.snapshot(j)
}

// pruneJobs drops finished jobs older than an hour. The caller holds s.mu.
func (s *Server) pruneJobs() {
	for id, j := range s.jobs {
		if j.FinishedAt != nil && time.Since(*j.FinishedAt) > time.Hour {
			delete(s.jobs, id)
		}
	}
}

func (s *Server) snapshot(j *model.Job) *model.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *j
	return &c
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	j, ok := s.jobs[r.PathValue("id")]
	var c model.Job
	if ok {
		c = *j
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "no such job")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type probeRequest struct {
	Contexts []string `json:"contexts"`
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	j := s.startJob("probe", nil, func(ctx context.Context, j *model.Job) error {
		failed := 0
		_, err := s.app.Probe(ctx, req.Contexts, func(done, total int, id string, err error) {
			s.mu.Lock()
			j.Done, j.Total = done, total
			if err != nil {
				failed++
			}
			s.mu.Unlock()
		})
		s.mu.Lock()
		allFailed := j.Total > 0 && failed == j.Total
		s.mu.Unlock()
		if err != nil && (len(req.Contexts) == 1 || allFailed) {
			return err
		}
		if allFailed {
			return errors.New("every probe failed; see each context's error")
		}
		return nil
	})
	writeJSON(w, http.StatusAccepted, j)
}

type analyseRequest struct {
	Context string `json:"context"`
}

func (s *Server) analyse(w http.ResponseWriter, r *http.Request) {
	var req analyseRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	j := s.startJob("analyse", func(j *model.Job) { j.Context = req.Context }, func(ctx context.Context, j *model.Job) error {
		fs, err := s.app.Analyse(ctx, req.Context)
		if err != nil {
			return err
		}
		s.mu.Lock()
		j.Findings = fs
		s.mu.Unlock()
		return nil
	})
	writeJSON(w, http.StatusAccepted, j)
}

type fixRequest struct {
	FindingID string `json:"findingId"`
}

func (s *Server) fix(w http.ResponseWriter, r *http.Request) {
	var req fixRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	j := s.startJob("fix", func(j *model.Job) { j.FindingID = req.FindingID }, func(ctx context.Context, j *model.Job) error {
		p, err := s.app.Propose(ctx, req.FindingID)
		if err != nil {
			return err
		}
		s.mu.Lock()
		j.Proposal = p
		s.mu.Unlock()
		return nil
	})
	writeJSON(w, http.StatusAccepted, j)
}

type applyRequest struct {
	Edits []saveRequest `json:"edits"`
}

type applyResponse struct {
	Results []saveResponse `json:"results"`
}

// apply saves several files, checking every base hash before writing any.
func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, e := range req.Edits {
		f, err := s.app.File(e.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if !f.Access.Writable {
			writeError(w, http.StatusForbidden, fmt.Sprintf("%s is read-only: %s", f.Display, f.Access.Reason))
			return
		}
		cur, err := edit.Current(f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if h := discover.Hash(cur); h != e.BaseHash {
			writeJSON(w, http.StatusConflict, conflictResponse{Error: f.Display + " changed on disk since the proposal was made", Current: cur, Hash: h})
			return
		}
	}
	var out applyResponse
	for _, e := range req.Edits {
		res, status, err := s.save(r.Context(), e)
		if err != nil {
			writeError(w, status, err.Error())
			return
		}
		out.Results = append(out.Results, *res)
	}
	if _, err := s.app.Scan(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range out.Results {
		s.refresh(r.Context(), &out.Results[i])
	}
	writeJSON(w, http.StatusOK, out)
}
