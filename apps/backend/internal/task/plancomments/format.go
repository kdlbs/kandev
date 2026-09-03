// Package plancomments owns canonical prompt formatting for task plan feedback.
package plancomments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const placeholder = "\x00kandev-plan-comments\x00"

const (
	MetadataRefs               = "plan_comment_refs"
	MetadataRequestFingerprint = "plan_comment_request_fingerprint"
	MetadataClientQueueID      = "client_queue_id"
)

// WithPlaceholder reserves the task-comment position in content that may be wrapped later.
func WithPlaceholder(content string) string {
	return placeholder + content
}

// ResolvePlaceholder replaces the server-owned marker with canonical comment Markdown.
func ResolvePlaceholder(content string, comments []*models.TaskPlanComment) (string, error) {
	if strings.Count(content, placeholder) != 1 {
		return "", errors.New("plan comment placeholder must occur exactly once")
	}
	if len(comments) == 0 {
		return "", errors.New("plan comment placeholder requires comments")
	}
	return strings.Replace(content, placeholder, formatMarkdown(comments), 1), nil
}

// Fingerprint returns a stable digest for replay identity data.
func Fingerprint(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal plan comment replay identity: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// MetadataRefsMatch reports whether persisted delivery metadata names exactly refs.
func MetadataRefsMatch(metadata map[string]interface{}, refs []models.TaskPlanCommentRef) bool {
	raw, exists := metadata[MetadataRefs]
	if !exists {
		return len(refs) == 0
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var stored []models.TaskPlanCommentRef
	if err := json.Unmarshal(data, &stored); err != nil || len(stored) != len(refs) {
		return false
	}
	for i := range refs {
		if stored[i] != refs[i] {
			return false
		}
	}
	return true
}

func formatMarkdown(comments []*models.TaskPlanComment) string {
	var out strings.Builder
	out.WriteString("### Plan Comments\n\n")
	for _, comment := range comments {
		if comment.SelectedText != "" {
			out.WriteString("```\n")
			out.WriteString(comment.SelectedText)
			out.WriteString("\n```\n")
		}
		for _, line := range strings.Split(comment.Body, "\n") {
			out.WriteString("> ")
			out.WriteString(line)
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	out.WriteString("---\n\n")
	return out.String()
}
