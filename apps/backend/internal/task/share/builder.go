package share

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrSessionNotShareable is returned when BuildSnapshot is asked to snapshot
// a session that has not produced any conversation yet (CREATED / STARTING).
// The HTTP handler maps this to 409.
var ErrSessionNotShareable = errors.New("session has no shareable content yet")

// TaskReader is the narrow slice of the task repository BuildSnapshot needs.
// Keeping it small lets us stub it in unit tests without pulling in the full
// task repo interface.
type TaskReader interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
}

// BuildSnapshot loads the task, session, and messages for the given session
// and returns a redacted snapshot. The kandevVersion is recorded in the
// snapshot for forward debugging but is otherwise opaque.
//
// Sessions in CREATED or STARTING state are rejected because they predate
// any agent output. Every other state — RUNNING, IDLE, WAITING_FOR_INPUT,
// COMPLETED, FAILED, CANCELLED — produces a valid snapshot of whatever
// conversation has happened so far.
func BuildSnapshot(ctx context.Context, repo TaskReader, taskSessionID, kandevVersion string) (*Snapshot, error) {
	session, err := repo.GetTaskSession(ctx, taskSessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("load session: %w", ErrSessionNotShareable)
	}
	if !isShareableState(session.State) {
		return nil, ErrSessionNotShareable
	}

	task, err := repo.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("load task: %w", err)
	}

	messages, err := repo.ListMessages(ctx, taskSessionID)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	red := NewRedactor(workspaceRootsFor(session)...)
	snap := &Snapshot{
		Version:       SnapshotVersion,
		KandevVersion: kandevVersion,
		ExportedAt:    time.Now().UTC(),
		Task:          taskMeta(task),
		Session:       sessionMeta(session),
		Messages:      mapMessages(messages, red),
	}
	snap.Redaction.AppliedRules = red.Applied()
	return snap, nil
}

// isShareableState mirrors the frontend gate: any session that has
// progressed past the pre-history states (CREATED, STARTING) is fair game.
func isShareableState(s models.TaskSessionState) bool {
	switch s {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting:
		return false
	}
	return true
}

func taskMeta(t *models.Task) TaskMeta {
	if t == nil {
		return TaskMeta{}
	}
	return TaskMeta{
		Title:        t.Title,
		WorkflowStep: t.WorkflowStepID,
	}
}

func sessionMeta(s *models.TaskSession) SessionMeta {
	return SessionMeta{
		AgentType:    snapshotString(s.AgentProfileSnapshot, "agent_type"),
		Model:        snapshotString(s.AgentProfileSnapshot, "model"),
		ExecutorType: snapshotString(s.ExecutorSnapshot, "type"),
		StartedAt:    s.StartedAt,
		CompletedAt:  s.CompletedAt,
	}
}

// workspaceRootsFor returns every filesystem root the redactor should rewrite
// to repo-relative form. Multi-repo sessions carry multiple worktrees, so we
// emit each WorktreePath plus the session's WorkspacePath fallback — the
// redactor sorts them longest-first internally to handle nested roots.
func workspaceRootsFor(s *models.TaskSession) []string {
	roots := make([]string, 0, len(s.Worktrees)+1)
	for _, w := range s.Worktrees {
		if w != nil && w.WorktreePath != "" {
			roots = append(roots, w.WorktreePath)
		}
	}
	if s.WorkspacePath != "" {
		roots = append(roots, s.WorkspacePath)
	}
	return roots
}

// snapshotString extracts a string field from one of the *Snapshot map
// metadata fields stored on TaskSession. Returns "" if the field is missing
// or is not a string.
func snapshotString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// mapMessages converts the live message rows into snapshot Messages.
// Hidden system content (wrapped in <kandev-system>) is stripped per the
// existing Message.ToAPI() convention before redaction.
func mapMessages(in []*models.Message, red *Redactor) []Message {
	out := make([]Message, 0, len(in))
	for _, m := range in {
		role := roleForAuthor(m.AuthorType, m.Type)
		blocks := blocksForMessage(m, red)
		if len(blocks) == 0 {
			continue
		}
		out = append(out, Message{
			Role:   role,
			Ts:     m.CreatedAt,
			Blocks: blocks,
		})
	}
	return out
}

