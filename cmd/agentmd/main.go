// Command agentmd shows, checks and edits the markdown files coding agents read.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/ViktorBarzin/agentmd/internal/app"
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
	"github.com/ViktorBarzin/agentmd/internal/server"
	"github.com/ViktorBarzin/agentmd/web"
)

// version is set at build time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `agentmd shows, checks and edits the markdown files coding agents read.

Usage:
  agentmd [serve] [flags]     start the browser UI (the default)
  agentmd check [flags]       print findings; exit 1 when problems exist
  agentmd probe [flags]       record what a harness loads in a directory
  agentmd version

Run "agentmd <command> -h" for the flags of a command.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch cmd {
	case "serve":
		err = serve(ctx, args, stdout, stderr)
	case "check":
		var code int
		code, err = check(ctx, args, stdout, stderr)
		if err == nil {
			return code
		}
	case "probe":
		err = probeCmd(ctx, args, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "agentmd", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "agentmd: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "agentmd:", err)
		return 2
	}
	return 0
}

// common holds the flags every command shares.
type common struct {
	roots   stringList
	configs stringList
	home    string
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func (c *common) register(fs *flag.FlagSet) {
	fs.Var(&c.roots, "root", "a directory tree to search for repositories (repeatable; default ~/code)")
	fs.Var(&c.configs, "config", "a config file to read instead of the defaults (repeatable)")
	fs.StringVar(&c.home, "home", "", "the home directory to read (default: yours)")
}

func (c *common) options() app.Options {
	// AGENTMD_NO_CLIS=1 skips looking for claude and codex, and AGENTMD_ETC
	// replaces /etc, for tests and machines where probing is not wanted.
	o := app.Options{Home: c.home, Etc: os.Getenv("AGENTMD_ETC"), NoCLIs: os.Getenv("AGENTMD_NO_CLIS") == "1"}
	for _, r := range c.roots {
		if abs, err := filepath.Abs(r); err == nil {
			o.Roots = append(o.Roots, abs)
		}
	}
	if len(c.configs) > 0 {
		o.ConfigPaths = c.configs
	}
	return o
}

func check(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	asJSON := fs.Bool("json", false, "print the whole model as JSON")
	doProbe := fs.Bool("probe", false, "probe unprobed and stale contexts first")
	dir := fs.String("dir", "", "only report findings about files under this directory")
	harnessName := fs.String("harness", "", "only report findings from this harness's contexts")
	failOn := fs.String("fail-on", model.Problem, "exit 1 when findings at this level exist: problem, hint or none")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	switch *failOn {
	case model.Problem, model.Hint, "none":
	default:
		return 2, fmt.Errorf("--fail-on must be problem, hint or none")
	}
	a, err := app.New(c.options())
	if err != nil {
		return 2, err
	}
	var st *model.State
	if *doProbe {
		if _, err := a.Scan(); err != nil {
			return 2, err
		}
		st, err = a.Probe(ctx, nil, func(done, total int, id string, err error) {
			if err != nil {
				fmt.Fprintf(stderr, "probe %d/%d %s: %v\n", done, total, id, err)
			} else {
				fmt.Fprintf(stderr, "probe %d/%d %s\n", done, total, id)
			}
		})
	} else {
		st, err = a.Scan()
	}
	if err != nil {
		return 2, err
	}
	st.Findings = filterFindings(st, *dir, *harnessName)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			return 2, err
		}
	} else {
		printCheck(stdout, st)
	}
	for _, f := range st.Findings {
		if *failOn == model.Hint || (*failOn == model.Problem && f.Severity == model.Problem) {
			return 1, nil
		}
	}
	return 0, nil
}

