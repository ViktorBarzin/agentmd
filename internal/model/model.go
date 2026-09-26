// Package model holds the types agentmd passes between discovery, loading,
// findings, the HTTP API and the CLI. The terms follow CONTEXT.md.
package model

import "time"

// Kind says what an agent file is for.
type Kind string

const (
	KindInstruction Kind = "instruction"
	KindSkill       Kind = "skill"
	KindSubagent    Kind = "subagent"
	KindCommand     Kind = "command"
	KindDoc         Kind = "doc"
)

// Scope says who a file belongs to.
type Scope string

const (
	ScopeOrg     Scope = "org"
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopePlugin  Scope = "plugin"
	ScopeOther   Scope = "other"
)

// Harness names.
const (
	HarnessClaude   = "claude"
	HarnessCodex    = "codex"
	HarnessAgentsMD = "agents-md"
)

// File is one agent file. ID is the path it was discovered at, or
// "<path>#<field>" for an embedded file.
type File struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	RealPath    string   `json:"realPath"`
	Display     string   `json:"display"`
	Field       string   `json:"field,omitempty"`
	Kind        Kind     `json:"kind"`
	Scope       Scope    `json:"scope"`
	Harnesses   []string `json:"harnesses"`
	Repo        string   `json:"repo,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Size        int      `json:"size"`
	Lines       int      `json:"lines"`
	Hash        string   `json:"hash"`
	IsLink      bool     `json:"isLink,omitempty"`
	LinkTarget  string   `json:"linkTarget,omitempty"`
	Missing     bool     `json:"missing,omitempty"`
	Access      Access   `json:"access"`
	// Content is loaded on demand and never serialised with the inventory.
	Content string `json:"-"`
}

// Access says who can edit a file and where edits belong.
type Access struct {
	Writable  bool     `json:"writable"`
	Reason    string   `json:"reason,omitempty"`
	Owner     string   `json:"owner,omitempty"`
	BuiltFrom []string `json:"builtFrom,omitempty"`
	BuiltBy   string   `json:"builtBy,omitempty"`
	Origin    *Origin  `json:"origin,omitempty"`
	Managed   *Managed `json:"managed,omitempty"`
}

// Origin is where edits to a copy belong.
type Origin struct {
	FileID string `json:"fileId,omitempty"`
	Path   string `json:"path"`
	Field  string `json:"field,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Managed names the tool that maintains a file.
type Managed struct {
	Tool   string `json:"tool"`
	Source string `json:"source,omitempty"`
	Note   string `json:"note,omitempty"`
	// Warn is true when edits in place would be lost on the tool's next run.
	Warn bool `json:"warn,omitempty"`
}

// RefKind is one of the four kinds of reference.
type RefKind string

const (
	RefSymlink RefKind = "symlink"
	RefBuild   RefKind = "build"
	RefMention RefKind = "mention"
	RefLoad    RefKind = "load"
)

// Ref is a directed reference between two files. To is a file ID, or the
// resolved path when the reference dangles.
type Ref struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Kind     RefKind `json:"kind"`
	Sub      string  `json:"sub,omitempty"`
	Line     int     `json:"line,omitempty"`
	Text     string  `json:"text,omitempty"`
	Dangling bool    `json:"dangling,omitempty"`
	Context  string  `json:"context,omitempty"`
}

// Context is a runtime context: what one harness loads in one directory.
type Context struct {
	ID        string         `json:"id"`
	Harness   string         `json:"harness"`
	Dir       string         `json:"dir"`
	Display   string         `json:"display"`
	Source    string         `json:"source"` // "probe" or "static"
	Version   string         `json:"version,omitempty"`
	ProbedAt  *time.Time     `json:"probedAt,omitempty"`
	Entries   []ContextEntry `json:"entries"`
	Skills    []ContextItem  `json:"skills"`
	Subagents []ContextItem  `json:"subagents"`
	Bytes     int            `json:"bytes"`
	Error     string         `json:"error,omitempty"`
	Analysis  *AnalysisInfo  `json:"analysis,omitempty"`
}

// AnalysisInfo records the last analysis of a context.
type AnalysisInfo struct {
	At       time.Time `json:"at"`
	CLI      string    `json:"cli"`
	Stale    bool      `json:"stale,omitempty"`
	Findings int       `json:"findings"`
}

// ContextEntry is one loaded file, in load order.
type ContextEntry struct {
	FileID    string `json:"fileId"`
	Label     string `json:"label,omitempty"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
	LostBytes int    `json:"lostBytes,omitempty"`
}

// ContextItem is a skill or subagent a context offers.
type ContextItem struct {
	Name   string `json:"name"`
	FileID string `json:"fileId,omitempty"`
}

// Severity of a finding.
const (
	Problem = "problem"
	Hint    = "hint"
)

// Finding kinds.
const (
	FindDuplicate     = "duplicate"
	FindReworded      = "reworded"
	FindContradiction = "contradiction"
	FindDangling      = "dangling"
	FindBudget        = "budget"
	FindNotLoaded     = "not-loaded"
)

// Finding is something worth cleaning up.
type Finding struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Summary  string   `json:"summary"`
	Detail   string   `json:"detail,omitempty"`
	Spans    []Span   `json:"spans"`
	Contexts []string `json:"contexts,omitempty"`
	Source   string   `json:"source"` // "rule" or "analysis"
}

// Span is a line range in one file, 1-based and inclusive.
type Span struct {
	FileID    string `json:"fileId"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Quote     string `json:"quote,omitempty"`
}

// HarnessInfo says whether a harness CLI is installed.
type HarnessInfo struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Probe     bool   `json:"probe"`
}

// State is the whole model the UI and `agentmd check --json` receive.
type State struct {
	Owner     string        `json:"owner"`
	Home      string        `json:"home"`
	Roots     []string      `json:"roots"`
	ScannedAt time.Time     `json:"scannedAt"`
	Harnesses []HarnessInfo `json:"harnesses"`
	Files     []File        `json:"files"`
	Refs      []Ref         `json:"refs"`
	Contexts  []Context     `json:"contexts"`
	Findings  []Finding     `json:"findings"`
	Analysis  string        `json:"analysis,omitempty"` // CLI used for analysis, empty when none
}

// Job is a long-running analysis or fix proposal.
type Job struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`   // "analyse" or "fix"
	Status     string     `json:"status"` // "running", "done" or "error"
	Context    string     `json:"context,omitempty"`
	FindingID  string     `json:"findingId,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
	Findings   []Finding  `json:"findings,omitempty"`
	Proposal   *Proposal  `json:"proposal,omitempty"`
}

// Proposal is a fix proposal: whole-file before and after for each file it touches.
type Proposal struct {
	FindingID string         `json:"findingId"`
	Rationale string         `json:"rationale"`
	Edits     []ProposalEdit `json:"edits"`
}

// ProposalEdit is one file in a fix proposal.
type ProposalEdit struct {
	FileID   string `json:"fileId"`
	Display  string `json:"display"`
	BaseHash string `json:"baseHash"`
	Before   string `json:"before"`
	After    string `json:"after"`
}
