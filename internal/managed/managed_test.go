package managed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViktorBarzin/agentmd/internal/model"
)

func f(p string) *model.File {
	return &model.File{ID: p, Path: p, RealPath: p, Access: model.Access{Writable: true}}
}

func TestApply(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	os.MkdirAll(agents, 0o755)
	os.WriteFile(filepath.Join(agents, ".skill-lock.json"), []byte(`{"version":3,"skills":{"tdd":{"source":"mattpocock/skills","sourceUrl":"https://github.com/mattpocock/skills.git"}}}`), 0o644)
	core := filepath.Join(agents, "core.md")
	fakeChezmoi := filepath.Join(home, "chezmoi")
	script := "#!/bin/sh\nif [ \"$1\" = managed ]; then echo " + core + "; exit 0; fi\n" +
		"if [ \"$1\" = source-path ]; then shift; for p in \"$@\"; do echo /src/$(basename $p); done; fi\n"
	os.WriteFile(fakeChezmoi, []byte(script), 0o755)

	plugin := f(filepath.Join(home, ".claude/plugins/marketplaces/x/skills/y/SKILL.md"))
	system := f(filepath.Join(home, ".codex/skills/.system/imagegen/SKILL.md"))
	skill := f(filepath.Join(agents, "skills/tdd/SKILL.md"))
	own := f(filepath.Join(agents, "skills/mine/SKILL.md"))
	chez := f(core)
	embedded := &model.File{ID: "/etc/x.json#claudeMd", Path: "/etc/x.json", RealPath: "/etc/x.json", Field: "claudeMd", Access: model.Access{Writable: true}}
	files := map[string]*model.File{}
	for _, x := range []*model.File{plugin, system, skill, own, chez, embedded} {
		files[x.ID] = x
	}
	m := &Marker{Env: Env{Home: home, CodexHome: filepath.Join(home, ".codex"), ClaudeConfigDir: filepath.Join(home, ".claude"),
		Chezmoi: fakeChezmoi, Lockfiles: []string{filepath.Join(agents, ".skill-lock.json")}}}
	m.Apply(files)

	if plugin.Access.Managed == nil || plugin.Access.Managed.Tool != "plugin" || plugin.Access.Writable {
		t.Errorf("plugin = %+v", plugin.Access)
	}
	if system.Access.Managed == nil || system.Access.Managed.Tool != "codex" || system.Access.Writable {
		t.Errorf("codex system skill = %+v", system.Access)
	}
	if skill.Access.Managed == nil || skill.Access.Managed.Tool != "skills" || !skill.Access.Managed.Warn ||
		skill.Access.Managed.Source != "mattpocock/skills" || !skill.Access.Writable {
		t.Errorf("installed skill = %+v", skill.Access)
	}
	if own.Access.Managed != nil {
		t.Errorf("a skill the lockfile does not name is the owner's own: %+v", own.Access.Managed)
	}
	if chez.Access.Managed == nil || chez.Access.Managed.Tool != "chezmoi" || chez.Access.Managed.Source != "/src/core.md" || chez.Access.Managed.Warn {
		t.Errorf("chezmoi file = %+v", chez.Access.Managed)
	}
	if !strings.Contains(chez.Access.Managed.Note, "chezmoi re-add") {
		t.Errorf("note = %q", chez.Access.Managed.Note)
	}
	if embedded.Access.Managed != nil {
		t.Error("embedded fields are left alone")
	}
}
