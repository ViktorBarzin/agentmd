// Package gitx runs the few git commands agentmd needs: find the repository,
// show a diff, commit named paths, and fast-forward push.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Timeout bounds one git command. A commit may run hooks, so it gets longer.
const (
	Timeout       = 30 * time.Second
	CommitTimeout = 3 * time.Minute
)

// ErrNotInRepo means the path is not inside a git repository.
var ErrNotInRepo = errors.New("not in a git repository")

func run(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "LC_ALL=C")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// Root returns the top of the repository holding path.
func Root(ctx context.Context, path string) (string, error) {
	dir := path
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		dir = filepath.Dir(path)
	}
	out, _, err := run(ctx, Timeout, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotInRepo
	}
	return strings.TrimSpace(out), nil
}

// Diff returns the working-tree diff of path against HEAD, or the whole file
// as added when git does not track it yet.
func Diff(ctx context.Context, root, path string) (string, error) {
	status, _, err := run(ctx, Timeout, root, "status", "--porcelain", "--", path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(status, "??") {
		out, errOut, err := run(ctx, Timeout, root, "diff", "--no-color", "--no-index", "--", "/dev/null", path)
		var exit *exec.ExitError
		if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
			return "", fmt.Errorf("git diff: %s", strings.TrimSpace(errOut))
		}
		return out, nil
	}
	out, errOut, err := run(ctx, Timeout, root, "diff", "--no-color", "HEAD", "--", path)
	if err != nil {
		return "", fmt.Errorf("git diff: %s", strings.TrimSpace(errOut))
	}
	return out, nil
}

// Status describes the checked-out branch.
type Status struct {
	Branch   string
	Upstream string
	Ahead    int
	Behind   int
	Dirty    bool
}

// Info reads the branch, its upstream, how far apart they are, and whether
// path has uncommitted changes.
func Info(ctx context.Context, root, path string) (Status, error) {
	var s Status
	out, _, err := run(ctx, Timeout, root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return s, err
	}
	s.Branch = strings.TrimSpace(out)
	if up, _, err := run(ctx, Timeout, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		s.Upstream = strings.TrimSpace(up)
		if counts, _, err := run(ctx, Timeout, root, "rev-list", "--left-right", "--count", "HEAD...@{u}"); err == nil {
			f := strings.Fields(counts)
			if len(f) == 2 {
				s.Ahead, _ = strconv.Atoi(f[0])
				s.Behind, _ = strconv.Atoi(f[1])
			}
		}
	}
	if path != "" {
		st, _, err := run(ctx, Timeout, root, "status", "--porcelain", "--", path)
		if err == nil {
			s.Dirty = strings.TrimSpace(st) != ""
		}
	}
	return s, nil
}

// Commit stages and commits exactly the given paths, leaving anything else
// that is staged alone. Hooks run as usual.
func Commit(ctx context.Context, root string, paths []string, message string) (string, string, error) {
	if strings.TrimSpace(message) == "" {
		return "", "", errors.New("a commit needs a message")
	}
	if len(paths) == 0 {
		return "", "", errors.New("nothing to commit")
	}
	args := append([]string{"add", "--"}, paths...)
	if _, errOut, err := run(ctx, Timeout, root, args...); err != nil {
		return "", errOut, fmt.Errorf("git add: %s", strings.TrimSpace(errOut))
	}
	args = append([]string{"commit", "-m", message, "--"}, paths...)
	out, errOut, err := run(ctx, CommitTimeout, root, args...)
	output := strings.TrimSpace(out + "\n" + errOut)
	if err != nil {
		return "", output, fmt.Errorf("git commit failed")
	}
	sha, _, err := run(ctx, Timeout, root, "rev-parse", "HEAD")
	if err != nil {
		return "", output, err
	}
	return strings.TrimSpace(sha), output, nil
}

// ErrRejected means git refused the push; the output says why.
var ErrRejected = errors.New("git refused the push")

// Push sends the current branch to its upstream branch. Git refuses anything
// that is not a fast-forward, and agentmd never forces.
func Push(ctx context.Context, root string) (Status, string, error) {
	s, err := Info(ctx, root, "")
	if err != nil {
		return s, "", err
	}
	if s.Upstream == "" {
		return s, "", fmt.Errorf("%s has no upstream branch to push to", s.Branch)
	}
	remote, branch, ok := strings.Cut(s.Upstream, "/")
	if !ok {
		return s, "", fmt.Errorf("cannot read the upstream %q", s.Upstream)
	}
	out, errOut, err := run(ctx, CommitTimeout, root, "push", "--porcelain", remote, "HEAD:refs/heads/"+branch)
	output := strings.TrimSpace(out + "\n" + errOut)
	if err != nil {
		return s, output, fmt.Errorf("%w: %s", ErrRejected, lastLines(output, 6))
	}
	after, _ := Info(ctx, root, "")
	return after, output, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
