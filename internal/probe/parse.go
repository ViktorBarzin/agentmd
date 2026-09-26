// Package probe records what Claude Code and Codex load in a directory by
// running them, and parses what they would send to a model.
package probe

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Loaded is one instruction file as a harness names it, with the text it sent.
type Loaded struct {
	// Label is how the harness named the file: a path, or "<managed-settings>".
	Label string
	// Description is Claude's note after the path, such as "project
	// instructions, checked into the codebase".
	Description string
	Content     string
}

// Item is a skill or subagent a harness offers, with a path when the harness
// gives one.
type Item struct {
	Name string
	Path string
}

// ClaudeCapture is what a Claude Code probe recorded.
type ClaudeCapture struct {
	Files     []Loaded
	Skills    []Item
	Subagents []Item
}

// ErrNoRequest means the stand-in API never received a message request.
var ErrNoRequest = errors.New("the harness sent no message request")

var (
	contentsRe = regexp.MustCompile(`(?m)^Contents of (.+?) \(([^()\n]*(?:\([^()\n]*\)[^()\n]*)*)\):\n\n`)
	skillsHead = "The following skills are available for use with the Skill tool:"
	agentsHead = "Available agent types for the Agent tool:"
	agentLine  = regexp.MustCompile(`^- ([A-Za-z0-9_:.-]+): `)
)

// ParseClaudeRequest reads a recorded /v1/messages request body.
func ParseClaudeRequest(body []byte) (*ClaudeCapture, error) {
	var req struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	var texts []string
	texts = append(texts, textsOf(req.System)...)
	for _, m := range req.Messages {
		texts = append(texts, textsOf(m.Content)...)
	}
	c := &ClaudeCapture{}
	for _, t := range texts {
		if strings.Contains(t, "Contents of ") && len(c.Files) == 0 {
			c.Files = parseContents(t)
		}
		if i := strings.Index(t, skillsHead); i >= 0 && c.Skills == nil {
			c.Skills = parseList(t[i+len(skillsHead):], func(line string) (Item, bool) {
				name := strings.TrimPrefix(line, "- ")
				if j := strings.Index(name, ":"); j >= 0 {
					name = name[:j]
				}
				name = strings.TrimSpace(name)
				return Item{Name: name}, name != ""
			})
		}
		if i := strings.Index(t, agentsHead); i >= 0 && c.Subagents == nil {
			c.Subagents = parseList(t[i+len(agentsHead):], func(line string) (Item, bool) {
				m := agentLine.FindStringSubmatch(line)
				if m == nil {
					return Item{}, false
				}
				return Item{Name: m[1]}, true
			})
		}
	}
	return c, nil
}

// textsOf returns the text blocks of a string or content-block array.
func textsOf(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Text != "" {
			out = append(out, b.Text)
		}
	}
	return out
}

// parseContents splits "Contents of <path> (<description>):\n\n<text>" blocks.
func parseContents(t string) []Loaded {
	locs := contentsRe.FindAllStringSubmatchIndex(t, -1)
	var out []Loaded
	for i, loc := range locs {
		start := loc[1]
		end := len(t)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		} else if j := strings.Index(t[start:], "\n</system-reminder>"); j >= 0 {
			end = start + j
		}
		text := strings.TrimSuffix(t[start:end], "\n\n")
		out = append(out, Loaded{Label: t[loc[2]:loc[3]], Description: t[loc[4]:loc[5]], Content: text})
	}
	return out
}

// parseList reads "- item" lines after a heading until the first line that
// is neither blank-before-items nor an item.
func parseList(rest string, item func(string) (Item, bool)) []Item {
	out := []Item{}
	started := false
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			if started {
				break
			}
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			if started {
				break
			}
			continue
		}
		started = true
		if it, ok := item(line); ok {
			out = append(out, it)
		}
	}
	return out
}

// CodexCapture is what `codex debug prompt-input` printed.
type CodexCapture struct {
	// Instructions is the text inside <INSTRUCTIONS>, with the user file and
	// the project docs joined by Codex.
	Instructions string
	HasBlock     bool
	// OrgPolicy is the <managed_developer_instructions> text, if any.
	OrgPolicy string
	Skills    []Item
}

var (
	rootRe       = regexp.MustCompile("^- `(r\\d+)` = `([^`]+)`")
	codexSkillRe = regexp.MustCompile(`^- ([^:]+): .*\(file: (r\d+)/([^)]+)\)\s*$`)
)

// ParseCodexPromptInput reads the JSON list printed by
// `codex debug prompt-input`.
func ParseCodexPromptInput(data []byte) (*CodexCapture, error) {
	var items []struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	c := &CodexCapture{}
	for _, it := range items {
		for _, t := range textsOf(it.Content) {
			switch {
			case strings.HasPrefix(t, "# AGENTS.md instructions for ") && !c.HasBlock:
				if body, ok := between(t, "<INSTRUCTIONS>\n", "\n</INSTRUCTIONS>"); ok {
					c.Instructions, c.HasBlock = body, true
				} else if body, ok := between(t, "<INSTRUCTIONS>", "</INSTRUCTIONS>"); ok {
					c.Instructions, c.HasBlock = strings.Trim(body, "\n"), true
				}
			case strings.HasPrefix(t, "<managed_developer_instructions>"):
				if body, ok := between(t, "<managed_developer_instructions>", "</managed_developer_instructions>"); ok {
					c.OrgPolicy = strings.Trim(body, "\n")
				}
			case strings.HasPrefix(t, "<skills_instructions>"):
				c.Skills = parseCodexSkills(t)
			}
		}
	}
	return c, nil
}

func between(s, open, close string) (string, bool) {
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.LastIndex(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

func parseCodexSkills(t string) []Item {
	roots := map[string]string{}
	var out []Item
	for _, line := range strings.Split(t, "\n") {
		if m := rootRe.FindStringSubmatch(line); m != nil {
			roots[m[1]] = m[2]
			continue
		}
		if m := codexSkillRe.FindStringSubmatch(line); m != nil {
			path := m[3]
			if r, ok := roots[m[2]]; ok {
				path = strings.TrimSuffix(r, "/") + "/" + m[3]
			}
			out = append(out, Item{Name: strings.TrimSpace(m[1]), Path: path})
		}
	}
	return out
}
