package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/common/truncate"
)

// commentBodyMaxBytes and commentResponseBudgetBytes are the per-comment
// and aggregate byte caps required by REQ-OFFICE-AGENT-COMMENT-READS-004.
const (
	commentBodyMaxBytes        = 8192
	commentResponseBudgetBytes = 65536
)

// commentWindowDefaultLimit and commentWindowMaxLimit bound the comment
// window returned to callers (default 20, clamp 100). The dashboard HTTP
// handler (handler_comments.go) parses its "limit" query parameter with
// strconv.Atoi and discards the error, so a missing, non-numeric, zero, or
// negative value all reach this method as 0 or negative with no validation
// of its own — clamp here so any non-positive or oversized limit degrades to
// a bounded window instead of reaching SQLite's LIMIT clause raw (LIMIT 0
// returns nothing, a negative LIMIT returns the entire table).
const (
	commentWindowDefaultLimit = 20
	commentWindowMaxLimit     = 100
)

// clampCommentWindowLimit normalizes a caller-supplied limit to the
// documented [1, commentWindowMaxLimit] window, defaulting a non-positive
// value to commentWindowDefaultLimit.
func clampCommentWindowLimit(limit int) int {
	if limit <= 0 {
		return commentWindowDefaultLimit
	}
	if limit > commentWindowMaxLimit {
		return commentWindowMaxLimit
	}
	return limit
}

// errCommentReaderNotConfigured is returned when ListCommentsForCaller is
// called before SetCommentReader wires a backing store. Deliberately not a
// shared sentinel like ErrAccessDenied / ErrDocumentTaskRequired — a caller
// must never confuse "dependency unconfigured" with either of those
// (AC-005.2/AC-005.3).
var errCommentReaderNotConfigured = errors.New("comment reader not configured")

// CommentRecord is the task-service-owned projection returned by a comment
// store. It contains only fields needed by the handoff read contract and does
// not expose an Office persistence model to this service.
type CommentRecord struct {
	ID         string
	TaskID     string
	AuthorType string
	AuthorID   string
	Source     string
	Body       string
	CreatedAt  time.Time
}

// CommentReader is the minimal read surface HandoffService.ListCommentsForCaller
// depends on. Persistence adapters convert their storage model to CommentRecord.
type CommentReader interface {
	ListTaskCommentsWindow(ctx context.Context, taskID string, limit int) ([]CommentRecord, int, error)
}

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
// read-access guard the document tools use. There is no "self"/empty
// sentinel: targetTaskID crosses only from the caller-supplied argument (a
// raw path segment on the HTTP transport, interpolated without validation)
// and is resolved by the guard like any other target
// (AC-OFFICE-AGENT-COMMENT-READS-005.4). A caller with no task identity
// (callerTaskID == "") is denied on this same path for every target, rather
// than surfacing a distinct missing-target validation error
// (AC-OFFICE-AGENT-COMMENT-READS-001.13).
func (s *HandoffService) ListCommentsForCaller(ctx context.Context, callerTaskID, targetTaskID string, limit int) (*CommentWindow, error) {
	ok, err := canReadDocuments(ctx, repoTaskLookupAdapter{r: s.tasks}, blockerLookupAdapter{repo: s.blockers}, callerTaskID, targetTaskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}

	if s.comments == nil {
		return nil, errCommentReaderNotConfigured
	}
	rows, total, err := s.comments.ListTaskCommentsWindow(ctx, targetTaskID, clampCommentWindowLimit(limit))
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

// projectComment maps a stored comment onto the task-service-owned wire
// projection and applies the per-body byte cap.
func projectComment(c CommentRecord) CommentProjection {
	p := CommentProjection{
		ID:         c.ID,
		TaskID:     c.TaskID,
		AuthorType: c.AuthorType,
		AuthorID:   c.AuthorID,
		Source:     c.Source,
		Body:       c.Body,
		CreatedAt:  c.CreatedAt,
	}
	if len(c.Body) > commentBodyMaxBytes {
		p.BodyBytes = len(c.Body)
		p.Body = truncate.UTF8(c.Body, commentBodyMaxBytes)
		p.BodyTruncated = true
	}
	return p
}
