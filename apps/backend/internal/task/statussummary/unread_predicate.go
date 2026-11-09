package statussummary

import (
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// unreadVisibleMessageTypes is the transcript-visible type set. It must equal
// apps/packages/types/src/unread-message-visible-types.json, which the web
// transcript filter reads; the contract test enforces exact equality.
var unreadVisibleMessageTypes = map[models.MessageType]struct{}{
	models.MessageTypeMessage:         {},
	models.MessageTypeContent:         {},
	models.MessageTypeToolCall:        {},
	models.MessageTypeToolRead:        {},
	models.MessageTypeToolEdit:        {},
	models.MessageTypeToolExecute:     {},
	models.MessageTypeToolSearch:      {},
	models.MessageTypeProgress:        {},
	models.MessageTypeStatus:          {},
	models.MessageTypeError:           {},
	models.MessageTypeThinking:        {},
	models.MessageTypeTodo:            {},
	models.MessageTypeScriptExecution: {},
	models.MessageTypeAgentPlan:       {},
}

const (
	unreadStatusNewSession = "New session started"
	unreadStatusResumed    = "Session resumed"
	unreadSetupScriptType  = "setup"
)

// UnreadCandidateMessageTypes returns, sorted, every stored message type that
// can qualify, including the empty type that IsUnreadQualifyingMessage treats
// as a message. A row whose type is absent from this list, or whose author is
// models.MessageAuthorUser, never qualifies, so a query may exclude it before
// the full predicate runs.
func UnreadCandidateMessageTypes() []string {
	types := make([]string, 0, len(unreadVisibleMessageTypes)+1)
	types = append(types, "")
	for kind := range unreadVisibleMessageTypes {
		types = append(types, string(kind))
	}
	sort.Strings(types)
	return types
}

// IsUnreadQualifyingMessage reports whether a stored or published message can
// make its session unread: a transcript-visible row, not written by the user,
// that the read cursor can reach. Setup output, subagent children, session
// start/resume notices, recovery cards, footer-action rows, and prompt
// requests never qualify.
func IsUnreadQualifyingMessage(authorType, messageType, content string, metadata map[string]interface{}) bool {
	if authorType == messageTypeUser {
		return false
	}
	kind := models.MessageType(messageType)
	if kind == "" {
		kind = models.MessageTypeMessage
	}
	if _, ok := unreadVisibleMessageTypes[kind]; !ok {
		return false
	}
	if kind == models.MessageTypeScriptExecution && stringField(metadata, "script_type") == unreadSetupScriptType {
		return false
	}
	if stringField(metadata, "parent_tool_call_id") != "" {
		return false
	}
	if kind == models.MessageTypeStatus && (content == unreadStatusNewSession || content == unreadStatusResumed) {
		return false
	}
	if boolValue(metadata["recovery_actions"]) {
		return false
	}
	return !isSliceValue(metadata["actions"])
}

// isSliceValue matches a JSON array whether it was decoded from storage
// ([]interface{}) or carried in-process with a concrete element type.
func isSliceValue(value interface{}) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// UnreadRank orders transcript messages by (created_at, id), the order the
// read-cursor write guard uses. CreatedAt is normalized to UTC microseconds so
// a nanosecond event timestamp and its database value compare equal.
type UnreadRank struct {
	ID        string
	CreatedAt time.Time
}

// NewUnreadRank builds a normalized rank.
func NewUnreadRank(id string, createdAt time.Time) UnreadRank {
	if createdAt.IsZero() {
		return UnreadRank{ID: id}
	}
	return UnreadRank{ID: id, CreatedAt: createdAt.UTC().Truncate(time.Microsecond)}
}

// After reports whether r is strictly later than other. Equal ids are the
// same message regardless of timestamp precision.
func (r UnreadRank) After(other UnreadRank) bool {
	if r.ID == other.ID {
		return false
	}
	if !r.CreatedAt.Equal(other.CreatedAt) {
		return r.CreatedAt.After(other.CreatedAt)
	}
	return strings.Compare(r.ID, other.ID) > 0
}

// SessionUnreadState is one session's read cursor rank and its newest
// qualifying rank. A session without a cursor has a zero Read rank, so any
// qualifying message makes it unread; Newest is zero when nothing qualifies.
// Newest equals Read when nothing qualifies after a cursor. A cursor whose
// message was deleted has a zero CreatedAt.
type SessionUnreadState struct {
	Read   UnreadRank
	Newest UnreadRank
}

// Unread reports whether the newest qualifying message is after the cursor.
func (s SessionUnreadState) Unread() bool {
	return s.Newest.After(s.Read)
}
