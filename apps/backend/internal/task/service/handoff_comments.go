package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

// commentBodyMaxBytes and commentResponseBudgetBytes are the per-comment
// and aggregate byte caps required by REQ-OFFICE-AGENT-COMMENT-READS-004.
const (
	commentBodyMaxBytes        = 8192
	commentResponseBudgetBytes = 65536
)

// errCommentReaderNotConfigured is returned when ListCommentsForCaller is
// called before SetCommentReader wires a backing store. Deliberately not a
// shared sentinel like ErrAccessDenied / ErrDocumentTaskRequired — a caller
// must never confuse "dependency unconfigured" with either of those
// (AC-005.2/AC-005.3).
var errCommentReaderNotConfigured = errors.New("comment reader not configured")

// CommentProjection is the wire shape of a single comment returned by
// ListCommentsForCaller. It deliberately omits ReplyChannelID and any
// per-comment run lifecycle field (AC-002.4).
type CommentProjection struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	AuthorType    string    `json:"author_type"`
	AuthorID      string    `json:"author_id"`
	Source        string    `json:"source"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
	BodyTruncated bool      `json:"body_truncated,omitempty"`
	BodyBytes     int       `json:"body_bytes,omitempty"`
}

// CommentWindow is the response envelope for ListCommentsForCaller.
type CommentWindow struct {
	Comments []CommentProjection `json:"comments"`
	Total    int                 `json:"total"`
	Returned int                 `json:"returned"`
	HasMore  bool                `json:"has_more"`
}

// ListCommentsForCaller returns targetTaskID's comments after the same
// read-access guard the document tools use. targetTaskID may be empty,
// whitespace-only, or the literal "self" (after trimming) to mean
// callerTaskID (AC-005.4/005.6/005.8); when that resolves to empty because
// callerTaskID is also empty, this returns ErrDocumentTaskRequired
// (AC-005.5) rather than falling through to a plain access denial.
func (s *HandoffService) ListCommentsForCaller(ctx context.Context, callerTaskID, targetTaskID string, limit int) (*CommentWindow, error) {
	resolvedTarget := strings.TrimSpace(targetTaskID)
	if resolvedTarget == "self" || resolvedTarget == "" {
		resolvedTarget = callerTaskID
	}
	if resolvedTarget == "" {
		return nil, ErrDocumentTaskRequired
	}

	ok, err := canReadDocuments(ctx, repoTaskLookupAdapter{r: s.tasks}, blockerLookupAdapter{repo: s.blockers}, callerTaskID, resolvedTarget)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}

	if s.comments == nil {
		return nil, errCommentReaderNotConfigured
	}
	rows, total, err := s.comments.ListTaskCommentsWindow(ctx, resolvedTarget, limit)
	if err != nil {
		return nil, err
	}

	projected := make([]CommentProjection, 0, len(rows))
	for _, c := range rows {
		projected = append(projected, projectComment(c))
	}
	projected = trimCommentsToBudget(projected, commentResponseBudgetBytes)

	return &CommentWindow{
		Comments: projected,
		Total:    total,
		Returned: len(projected),
		HasMore:  len(projected) < total,
	}, nil
}

// trimCommentsToBudget drops whole comments from the oldest end (index 0)
// of an ascending-ordered slice until the summed body bytes fit within
// budgetBytes, per AC-004.6/004.7/004.8. The `len(comments)-start > 1`
// guard is deliberately generic rather than relying on the current
// 8192/65536 constants: even if those change, a non-empty window is never
// reduced to empty (AC-004.9/004.10) — the single newest comment always
// survives.
func trimCommentsToBudget(comments []CommentProjection, budgetBytes int) []CommentProjection {
	total := 0
	for _, c := range comments {
		total += len(c.Body)
	}
	start := 0
	for total > budgetBytes && len(comments)-start > 1 {
		total -= len(comments[start].Body)
		start++
	}
	return comments[start:]
}

// runeSafeTruncateUTF8 cuts s to at most maxBytes at a rune boundary,
// returning valid UTF-8. Deliberately duplicates
// internal/office/truncate.UTF8 instead of importing it: ARCH-TASK-OFFICE-IMPORT
// forbids internal/task/ files from adding new internal/office imports, and
// this file has no other reason to cross that boundary.
func runeSafeTruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
