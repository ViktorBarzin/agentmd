// Package testutil builds throwaway directory trees for tests: a fake home,
// a fake /etc and repositories, with files and symlinks.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Tree is a temp directory laid out like a machine: Home, Etc and Code.
type Tree struct {
	t    *testing.T
	Root string
	Home string
	Etc  string
	Code string
}

// New makes an empty tree.
func New(t *testing.T) *Tree {
	t.Helper()
	root := t.TempDir()
	// Resolve the temp dir itself so paths compare equal after EvalSymlinks
	// (macOS puts temp dirs behind /var -> /private/var).
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	tr := &Tree{t: t, Root: root,
		Home: filepath.Join(root, "home", "alex"),
		Etc:  filepath.Join(root, "etc"),
		Code: filepath.Join(root, "home", "alex", "code"),
	}
	for _, d := range []string{tr.Home, tr.Etc, tr.Code} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return tr
}

// Path joins rel onto the tree root.
func (tr *Tree) Path(rel string) string { return filepath.Join(tr.Root, rel) }

// File writes content at path (absolute, or relative to the tree root) and
// returns the absolute path.
func (tr *Tree) File(path, content string) string {
	tr.t.Helper()
	p := tr.abs(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		tr.t.Fatal(err)
	}
	return p
}

// Link creates a symlink at path pointing at target, which is used verbatim
// (so a relative target stays relative).
func (tr *Tree) Link(path, target string) string {
	tr.t.Helper()
	p := tr.abs(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		tr.t.Fatal(err)
	}
	return p
}

// Dir creates a directory.
func (tr *Tree) Dir(path string) string {
	tr.t.Helper()
	p := tr.abs(path)
	if err := os.MkdirAll(p, 0o755); err != nil {
		tr.t.Fatal(err)
	}
	return p
}

// Repo creates a directory with a .git folder.
func (tr *Tree) Repo(path string) string {
	tr.t.Helper()
	p := tr.Dir(path)
	tr.Dir(filepath.Join(p, ".git"))
	return p
}

func (tr *Tree) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(tr.Root, p)
}
