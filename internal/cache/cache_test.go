package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPutGet(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "agentmd")}
	type v struct{ A int }
	k := Key("claude", "/home/alex/code")
	if s.Get("probe", k, &v{}) {
		t.Fatal("empty store has nothing")
	}
	if err := s.Put("probe", k, v{A: 7}); err != nil {
		t.Fatal(err)
	}
	var got v
	if !s.Get("probe", k, &got) || got.A != 7 {
		t.Fatalf("got %+v", got)
	}
	st, err := os.Stat(filepath.Join(s.Dir, "probe", k+".json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("entry mode = %v, %v", st.Mode(), err)
	}
	dst, _ := os.Stat(s.Dir)
	if dst.Mode().Perm() != 0o700 {
		t.Errorf("cache dir mode = %v", dst.Mode())
	}
	s.Delete("probe", k)
	if s.Get("probe", k, &got) {
		t.Error("deleted entry still there")
	}
}

func TestKeyIsStableAndSeparated(t *testing.T) {
	if Key("a", "bc") == Key("ab", "c") {
		t.Error("parts must not run together")
	}
	if Key("x") != Key("x") || len(Key("x")) != 40 {
		t.Error("keys are stable, 40 hex characters")
	}
}
