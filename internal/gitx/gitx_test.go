package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// repo makes a bare "origin" and a clone of it with one commit on master.
func repo(t *testing.T) (clone, origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	git(t, root, "init", "-q", "--bare", "-b", "master", origin)
	clone = filepath.Join(root, "clone")
	git(t, root, "clone", "-q", origin, clone)
	git(t, clone, "config", "user.name", "T")
	git(t, clone, "config", "user.email", "t@example.com")
	os.WriteFile(filepath.Join(clone, "AGENTS.md"), []byte("# one\n"), 0o644)
	git(t, clone, "add", "AGENTS.md")
	git(t, clone, "commit", "-q", "-m", "first")
	git(t, clone, "push", "-q", "origin", "HEAD:master")
	git(t, clone, "branch", "-q", "--set-upstream-to=origin/master")
	return clone, origin
}

func TestDiffCommitPush(t *testing.T) {
	clone, origin := repo(t)
	ctx := context.Background()
	p := filepath.Join(clone, "AGENTS.md")
	if r, err := Root(ctx, p); err != nil || r != clone {
		t.Fatalf("root = %q, %v", r, err)
	}
	os.WriteFile(p, []byte("# one\n# two\n"), 0o644)
	d, err := Diff(ctx, clone, p)
	if err != nil || !strings.Contains(d, "+# two") {
		t.Fatalf("diff = %q, %v", d, err)
	}
	// Something else is staged; the commit must leave it staged.
	os.WriteFile(filepath.Join(clone, "other.txt"), []byte("x\n"), 0o644)
	git(t, clone, "add", "other.txt")
	sha, _, err := Commit(ctx, clone, []string{p}, "Add a second rule")
	if err != nil || len(sha) != 40 {
		t.Fatalf("commit = %q, %v", sha, err)
	}
	if staged := git(t, clone, "diff", "--cached", "--name-only"); strings.TrimSpace(staged) != "other.txt" {
		t.Errorf("other staged work was touched: %q", staged)
	}
	s, err := Info(ctx, clone, p)
	if err != nil || s.Branch != "master" || s.Upstream != "origin/master" || s.Ahead != 1 || s.Behind != 0 || s.Dirty {
		t.Fatalf("status = %+v, %v", s, err)
	}
	after, out, err := Push(ctx, clone)
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	if after.Ahead != 0 {
		t.Errorf("after push ahead = %d", after.Ahead)
	}
	if got := strings.TrimSpace(git(t, origin, "rev-parse", "master")); got != sha {
		t.Errorf("origin master = %s, want %s", got, sha)
	}
}

func TestPushRefusesNonFastForward(t *testing.T) {
	clone, origin := repo(t)
	ctx := context.Background()
	// Someone else lands first.
	other := filepath.Join(filepath.Dir(origin), "other")
	git(t, filepath.Dir(origin), "clone", "-q", origin, other)
	git(t, other, "config", "user.name", "O")
	git(t, other, "config", "user.email", "o@example.com")
	os.WriteFile(filepath.Join(other, "AGENTS.md"), []byte("# theirs\n"), 0o644)
	git(t, other, "commit", "-q", "-am", "theirs")
	git(t, other, "push", "-q", "origin", "HEAD:master")

	p := filepath.Join(clone, "AGENTS.md")
	os.WriteFile(p, []byte("# mine\n"), 0o644)
	if _, _, err := Commit(ctx, clone, []string{p}, "mine"); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "fetch", "-q")
	_, out, err := Push(ctx, clone)
	if !errors.Is(err, ErrRejected) || !strings.Contains(out, "rejected") {
		t.Fatalf("want a rejection, got %v\n%s", err, out)
	}
}

func TestUntrackedFileDiffAndErrors(t *testing.T) {
	clone, _ := repo(t)
	ctx := context.Background()
	p := filepath.Join(clone, "NEW.md")
	os.WriteFile(p, []byte("brand new\n"), 0o644)
	d, err := Diff(ctx, clone, p)
	if err != nil || !strings.Contains(d, "+brand new") {
		t.Errorf("untracked diff = %q, %v", d, err)
	}
	if _, _, err := Commit(ctx, clone, []string{p}, "  "); err == nil {
		t.Error("an empty message is refused")
	}
	if _, err := Root(ctx, t.TempDir()); !errors.Is(err, ErrNotInRepo) {
		t.Errorf("want ErrNotInRepo, got %v", err)
	}
}
