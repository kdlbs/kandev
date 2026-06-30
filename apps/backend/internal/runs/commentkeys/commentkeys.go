// Package commentkeys centralizes the idempotency-key contract that links
// office comments to queued runs.
package commentkeys

import "strings"

const (
	// TaskCommentPrefix prefixes run idempotency keys that originate from an
	// office task comment.
	TaskCommentPrefix = "task_comment:"
	// TaskCommentReason is the run reason used for comment-triggered runs.
	TaskCommentReason = "task_comment"
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
