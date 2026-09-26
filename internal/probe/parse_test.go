package probe

import (
	"encoding/json"
	"reflect"
	"testing"
)

func claudeBody(t *testing.T, userText, systemText string) []byte {
	t.Helper()
	body := map[string]any{
		"model":  "claude-test",
		"system": []map[string]string{{"type": "text", "text": "You are a Claude agent."}},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]string{
				{"type": "text", "text": userText},
				{"type": "text", "text": "agentmd probe"},
			}},
			{"role": "system", "content": []map[string]string{{"type": "text", "text": systemText}}},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseClaudeRequest(t *testing.T) {
	user := "<system-reminder>\nCodebase and user instructions are shown below.\n\n" +
		"Contents of <managed-settings> (organization-managed policy instructions):\n\n# Org\nNo secrets.\n\n" +
		"Contents of /home/alex/.claude/CLAUDE.md (user's private global instructions for all projects):\n\n# User\nBe brief.\n\n\n" +
		"Contents of /home/alex/code/app/CLAUDE.md (project instructions, checked into the codebase):\n\n# App\nUse Go.\n</system-reminder>"
	sys := "# Environment\n\nAvailable agent types for the Agent tool:\n- reviewer: Reviews diffs (Tools: Read, Grep)\n- claude: Catch-all (Tools: *)\n\nWhen you launch agents...\n\n" +
		"The following skills are available for use with the Skill tool:\n\n- tdd\n- publish-page: Publish a page\n- context7:docs\n\nMore text."
	c, err := ParseClaudeRequest(claudeBody(t, user, sys))
	if err != nil {
		t.Fatal(err)
	}
	want := []Loaded{
		{Label: "<managed-settings>", Description: "organization-managed policy instructions", Content: "# Org\nNo secrets."},
		{Label: "/home/alex/.claude/CLAUDE.md", Description: "user's private global instructions for all projects", Content: "# User\nBe brief.\n"},
		{Label: "/home/alex/code/app/CLAUDE.md", Description: "project instructions, checked into the codebase", Content: "# App\nUse Go."},
	}
	if !reflect.DeepEqual(c.Files, want) {
		t.Errorf("files =\n%#v\nwant\n%#v", c.Files, want)
	}
	if got := names(c.Skills); !reflect.DeepEqual(got, []string{"tdd", "publish-page", "context7"}) {
		// "context7:docs" keeps only the part before the first colon, like a description.
		t.Errorf("skills = %v", got)
	}
	if got := names(c.Subagents); !reflect.DeepEqual(got, []string{"reviewer", "claude"}) {
		t.Errorf("subagents = %v", got)
	}
}

func names(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

func TestParseClaudeRequestWithoutFiles(t *testing.T) {
	c, err := ParseClaudeRequest(claudeBody(t, "hello", "no lists here"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 0 || len(c.Skills) != 0 || len(c.Subagents) != 0 {
		t.Errorf("want nothing, got %+v", c)
	}
}

func TestParseCodexPromptInput(t *testing.T) {
	skills := "<skills_instructions>\n## Skills\n### Skill roots\n- `r0` = `/home/alex/.agents/skills`\n- `r1` = `/home/alex/.codex/skills/.system`\n### Available skills\n" +
		"- tdd: Test first. (file: r0/tdd/SKILL.md)\n- imagegen: Make images (with care). (file: r1/imagegen/SKILL.md)\n</skills_instructions>"
	items := []map[string]any{
		{"type": "message", "role": "developer", "content": []map[string]string{{"type": "input_text", "text": skills}}},
		{"type": "message", "role": "user", "content": []map[string]string{
			{"type": "input_text", "text": "# AGENTS.md instructions for /home/alex/code/app\n\n<INSTRUCTIONS>\n# User\n\n--- project-doc ---\n\n# App\n</INSTRUCTIONS>"},
			{"type": "input_text", "text": "<environment_context>\n  <cwd>/home/alex/code/app</cwd>\n</environment_context>"},
		}},
		{"type": "message", "role": "developer", "content": []map[string]string{{"type": "input_text", "text": "<managed_developer_instructions>\n# Org\n</managed_developer_instructions>"}}},
	}
	data, _ := json.Marshal(items)
	c, err := ParseCodexPromptInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasBlock || c.Instructions != "# User\n\n--- project-doc ---\n\n# App" {
		t.Errorf("instructions = %q", c.Instructions)
	}
	if c.OrgPolicy != "# Org" {
		t.Errorf("org policy = %q", c.OrgPolicy)
	}
	want := []Item{{Name: "tdd", Path: "/home/alex/.agents/skills/tdd/SKILL.md"}, {Name: "imagegen", Path: "/home/alex/.codex/skills/.system/imagegen/SKILL.md"}}
	if !reflect.DeepEqual(c.Skills, want) {
		t.Errorf("skills = %+v", c.Skills)
	}
}

// Outside a git repository Codex drops the "for <dir>" part of the heading.
func TestParseCodexPromptInputOutsideARepo(t *testing.T) {
	items := []map[string]any{{"type": "message", "role": "user", "content": []map[string]string{
		{"type": "input_text", "text": "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n# User only\n</INSTRUCTIONS>"},
	}}}
	data, _ := json.Marshal(items)
	c, err := ParseCodexPromptInput(data)
	if err != nil || !c.HasBlock || c.Instructions != "# User only" {
		t.Errorf("got %+v, %v", c, err)
	}
}