func filterFindings(st *model.State, dir, h string) []model.Finding {
	if dir == "" && h == "" {
		return st.Findings
	}
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	paths := map[string]string{}
	for _, f := range st.Files {
		paths[f.ID] = f.Path
	}
	out := []model.Finding{}
	for _, f := range st.Findings {
		if dir != "" {
			hit := false
			for _, s := range f.Spans {
				if discover.Within(paths[s.FileID], dir) {
					hit = true
				}
			}
			if !hit {
				continue
			}
		}
		if h != "" {
			hit := f.Source == "analysis" && len(f.Contexts) == 0
			for _, c := range f.Contexts {
				if strings.HasPrefix(c, h+":") {
					hit = true
				}
			}
			if !hit && len(f.Contexts) > 0 {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func printCheck(w io.Writer, st *model.State) {
	display := map[string]string{}
	for _, f := range st.Files {
		display[f.ID] = f.Display
	}
	probed, stale, failed := 0, 0, 0
	for _, c := range st.Contexts {
		if c.Source == "probe" {
			probed++
			if c.Stale {
				stale++
			}
			if c.Error != "" {
				failed++
			}
		}
	}
	problems, hints := 0, 0
	for _, f := range st.Findings {
		if f.Severity == model.Problem {
			problems++
		} else {
			hints++
		}
	}
	fmt.Fprintf(w, "%d files, %d references, %d contexts (%d probed, %d stale, %d failed, %d not probed yet)\n",
		len(st.Files), len(st.Refs), len(st.Contexts), probed, stale, failed, len(st.Unprobed))
	fmt.Fprintf(w, "%d problems, %d hints\n", problems, hints)
	if len(st.Unprobed) > 0 {
		fmt.Fprintln(w, "Run with --probe to probe Claude Code and Codex, which decides which files co-load.")
	}
	for _, f := range st.Findings {
		fmt.Fprintf(w, "\n%-7s %-13s %s\n", f.Severity, f.Kind, f.Summary)
		if f.Detail != "" {
			fmt.Fprintf(w, "        %s\n", f.Detail)
		}
		for _, s := range f.Spans {
			d := display[s.FileID]
			if d == "" {
				d = s.FileID
			}
			switch {
			case s.StartLine <= 0:
				fmt.Fprintf(w, "        %s\n", d)
			case s.EndLine > s.StartLine:
				fmt.Fprintf(w, "        %s:%d-%d\n", d, s.StartLine, s.EndLine)
			default:
				fmt.Fprintf(w, "        %s:%d\n", d, s.StartLine)
			}
		}
	}
}

func probeCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	h := fs.String("harness", model.HarnessClaude, "claude, codex or agents-md")
	dir := fs.String("dir", ".", "the directory a session would start in")
	asJSON := fs.Bool("json", false, "print the context as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	a, err := app.New(c.options())
	if err != nil {
		return err
	}
	st, err := a.Scan()
	if err != nil {
		return err
	}
	id := *h + ":" + abs
	if *h != model.HarnessAgentsMD {
		st, err = a.Probe(ctx, []string{id}, nil)
		if err != nil {
			return err
		}
	}
	var found *model.Context
	for i := range st.Contexts {
		if st.Contexts[i].ID == id {
			found = &st.Contexts[i]
		}
	}
	if found == nil {
		return fmt.Errorf("no %s context for %s", *h, abs)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(found)
	}
	display := map[string]string{}
	for _, f := range st.Files {
		display[f.ID] = f.Display
	}
	fmt.Fprintf(stdout, "%s in %s (%s", *h, found.Display, found.Source)
	if found.Version != "" {
		fmt.Fprintf(stdout, ", %s", found.Version)
	}
	fmt.Fprintf(stdout, "): %d bytes\n", found.Bytes)
	for i, e := range found.Entries {
		name := display[e.FileID]
		if name == "" {
			name = e.Label
		}
		fmt.Fprintf(stdout, "  %2d. %-60s %7d B", i+1, name, e.Bytes)
		if e.Truncated {
			fmt.Fprintf(stdout, "  cut, %d B lost", e.LostBytes)
		}
		fmt.Fprintln(stdout)
	}
	for _, s := range found.Skipped {
		fmt.Fprintf(stdout, "  skipped %s: %s\n", display[s.FileID], s.Reason)
	}
	if len(found.Skills) > 0 {
		names := make([]string, 0, len(found.Skills))
		for _, s := range found.Skills {
			names = append(names, s.Name)
		}
		sort.Strings(names)
		fmt.Fprintf(stdout, "  %d skills: %s\n", len(names), strings.Join(names, ", "))
	}
	if len(found.Subagents) > 0 {
		names := make([]string, 0, len(found.Subagents))
		for _, s := range found.Subagents {
			names = append(names, s.Name)
		}
		fmt.Fprintf(stdout, "  %d subagents: %s\n", len(names), strings.Join(names, ", "))
	}
	if found.Error != "" {
		return errors.New(found.Error)
	}
	return nil
}

func serve(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	listen := fs.String("listen", server.DefaultListen, "address to listen on; anything but loopback needs --proxy-secret-file")
	open := fs.Bool("open", false, "open the UI in a browser")
	secret := fs.String("proxy-secret-file", "", "require this shared secret in "+server.SecretHeader+" on every request")
	identity := fs.String("identity-header", "X-Forwarded-User", "with a proxy secret: the header naming the signed-in user")
	var allow stringList
	fs.Var(&allow, "allow-identity", "with a proxy secret: an identity allowed to use this instance (repeatable; default: your user name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.New(c.options())
	if err != nil {
		return err
	}
	if _, err := a.Scan(); err != nil {
		return err
	}
	s, err := server.New(a, server.Options{Listen: *listen, ProxySecretFile: *secret, IdentityHeader: *identity,
		AllowIdentities: allow, Open: *open, Version: version, UI: web.Dist()}, stderr)
	if err != nil {
		return err
	}
	ln, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}
