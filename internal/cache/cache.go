// Package cache stores probe and analysis results as JSON files under the
// owner's cache directory, readable only by the owner.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Store is a directory of JSON files grouped by kind.
type Store struct {
	Dir string
}

// DefaultDir is $XDG_CACHE_HOME/agentmd or ~/.cache/agentmd.
func DefaultDir(home string) string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "agentmd")
	}
	return filepath.Join(home, ".cache", "agentmd")
}

// Key hashes its parts into a file-name-safe key.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:40]
}

func (s Store) path(kind, key string) string {
	return filepath.Join(s.Dir, kind, key+".json")
}

// Get reads an entry into v. It reports false when there is none or it does
// not parse.
func (s Store) Get(kind, key string, v any) bool {
	if s.Dir == "" {
		return false
	}
	data, err := os.ReadFile(s.path(kind, key))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// Put writes an entry atomically with mode 0600 in a 0700 directory.
func (s Store) Put(kind, key string, v any) error {
	if s.Dir == "" {
		return nil
	}
	dir := filepath.Join(s.Dir, kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(s.Dir, 0o700)
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path(kind, key))
}

// Delete removes an entry.
func (s Store) Delete(kind, key string) {
	if s.Dir != "" && !strings.ContainsAny(key, "/\\") {
		os.Remove(s.path(kind, key))
	}
}
