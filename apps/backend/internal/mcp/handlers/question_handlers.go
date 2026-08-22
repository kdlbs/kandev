package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ClarificationBundleLister is the minimal read surface list_pending_questions_kandev
// and answer_question_kandev need: enumerate a caller's visible unresolved
// clarification bundles (spec D4a/L1-L14) and rebuild a bundle's durable
// question list (spec L3-L5).
type ClarificationBundleLister interface {
	ListUnresolvedClarificationBundles(ctx context.Context, opts models.ListClarificationBundlesOptions) (*models.ClarificationBundlePage, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*models.Message, error)
}

// SetClarificationResolver wires list_pending_questions_kandev and
// answer_question_kandev (external MCP surface only, spec S1/S2). Both tools
// are registered together, and only when both dependencies are set.
func (h *Handlers) SetClarificationResolver(resolver *clarification.Resolver, bundles ClarificationBundleLister) {
	h.clarificationResolver = resolver
	h.clarificationBundles = bundles
}

const (
	// defaultPendingQuestionsLimit and maxPendingQuestionsLimit implement L10:
	// a limit below 1, or absent, defaults to 50; above the cap it clamps to
	// 200 rather than being rejected.
	defaultPendingQuestionsLimit = 50
	maxPendingQuestionsLimit     = 200
)

// listPendingQuestionsRequest is list_pending_questions_kandev's request
// shape (L7, L8, L9, L10). Every field is optional.
type listPendingQuestionsRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	CreatedSince string `json:"created_since"`
	Cursor       string `json:"cursor"`
	Limit        int    `json:"limit"`
}

// pendingQuestionBundleView is one bundle in the L11 response envelope,
// carrying the L3/L4 fields with L4a's "never null" guarantee.
type pendingQuestionBundleView struct {
	PendingID  string                         `json:"pending_id"`
	TaskID     string                         `json:"task_id"`
	SessionID  string                         `json:"session_id"`
	CreatedAt  string                         `json:"created_at"`
	AgeSeconds int64                          `json:"age_seconds"`
	Context    string                         `json:"context"`
	Questions  []clarification.QuestionStatus `json:"questions"`
}

// listPendingQuestionsResponse is L11's response envelope. No grand total is
// provided by design (L11a).
type listPendingQuestionsResponse struct {
	Bundles    []pendingQuestionBundleView `json:"bundles"`
	Count      int                         `json:"count"`
	NextCursor string                      `json:"next_cursor"`
}

