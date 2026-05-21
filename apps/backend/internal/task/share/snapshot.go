// Package share builds, persists, and uploads frozen snapshots of completed
// task sessions to public locations (currently: GitHub Gists).
package share

import (
	"encoding/json"
	"time"
)

// SnapshotVersion is the current snapshot schema version. Bump when the
// JSON shape changes in a non-backwards-compatible way.
const SnapshotVersion = 1

// Snapshot is a self-contained, frozen JSON representation of a completed
// task session. It MUST NOT contain foreign keys to live kandev rows; it is
// designed to survive deletion of the underlying task, session, or workspace.
type Snapshot struct {
	Version       int          `json:"version"`
	KandevVersion string       `json:"kandev_version,omitempty"`
	ExportedAt    time.Time    `json:"exported_at"`
	Task          TaskMeta     `json:"task"`
	Session       SessionMeta  `json:"session"`
	Messages      []Message    `json:"messages"`
	Redaction     RedactionLog `json:"redaction"`
}

// TaskMeta is the durable bit of the task captured at export time.
type TaskMeta struct {
	Title        string `json:"title"`
	WorkflowStep string `json:"workflow_step,omitempty"`
}

// SessionMeta is the durable bit of the session captured at export time.
type SessionMeta struct {
	AgentType    string     `json:"agent_type,omitempty"`
	Model        string     `json:"model,omitempty"`
	ExecutorType string     `json:"executor_type,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// Message is a single message in the conversation, broken into typed blocks.
type Message struct {
	Role   string    `json:"role"` // "user" | "assistant" | "system"
	Ts     time.Time `json:"ts"`
	Blocks []Block   `json:"blocks"`
}

// Block is a single content unit inside a message. Only the fields relevant
// to the block kind are populated; everything else is omitted.
type Block struct {
	Kind        string          `json:"kind"`                   // "text" | "tool_call" | "tool_result" | "diff"
	Text        string          `json:"text,omitempty"`         // for "text"
	ToolName    string          `json:"tool_name,omitempty"`    // for "tool_call"
	Args        json.RawMessage `json:"args,omitempty"`         // for "tool_call" (already redacted)
	Output      string          `json:"output,omitempty"`       // for "tool_result" (already redacted)
	Truncated   bool            `json:"truncated,omitempty"`    // for "tool_result"
	Path        string          `json:"path,omitempty"`         // for "diff"
	UnifiedDiff string          `json:"unified_diff,omitempty"` // for "diff"
}

// RedactionLog records which redaction rules were triggered while building
// the snapshot. It is part of the snapshot so a reader can tell at a glance
// whether sensitive data was likely scrubbed.
type RedactionLog struct {
	AppliedRules []string `json:"applied_rules"`
}

// Block kind constants — kept private to discourage callers from constructing
// raw Blocks; use the Map* helpers in builder.go.
const (
	blockKindText       = "text"
	blockKindToolCall   = "tool_call"
	blockKindToolResult = "tool_result"
	blockKindDiff       = "diff"

	// Role constants used in Snapshot.Messages.
	roleUser      = "user"
	roleAssistant = "assistant"
	roleSystem    = "system"
)