func roleForAuthor(author models.MessageAuthorType, mt models.MessageType) string {
	switch author {
	case models.MessageAuthorUser:
		return roleUser
	case models.MessageAuthorAgent:
		return roleAssistant
	}
	if mt == models.MessageTypeStatus || mt == models.MessageTypeLog || mt == models.MessageTypeError {
		return roleSystem
	}
	return roleSystem
}

// blocksForMessage decides whether a Message is shareable conversation
// content and converts it into the snapshot Blocks for that role. We
// whitelist what gets through rather than blacklist noise — that way new
// internal message types (tool_search, agent_plan, todo, mock-agent stderr
// captures, …) don't quietly leak into shared snapshots.
//
// What gets through:
//   - User messages of any type → text block (the user's intent is always
//     part of the conversation, even if the runtime tagged it oddly).
//   - Agent messages with no explicit type, "message", or "content"
//     → text block (regular agent prose).
//   - Agent messages whose type starts with "tool_" (tool_call, tool_edit,
//     tool_read, tool_execute, tool_search, …) → collapsed tool block.
//
// Everything else — status, progress, log, error, thinking, agent_plan,
// todo, permission_request, clarification_request, script_execution, plus
// any future type we don't recognise — is dropped.
func blocksForMessage(m *models.Message, red *Redactor) []Block {
	if m.AuthorType == models.MessageAuthorUser {
		return textBlockOrNil(red, m.Content)
	}
	t := string(m.Type)
	if t == "" || t == string(models.MessageTypeMessage) || t == string(models.MessageTypeContent) {
		return textBlockOrNil(red, m.Content)
	}
	if strings.HasPrefix(t, "tool_") {
		return []Block{toolCallBlock(m, red)}
	}
	return nil
}

// textBlockOrNil returns a single text block, or nil if the content is empty
// after trimming. System-injected blocks wrapped in <kandev-system> tags
// (the system prompt the runtime prepends to user turns) are stripped first
// — those carry implementation details that should never appear in a public
// share. After stripping, if nothing is left we drop the message entirely.
func textBlockOrNil(red *Redactor, content string) []Block {
	stripped := sysprompt.StripSystemContent(content)
	text := strings.TrimSpace(stripped)
	if text == "" {
		return nil
	}
	return []Block{{Kind: blockKindText, Text: red.String(text)}}
}

// toolCallBlock builds a "tool_call" Block from a tool-call message. The
// message Content carries a human-readable summary; the Args, when present
// in Metadata, are redacted as a top-level JSON object so RuleEnvVars fires
// for shell payloads.
func toolCallBlock(m *models.Message, red *Redactor) Block {
	b := Block{
		Kind:     blockKindToolCall,
		Text:     red.String(strings.TrimSpace(m.Content)),
		ToolName: toolNameFor(m),
	}
	if raw, ok := toolArgsFor(m); ok {
		b.Args = red.JSON(raw)
	}
	return b
}

// toolNameFor picks the tool label. Production messages don't carry a
// "tool_name" metadata key — service_messages.go writes the normalized
// *streams.NormalizedPayload under metadata["normalized"] and the tool kind
// ends up on Message.Type ("tool_read", "tool_execute", …). Strip the
// "tool_" prefix from Type to get a useful label ("read", "execute", "edit").
// The debug-fixture replay path *does* set metadata["tool_name"], so prefer
// that when present for backward compatibility.
func toolNameFor(m *models.Message) string {
	if name := metaString(m.Metadata, "tool_name"); name != "" {
		return name
	}
	return strings.TrimPrefix(string(m.Type), "tool_")
}

// toolArgsFor picks the args JSON payload. Production stores the typed tool
// payload under metadata["normalized"]; the debug-fixture replay path stores
// it under metadata["args"]. Prefer "args" so existing fixtures keep their
// shape, fall back to "normalized" so real shares have something to redact.
func toolArgsFor(m *models.Message) (json.RawMessage, bool) {
	if raw, ok := metaJSON(m.Metadata, "args"); ok {
		return raw, true
	}
	return metaJSON(m.Metadata, "normalized")
}

func metaString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		s, _ := v.(string)
		return s
	}
	return ""
}

// metaJSON marshals an arbitrary metadata value back to a JSON RawMessage so
// it can be redacted via Redactor.JSON. Returns ok=false if the key is missing.
func metaJSON(m map[string]interface{}, key string) (json.RawMessage, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return raw, true
}