// handleListPendingQuestions backs list_pending_questions_kandev.
func (h *Handlers) handleListPendingQuestions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req listPendingQuestionsRequest
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
		}
	}

	opts := models.ListClarificationBundlesOptions{
		WorkspaceID: req.WorkspaceID,
		Limit:       clampPendingQuestionsLimit(req.Limit),
	}

	if req.CreatedSince != "" {
		since, err := time.Parse(time.RFC3339, req.CreatedSince)
		if err != nil {
			// L13: a validation error naming the offending argument.
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "created_since: "+err.Error(), nil)
		}
		opts.CreatedSince = &since
	}

	if req.Cursor != "" {
		createdAt, pendingID, err := decodeBundleCursor(req.Cursor)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "cursor: "+err.Error(), nil)
		}
		opts.CursorCreatedAt = createdAt
		opts.CursorPendingID = pendingID
	}

	if err := h.resolveBundleVisibility(ctx, &opts); err != nil {
		h.logger.Error("failed to resolve clarification bundle visibility", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list pending questions", nil)
	}

	page, err := h.clarificationBundles.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		h.logger.Error("failed to list unresolved clarification bundles", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list pending questions", nil)
	}

	resp, err := h.buildListPendingQuestionsResponse(ctx, page)
	if err != nil {
		h.logger.Error("failed to build list_pending_questions_kandev response", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list pending questions", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// resolveBundleVisibility implements L1a-L1c's caller-visibility resolution:
// an unscoped caller (no identity, or the synthetic auth-disabled identity)
// bypasses the workspace filter entirely; a scoped caller is limited to the
// workspace set service.Service.ListWorkspaces already resolves for them.
func (h *Handlers) resolveBundleVisibility(ctx context.Context, opts *models.ListClarificationBundlesOptions) error {
	if !callerScoped(ctx) {
		opts.Unscoped = true
		return nil
	}
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(workspaces))
	for _, w := range workspaces {
		ids = append(ids, w.ID)
	}
	opts.VisibleWorkspaceIDs = ids
	return nil
}

// buildListPendingQuestionsResponse maps a bundle page to the L11 envelope,
// rebuilding each bundle's question list and context from its durable
// messages (L2, L3, L5).
func (h *Handlers) buildListPendingQuestionsResponse(ctx context.Context, page *models.ClarificationBundlePage) (listPendingQuestionsResponse, error) {
	resp := listPendingQuestionsResponse{Bundles: make([]pendingQuestionBundleView, 0, len(page.Bundles))}
	now := time.Now()
	for _, b := range page.Bundles {
		msgs, err := h.clarificationBundles.FindMessagesByPendingID(ctx, b.PendingID)
		if err != nil {
			return listPendingQuestionsResponse{}, err
		}
		resp.Bundles = append(resp.Bundles, pendingQuestionBundleView{
			PendingID:  b.PendingID,
			TaskID:     b.TaskID,
			SessionID:  b.SessionID,
			CreatedAt:  b.CreatedAt.UTC().Format(time.RFC3339),
			AgeSeconds: ageSecondsFloored(now, b.CreatedAt),
			Context:    bundleContext(msgs),
			Questions:  clarification.BundleQuestionStatuses(msgs),
		})
	}
	resp.Count = len(resp.Bundles)
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeBundleCursor(last.CreatedAt, last.PendingID)
	}
	return resp, nil
}

// bundleContext reads the bundle-wide context string every message in the
// bundle carries under metadata["context"] (backendapp/adapters.go), never
// null per L4a.
func bundleContext(msgs []*models.Message) string {
	for _, m := range msgs {
		if v, ok := m.Metadata["context"].(string); ok {
			return v
		}
	}
	return ""
}

// ageSecondsFloored implements D7: server clock minus created_at, floored at
// 0 so clock skew never reports a negative age.
func ageSecondsFloored(now, createdAt time.Time) int64 {
	age := now.Sub(createdAt)
	if age < 0 {
		return 0
	}
	return int64(age.Seconds())
}

// clampPendingQuestionsLimit implements L10.
func clampPendingQuestionsLimit(limit int) int {
	if limit < 1 {
		return defaultPendingQuestionsLimit
	}
	if limit > maxPendingQuestionsLimit {
		return maxPendingQuestionsLimit
	}
	return limit
}

// encodeBundleCursor/decodeBundleCursor implement L9's opaque cursor: the
// (created_at, pending_id) pair, base64url-encoded so it round-trips through
// a JSON string without further escaping.
func encodeBundleCursor(createdAt time.Time, pendingID string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + pendingID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeBundleCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, parts[1], nil
}

// answerQuestionRequest is answer_question_kandev's request shape (N1):
// pending_id plus either answers or rejected(+reason).
type answerQuestionRequest struct {
	PendingID    string                 `json:"pending_id"`
	Answers      []clarification.Answer `json:"answers"`
	Rejected     bool                   `json:"rejected"`
	RejectReason string                 `json:"reject_reason"`
}

// answerQuestionResponse is answer_question_kandev's response shape (N3,
// N4). Response is the N3a-normalized JSON produced by
// clarification.SerializeResponse, embedded raw so its own never-omitted,
// never-null guarantees survive this envelope. No "resume" key, matching the
// REST envelope (R10-R12): the winner's delivery/resume path is internal to
// the resolver and not surfaced to callers.
type answerQuestionResponse struct {
	Claimed  bool            `json:"claimed"`
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response"`
}

// handleAnswerQuestion backs answer_question_kandev. It delegates entirely
// to the same ResolveBundle operation the REST endpoint uses (N2), so it
// inherits R1-R9 and A1-A5 without a second code path.
func (h *Handlers) handleAnswerQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req answerQuestionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	if req.PendingID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "pending_id is required", nil)
	}

	res, claimed, err := h.clarificationResolver.ResolveBundle(ctx, req.PendingID, clarification.Outcome{
		Answers:      req.Answers,
		Rejected:     req.Rejected,
		RejectReason: req.RejectReason,
	})
	switch {
	case err == nil:
		return h.answerQuestionSuccessResponse(msg, claimed, res)
	case errors.Is(err, clarification.ErrBundleNotFound):
		// N5: a pending_id the caller cannot access is the same not-found
		// error text as a nonexistent one.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "clarification request not found", nil)
	case clarification.IsValidationError(err):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	case clarification.IsNotActiveError(err):
		// N4a: R2's no-winner branch is a distinct, expected outcome (the
		// bundle was superseded/terminal/expired/malformed with no winner),
		// not an unclassified failure. Mirrors the REST endpoint's
		// IsNotActiveError -> http.StatusConflict mapping (handlers.go).
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), nil)
	default:
		h.logger.Error("failed to resolve clarification bundle", zap.String("pending_id", req.PendingID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to answer question", nil)
	}
}

func (h *Handlers) answerQuestionSuccessResponse(msg *ws.Message, claimed bool, res *clarification.Resolution) (*ws.Message, error) {
	serialized, err := clarification.SerializeResponse(res.Response)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, answerQuestionResponse{
		Claimed:  claimed,
		Status:   res.Status,
		Response: json.RawMessage(serialized),
	})
}

// resolvedByFromContext returns the caller's user ID for the resolution
// row's diagnostic-only resolved_by column, or "" for an unscoped caller —
// no identity in context (internal caller) or the synthetic auth-disabled
// identity. Mirrors clarification.resolvedByFromContext (unexported there).
func resolvedByFromContext(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return ""
	}
	return identity.UserID
}

// callerScoped reports whether ctx carries per-user scoping, per
// resolvedByFromContext's own definition (mirrors the "scoped" terminology of
// task/service's callerScope): when false, visibility is satisfied
// unconditionally (L1c).
func callerScoped(ctx context.Context) bool {
	return resolvedByFromContext(ctx) != ""
}
