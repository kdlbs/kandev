// Package commentkeys centralizes the idempotency-key contract that links
// office comments to queued runs.
package commentkeys

import (
	"encoding/json"
	"strings"
)

const (
	// TaskCommentPrefix prefixes run idempotency keys that originate from an
	// office task comment.
	TaskCommentPrefix = "task_comment:"
	// TaskCommentReason is the run reason used for comment-triggered runs.
	TaskCommentReason = "task_comment"
	// EngineDispatchedValue marks comment events whose synchronous publisher
	// path already routed the trigger through the workflow engine.
	EngineDispatchedValue = "true"
)

// TaskComment builds the canonical same-task comment idempotency key.
func TaskComment(commentID string) string {
	return TaskCommentPrefix + commentID
}

// HasTaskCommentPrefix reports whether key uses the comment idempotency prefix.
func HasTaskCommentPrefix(key string) bool {
	return strings.HasPrefix(key, TaskCommentPrefix)
}

// TrimTaskCommentPrefix removes the comment idempotency prefix when present.
func TrimTaskCommentPrefix(key string) string {
	return strings.TrimPrefix(key, TaskCommentPrefix)
}

// CommentIDFromKey extracts the leading comment id from canonical and salted
// task_comment keys. Salted keys append extra colon-separated fields after the
// comment id.
func CommentIDFromKey(key string) string {
	if !HasTaskCommentPrefix(key) {
		return ""
	}
	id := TrimTaskCommentPrefix(key)
	if before, _, found := strings.Cut(id, ":"); found {
		return before
	}
	return id
}

// IsSaltedTaskCommentKey reports whether a task_comment key carries extra
// salt after the leading comment id.
func IsSaltedTaskCommentKey(key string) bool {
	if !HasTaskCommentPrefix(key) {
		return false
	}
	return strings.Contains(TrimTaskCommentPrefix(key), ":")
}

// IdentityFromPayload extracts the task/comment pair used to link a run back
// to a comment. Cross-task wakes execute on task_id but the UI badge/comment
// anchor belongs to source_task_id, so source_task_id wins when present.
func IdentityFromPayload(payloadJSON string) (taskID, commentID string) {
	if payloadJSON == "" {
		return "", ""
	}
	var p struct {
		TaskID       string `json:"task_id"`
		SourceTaskID string `json:"source_task_id"`
		CommentID    string `json:"comment_id"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return "", ""
	}
	if p.SourceTaskID != "" {
		taskID = p.SourceTaskID
	} else {
		taskID = p.TaskID
	}
	return taskID, p.CommentID
}
