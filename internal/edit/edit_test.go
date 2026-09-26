package edit

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

const settings = `{
  "model": "claude-x",
  "nested": {"claudeMd": "not this one", "list": ["a}", "b{"]},
  "claudeMd": "# Org — policy\nUse <tags> & \"quotes\".\n",
  "hooks": {"Stop": [{"command": "echo \"}\""}]}
}
`

func TestReplaceJSONFieldKeepsEverythingElse(t *testing.T) {
	out, err := ReplaceJSONField([]byte(settings), "claudeMd", "# New — text\nwith <tags>\n")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(settings, `"# Org — policy\nUse <tags> & \"quotes\".\n"`, `"# New — text\nwith <tags>\n"`, 1)
	if string(out) != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestReplaceJSONFieldMatchesEscapingStyle(t *testing.T) {
	escaped := `{"claudeMd": "café \/ path"}`
	out, err := ReplaceJSONField([]byte(escaped), "claudeMd", "naïve / ok ✓")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"claudeMd": "naïve \/ ok ✓"}` {
		t.Errorf("got %s", out)
	}
}

func TestReplaceJSONFieldErrors(t *testing.T) {
	for _, c := range []struct{ doc, field string }{
		{`{"a": 1}`, "claudeMd"},
		{`{"claudeMd": 7}`, "claudeMd"},
		{`["claudeMd"]`, "claudeMd"},
		{`{"claudeMd": "unterminated}`, "claudeMd"},
	} {
		if _, err := ReplaceJSONField([]byte(c.doc), c.field, "x"); err == nil {
			t.Errorf("%s: want an error", c.doc)
		}
	}
}

// Any string round-trips, and every byte outside the field is unchanged.
func TestReplaceJSONFieldRoundTrips(t *testing.T) {
	alphabet := []rune("ab \n\t\"\\/<>&{}[]:,—é✓😀 \x01")
	gen := func(r *rand.Rand) string {
		n := r.Intn(40)
		rs := make([]rune, n)
		for i := range rs {
			rs[i] = alphabet[r.Intn(len(alphabet))]
		}
		return string(rs)
	}
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		v := gen(r)
		out, err := ReplaceJSONField([]byte(settings), "claudeMd", v)
		if err != nil {
			return false
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(out, &m) != nil {
			return false
		}
		var got string
		if json.Unmarshal(m["claudeMd"], &got) != nil || got != v {
			return false
		}
		s, e, _ := findField([]byte(settings), "claudeMd")
		s2, e2, _ := findField(out, "claudeMd")
		return string(out[:s2]) == settings[:s] && string(out[e2:]) == settings[e:]
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

func writeFile(t *testing.T, p, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestSaveThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "AGENTS.md")
	writeFile(t, real, "old\n", 0o640)
	link := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink("AGENTS.md", link); err != nil {
		t.Fatal(err)
	}
	f := &model.File{ID: link, Path: link, RealPath: real, IsLink: true, LinkTarget: real, Access: model.Access{Writable: true}}
	written, err := Save(f, "new\n", discover.Hash("old\n"))
	if err != nil {
		t.Fatal(err)
	}
	if written != real {
		t.Errorf("wrote %s, want the real file", written)
	}
	if data, _ := os.ReadFile(real); string(data) != "new\n" {
		t.Errorf("content = %q", data)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("the link must stay a link")
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 kept", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestSaveConflictAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	writeFile(t, p, "changed on disk\n", 0o644)
	f := &model.File{ID: p, Path: p, RealPath: p, Access: model.Access{Writable: true}}
	_, err := Save(f, "mine\n", discover.Hash("what I loaded\n"))
	var c *Conflict
	if !errors.As(err, &c) || c.Current != "changed on disk\n" || c.Hash != discover.Hash("changed on disk\n") {
		t.Fatalf("want a conflict carrying the disk version, got %v", err)
	}
	ro := &model.File{ID: p, Path: p, RealPath: p, Access: model.Access{Writable: false, Reason: "owned by root"}}
	if _, err := Save(ro, "x", discover.Hash("changed on disk\n")); !errors.Is(err, ErrReadOnly) || !strings.Contains(err.Error(), "owned by root") {
		t.Errorf("want read-only with the reason, got %v", err)
	}
}

func TestSaveEmbeddedField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "managed-settings.json")
	writeFile(t, p, settings, 0o644)
	cur := "# Org — policy\nUse <tags> & \"quotes\".\n"
	f := &model.File{ID: p + "#claudeMd", Path: p, RealPath: p, Field: "claudeMd", Access: model.Access{Writable: true}}
	if got, err := Current(f); err != nil || got != cur {
		t.Fatalf("current = %q, %v", got, err)
	}
	if _, err := Save(f, "# Edited\n", discover.Hash(cur)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), `"claudeMd": "# Edited\n",`) || !strings.Contains(string(data), `"nested": {"claudeMd": "not this one"`) {
		t.Errorf("file = %s", data)
	}
	if _, err := Save(f, "again", discover.Hash(cur)); err == nil {
		t.Error("a stale base hash conflicts for fields too")
	}
	toml := filepath.Join(dir, "requirements.toml")
	writeFile(t, toml, "additional_developer_instructions = \"x\"\n", 0o644)
	tf := &model.File{ID: toml + "#additional_developer_instructions", Path: toml, RealPath: toml, Field: "additional_developer_instructions", Access: model.Access{Writable: true}}
	if _, err := Save(tf, "y", discover.Hash("x")); !errors.Is(err, ErrReadOnly) {
		t.Errorf("TOML fields are read-only, got %v", err)
	}
}

func TestSaveInAReadOnlyDirectoryWritesInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	p := filepath.Join(dir, "AGENTS.md")
	writeFile(t, p, "old\n", 0o644)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	f := &model.File{ID: p, Path: p, RealPath: p, Access: model.Access{Writable: true}}
	if _, err := Save(f, "new\n", discover.Hash("old\n")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(p); string(data) != "new\n" {
		t.Errorf("content = %q", data)
	}
}
