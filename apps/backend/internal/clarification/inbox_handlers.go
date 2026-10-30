package clarification

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// inboxWorkspaceAuthorizer is the workspace-scope check the Needs-you Inbox
// read endpoints need (needs-you-inbox design, "Security"): the same rule
// AuthorizeWorkspaceScope already applies everywhere else, reached through a
// narrow interface so this package does not depend on service.Service's full
// surface.
type inboxWorkspaceAuthorizer interface {
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// inboxTaskLookup resolves the task_title/session_state enrichment fields in
// bounded batch reads. Enrichment degrades to empty fields on lookup failure
// rather than failing the page.
type inboxTaskLookup interface {
	GetTasksByIDs(ctx context.Context, ids []string) ([]*taskmodels.Task, error)
	BatchGetSessionsForTasks(ctx context.Context, taskIDs []string) (map[string][]*taskmodels.TaskSession, error)
}

// inboxWorkflowStepReader resolves a task's current workflow step for the
// History tab's step-starts-no-agent label. The clarification package has no
// other workflow dependency; this is that one, narrow read.
type inboxWorkflowStepReader interface {
	GetWorkflowStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
}

// inboxTaskService is the combined dependency the Needs-you Inbox and
// Inbox History handlers need from the task service.
type inboxTaskService interface {
	inboxWorkspaceAuthorizer
	inboxTaskLookup
	inboxWorkflowStepReader
}

// inboxBundleStore is the bounded read/write surface the Needs-you Inbox and
// Inbox History endpoints need: the existing bundle-query machinery, the
// per-user dismiss/snooze sidecar (needs-you-inbox design, "Persistence"),
// and the additive, isolated history read.
// inboxAsyncQuestionReader is the narrow async population the Needs-you Inbox
// merges into the same bounded response and count. The task question service
// implements it; clarification never imports that package.
type inboxAsyncQuestionReader interface {
	ListInboxAsyncQuestions(ctx context.Context, workspaceID, userID string, limit int, cursorAt, cursorID string) (*taskmodels.AsyncQuestionPage, error)
	CountInboxAsyncQuestions(ctx context.Context, workspaceID, userID string) (int, error)
	ListHiddenInboxAsyncQuestions(ctx context.Context, workspaceID, userID string, limit int, now time.Time) ([]*taskmodels.AsyncQuestion, int, *time.Time, error)
	HasParkedWait(ctx context.Context, sessionID string) bool
	AsyncQuestionWorkspace(ctx context.Context, questionID string) (string, bool, error)
}

type inboxBundleStore interface {
	ListUnresolvedClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (*taskmodels.ClarificationBundlePage, error)
	FindMessagesByPendingIDs(ctx context.Context, pendingIDs []string) (map[string][]*taskmodels.Message, error)
	UpsertClarificationInboxSidecar(ctx context.Context, userID, pendingID string, state taskmodels.ClarificationSidecarState, snoozeUntil *time.Time, now time.Time) error
	DeleteClarificationInboxSidecar(ctx context.Context, userID, pendingID string) error
	CountHiddenClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (taskmodels.ClarificationInboxHiddenSummary, error)
	CountVisibleClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (int, error)
	GetClarificationInboxSidecarStates(ctx context.Context, userID string, pendingIDs []string) (map[string]taskmodels.ClarificationInboxHiddenBundle, error)
	ListInboxHistoryBundles(ctx context.Context, opts taskmodels.ListClarificationHistoryOptions) (*taskmodels.ClarificationHistoryPage, error)
	CountInboxHistoryBundles(ctx context.Context, opts taskmodels.ListClarificationHistoryOptions) (int, error)
}

const (
	defaultInboxLimit = 50
	maxInboxLimit     = 200
	inboxReadTimeout  = 10 * time.Second

	inboxStateDismissed = string(taskmodels.ClarificationSidecarDismissed)
	inboxStateSnoozed   = string(taskmodels.ClarificationSidecarSnoozed)

	errInboxInternal = "failed to read needs-you inbox"
)

// inboxSnoozeDurations is the closed set of accepted snooze durations: any
// other value, including a well-formed "2h", is 400
// (design-01#Sidecar-write-and-restore-contracts).
var inboxSnoozeDurations = map[string]time.Duration{
	"1h":  time.Hour,
	"4h":  4 * time.Hour,
	"24h": 24 * time.Hour,
}

// inboxBundleView is one bundle row shared by the main list and the hidden
// enumeration (needs-you-inbox design, "Data and contracts").
type inboxBundleView struct {
	PendingID    string        `json:"pending_id"`
	TaskID       string        `json:"task_id"`
	SessionID    string        `json:"session_id"`
	SessionState string        `json:"session_state"`
	TaskTitle    string        `json:"task_title"`
	CreatedAt    string        `json:"created_at"`
	Context      string        `json:"context"`
	Messages     []*v1.Message `json:"messages"`
}

// inboxListResponse is GET /api/v1/clarification-inbox's response envelope.
type inboxAsyncQuestionView struct {
	QuestionID      string                           `json:"question_id"`
	TaskID          string                           `json:"task_id"`
	SessionID       string                           `json:"session_id"`
	SessionState    string                           `json:"session_state"`
	TaskTitle       string                           `json:"task_title"`
	ParentTaskID    string                           `json:"parent_task_id,omitempty"`
	ParentTaskTitle string                           `json:"parent_task_title,omitempty"`
	Forwarded       bool                             `json:"forwarded"`
	Waiting         bool                             `json:"waiting"`
	Status          string                           `json:"status"`
	Context         string                           `json:"context"`
	CreatedAt       string                           `json:"created_at"`
	Questions       []protocol.Question              `json:"questions"`
	Outcome         *taskmodels.AsyncQuestionOutcome `json:"outcome,omitempty"`
	WithdrawReason  string                           `json:"withdraw_reason,omitempty"`
}

type inboxListResponse struct {
	Bundles          []inboxBundleView        `json:"bundles"`
	AsyncQuestions   []inboxAsyncQuestionView `json:"async_questions"`
	Count            int                      `json:"count"`
	AsyncCount       int                      `json:"async_count"`
	HiddenCount      int                      `json:"hidden_count"`
	NextSnoozeExpiry *string                  `json:"next_snooze_expiry"`
	NextCursor       string                   `json:"next_cursor,omitempty"`
}

// inboxHiddenBundleView is one row of the hidden-bundles enumeration: the
// same shape as inboxBundleView plus why it is hidden.
type inboxHiddenBundleView struct {
	inboxBundleView
	State       string  `json:"state"`
	SnoozeUntil *string `json:"snooze_until"`
}

// inboxHiddenListResponse is GET /api/v1/clarification-inbox/hidden's
// response envelope. total is workspace-wide; count is page-scoped.
type inboxHiddenListResponse struct {
	Bundles        []inboxHiddenBundleView        `json:"bundles"`
	AsyncQuestions []inboxHiddenAsyncQuestionView `json:"async_questions"`
	Count          int                            `json:"count"`
	Total          int                            `json:"total"`
	NextCursor     string                         `json:"next_cursor,omitempty"`
}

type inboxHiddenAsyncQuestionView struct {
	inboxAsyncQuestionView
	State       string  `json:"state"`
	SnoozeUntil *string `json:"snooze_until"`
}

// inboxSidecarPutBody is PUT /api/v1/clarification-inbox/sidecar/:pendingID's
// request body.
type inboxSidecarPutBody struct {
	State          string `json:"state"`
	SnoozeDuration string `json:"snooze_duration"`
}

// respondInboxError writes a {"error": message} JSON body. Routed through one
// helper rather than repeating the gin.H{"error": ...} literal at every call
// site. The "error" key itself already recurs across this package's other,
// pre-existing handlers (handlers.go), so goconst's package-wide count stays
// over threshold regardless; consolidating to this one call site is the
// improvement available without touching that unrelated code.
//
//nolint:goconst // see comment above
func respondInboxError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func (h *Handlers) respondInboxReadError(c *gin.Context, requestCtx context.Context, err error) bool {
	if requestCtx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	c.Header("Retry-After", "2")
	respondInboxError(c, http.StatusServiceUnavailable, errInboxInternal)
	return true
}

// httpListInbox backs GET /api/v1/clarification-inbox.
func (h *Handlers) httpListInbox(c *gin.Context) {
	requestCtx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(requestCtx, inboxReadTimeout)
	defer cancel()
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}

	userID := InboxUserID(ctx)
	now := h.now()
	blockingCursorAt, blockingCursorID := time.Time{}, ""
	asyncCursorAt, asyncCursorID := time.Time{}, ""
	if !cursorCreatedAt.IsZero() {
		kind, storedID := inboxCursorSource(cursorPendingID)
		switch kind {
		case "a":
			// Async sorts before blocking at the same instant, so blocking rows
			// at T remain after the cursor while async resumes by ID.
			blockingCursorAt = cursorCreatedAt
			asyncCursorAt, asyncCursorID = cursorCreatedAt, storedID
		case "b":
			// Blocking sorts after async at T, so async must resume strictly
			// after T to avoid replaying rows before the cursor.
			blockingCursorAt, blockingCursorID = cursorCreatedAt, storedID
			asyncCursorAt = cursorCreatedAt.Add(time.Nanosecond)
		default:
			blockingCursorAt, blockingCursorID = cursorCreatedAt, storedID
			asyncCursorAt = cursorCreatedAt.Add(time.Nanosecond)
		}
	}
	page, err := h.inboxBundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:        true,
		WorkspaceID:     workspaceID,
		CursorCreatedAt: blockingCursorAt,
		CursorPendingID: blockingCursorID,
		Limit:           limit,
		Sidecar:         &taskmodels.ClarificationSidecarFilter{UserID: userID, Now: now},
	})
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to list needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	views, err := h.buildInboxBundleViews(ctx, page.Bundles)
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to hydrate needs-you inbox bundle messages", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	// The hidden-count query failing fails the whole read rather than
	// defaulting hidden_count/next_snooze_expiry, which would misreport both
	// the empty state and the snooze timer.
	summary, err := h.inboxBundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1,
		Sidecar: &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now},
	})
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to count hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	asyncCursorAtText := ""
	if !asyncCursorAt.IsZero() {
		asyncCursorAtText = asyncCursorAt.UTC().Format(time.RFC3339Nano)
	}
	asyncViews, _, asyncHasMore, err := h.buildInboxAsyncViews(
		ctx, userID, workspaceID, limit, asyncCursorAtText, asyncCursorID, now)
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to read async question inbox population", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	asyncCount := 0
	if h.inboxAsync != nil {
		asyncCount, err = h.inboxAsync.CountInboxAsyncQuestions(ctx, workspaceID, userID)
		if err != nil {
			if h.respondInboxReadError(c, requestCtx, err) {
				return
			}
			h.logger.Error("failed to count async question inbox population", zap.Error(err))
			respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
			return
		}
	}
	merged := mergeInboxPage(views, asyncViews, page.HasMore, asyncHasMore, limit)
	asyncHidden := 0
	var asyncNextSnooze *time.Time
	if h.inboxAsync != nil {
		_, asyncHidden, asyncNextSnooze, err = h.inboxAsync.ListHiddenInboxAsyncQuestions(ctx, workspaceID, userID, 1, now)
		if err != nil {
			if h.respondInboxReadError(c, requestCtx, err) {
				return
			}
			h.logger.Error("failed to count hidden async questions", zap.Error(err))
			respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
			return
		}
	}
	blockingTotal, err := h.inboxBundles.CountVisibleClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1,
		Sidecar: &taskmodels.ClarificationSidecarFilter{UserID: userID, Now: now},
	})
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to count visible needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	resp := inboxListResponse{
		Bundles:          merged.bundles,
		AsyncQuestions:   merged.async,
		Count:            blockingTotal + asyncCount,
		AsyncCount:       asyncCount,
		HiddenCount:      summary.HiddenCount + asyncHidden,
		NextSnoozeExpiry: formatOptionalInboxTime(earlierInboxTime(summary.NextSnoozeExpiry, asyncNextSnooze)),
	}
	if merged.hasMore && merged.lastItem != nil {
		resp.NextCursor = encodeInboxCursor(merged.lastItem.createdAt, merged.lastItem.kind+":"+merged.lastItem.id)
	}
	c.JSON(http.StatusOK, resp)
}

// httpListInboxHidden backs GET /api/v1/clarification-inbox/hidden.
func (h *Handlers) httpListInboxHidden(c *gin.Context) {
	requestCtx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(requestCtx, inboxReadTimeout)
	defer cancel()
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}

	userID := InboxUserID(ctx)
	now := h.now()
	sidecar := &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now}
	page, err := h.inboxBundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID,
		CursorCreatedAt: cursorCreatedAt, CursorPendingID: cursorPendingID, Limit: limit,
		Sidecar: sidecar,
	})
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to list hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	resp, err := h.buildInboxHiddenResponse(ctx, userID, page)
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to hydrate hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	summary, err := h.inboxBundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1, Sidecar: sidecar,
	})
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to count hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	asyncViews, asyncHidden, err := h.buildInboxHiddenAsyncViews(ctx, userID, workspaceID, limit, now)
	if err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.logger.Error("failed to read hidden async question population", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	resp.AsyncQuestions = asyncViews
	if len(resp.Bundles)+len(resp.AsyncQuestions) > limit {
		remaining := limit
		if len(resp.Bundles) > remaining {
			resp.Bundles = resp.Bundles[:remaining]
			remaining = 0
		} else {
			remaining -= len(resp.Bundles)
		}
		if len(resp.AsyncQuestions) > remaining {
			resp.AsyncQuestions = resp.AsyncQuestions[:remaining]
		}
	}
	resp.Total = summary.HiddenCount + asyncHidden
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) buildInboxHiddenResponse(
	ctx context.Context, userID string, page *taskmodels.ClarificationBundlePage,
) (inboxHiddenListResponse, error) {
	views, err := h.buildInboxBundleViews(ctx, page.Bundles)
	if err != nil {
		return inboxHiddenListResponse{}, err
	}
	pendingIDs := make([]string, 0, len(page.Bundles))
	for _, b := range page.Bundles {
		pendingIDs = append(pendingIDs, b.PendingID)
	}
	states, err := h.inboxBundles.GetClarificationInboxSidecarStates(ctx, userID, pendingIDs)
	if err != nil {
		return inboxHiddenListResponse{}, err
	}

	resp := inboxHiddenListResponse{Bundles: make([]inboxHiddenBundleView, 0, len(views))}
	for _, view := range views {
		entry := inboxHiddenBundleView{inboxBundleView: view}
		if state, ok := states[view.PendingID]; ok {
			entry.State = string(state.State)
			entry.SnoozeUntil = formatOptionalInboxTime(state.SnoozeUntil)
		}
		resp.Bundles = append(resp.Bundles, entry)
	}
	resp.Count = len(resp.Bundles)
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	return resp, nil
}

// buildInboxAsyncViews reads the additive async population and applies the
// same per-user hiding as blocking rows. Async sidecar identities are
// namespaced so they cannot collide with a blocking pending ID.
func (h *Handlers) buildInboxAsyncViews(
	ctx context.Context, userID, workspaceID string, limit int, cursorAt, cursorID string, now time.Time,
) ([]inboxAsyncQuestionView, int, bool, error) {
	if h.inboxAsync == nil {
		return []inboxAsyncQuestionView{}, 0, false, nil
	}
	page, err := h.inboxAsync.ListInboxAsyncQuestions(ctx, workspaceID, userID, limit, cursorAt, cursorID)
	if err != nil {
		return nil, 0, false, err
	}
	if len(page.Questions) == 0 {
		return []inboxAsyncQuestionView{}, 0, page.HasMore, nil
	}
	states, err := h.inboxAsyncSidecarStates(ctx, userID, page.Questions)
	if err != nil {
		return nil, 0, false, err
	}
	visible, hidden := filterHiddenAsyncQuestions(page.Questions, states, now)
	taskIDs := inboxAsyncQuestionTaskIDs(visible)
	titles := h.batchInboxTaskTitles(ctx, taskIDs)
	sessionStates := h.batchInboxSessionStates(ctx, taskIDs)
	views := make([]inboxAsyncQuestionView, 0, len(visible))
	for _, question := range visible {
		views = append(views, h.buildInboxAsyncView(ctx, question, titles, sessionStates))
	}
	return views, hidden, page.HasMore, nil
}

func (h *Handlers) inboxAsyncSidecarStates(
	ctx context.Context,
	userID string,
	questions []*taskmodels.AsyncQuestion,
) (map[string]taskmodels.ClarificationInboxHiddenBundle, error) {
	keys := make([]string, 0, len(questions))
	for _, question := range questions {
		keys = append(keys, inboxAsyncSidecarKey(question.ID))
	}
	return h.inboxBundles.GetClarificationInboxSidecarStates(ctx, userID, keys)
}

func filterHiddenAsyncQuestions(
	questions []*taskmodels.AsyncQuestion,
	states map[string]taskmodels.ClarificationInboxHiddenBundle,
	now time.Time,
) ([]*taskmodels.AsyncQuestion, int) {
	visible := make([]*taskmodels.AsyncQuestion, 0, len(questions))
	hidden := 0
	for _, question := range questions {
		state, ok := states[inboxAsyncSidecarKey(question.ID)]
		if !ok {
			visible = append(visible, question)
			continue
		}
		if state.State == taskmodels.ClarificationSidecarDismissed ||
			(state.State == taskmodels.ClarificationSidecarSnoozed && state.SnoozeUntil != nil && state.SnoozeUntil.After(now)) {
			hidden++
			continue
		}
		visible = append(visible, question)
	}
	return visible, hidden
}

func inboxAsyncQuestionTaskIDs(questions []*taskmodels.AsyncQuestion) []string {
	taskIDs := make([]string, 0, len(questions)*2)
	seen := make(map[string]bool, len(questions)*2)
	for _, question := range questions {
		for _, id := range []string{question.TaskID, question.ParentTaskID} {
			if id != "" && !seen[id] {
				seen[id] = true
				taskIDs = append(taskIDs, id)
			}
		}
	}
	return taskIDs
}

func (h *Handlers) buildInboxAsyncView(
	ctx context.Context,
	question *taskmodels.AsyncQuestion,
	titles, sessionStates map[string]string,
) inboxAsyncQuestionView {
	return inboxAsyncQuestionView{
		QuestionID:      question.ID,
		TaskID:          question.TaskID,
		SessionID:       question.SessionID,
		SessionState:    sessionStates[question.SessionID],
		TaskTitle:       titles[question.TaskID],
		ParentTaskID:    question.ParentTaskID,
		ParentTaskTitle: titles[question.ParentTaskID],
		Forwarded:       question.Forwarded,
		Waiting:         h.inboxAsync.HasParkedWait(ctx, question.SessionID),
		Status:          string(question.Status),
		Context:         question.Context,
		CreatedAt:       question.CreatedAt.UTC().Format(time.RFC3339Nano),
		Questions:       question.Questions,
		Outcome:         question.Outcome,
		WithdrawReason:  question.WithdrawReason,
	}
}

// buildInboxHiddenAsyncViews enumerates the async subset this operator's own
// dismiss or snooze is hiding, including snoozes that have not expired yet.
func (h *Handlers) buildInboxHiddenAsyncViews(
	ctx context.Context, userID, workspaceID string, limit int, now time.Time,
) ([]inboxHiddenAsyncQuestionView, int, error) {
	if h.inboxAsync == nil {
		return []inboxHiddenAsyncQuestionView{}, 0, nil
	}
	questions, total, _, err := h.inboxAsync.ListHiddenInboxAsyncQuestions(ctx, workspaceID, userID, limit, now)
	if err != nil {
		return nil, 0, err
	}
	if len(questions) == 0 {
		return []inboxHiddenAsyncQuestionView{}, 0, nil
	}
	states, err := h.inboxAsyncSidecarStates(ctx, userID, questions)
	if err != nil {
		return nil, 0, err
	}
	taskIDs := inboxAsyncQuestionTaskIDs(questions)
	titles := h.batchInboxTaskTitles(ctx, taskIDs)
	sessionStates := h.batchInboxSessionStates(ctx, taskIDs)
	views := make([]inboxHiddenAsyncQuestionView, 0, len(questions))
	for _, question := range questions {
		state := states[inboxAsyncSidecarKey(question.ID)]
		views = append(views, inboxHiddenAsyncQuestionView{
			inboxAsyncQuestionView: h.buildInboxAsyncView(ctx, question, titles, sessionStates),
			State:                  string(state.State),
			SnoozeUntil:            formatOptionalInboxTime(state.SnoozeUntil),
		})
	}
	return views, total, nil
}

func inboxAsyncSidecarKey(questionID string) string {
	return "async:" + questionID
}

// buildInboxBundleViews hydrates each bundle's durable messages and
// task_title/session_state enrichment. A bundle whose messages cannot be read
// fails the whole page rather than omitting a row silently; a bundle with
// zero resolvable messages (should not happen given the bundle query's own
// filters) is skipped and logged.
func (h *Handlers) buildInboxBundleViews(
	ctx context.Context, bundles []taskmodels.ClarificationBundleSummary,
) ([]inboxBundleView, error) {
	if len(bundles) == 0 {
		return []inboxBundleView{}, nil
	}
	pendingIDs := make([]string, 0, len(bundles))
	taskIDs := make([]string, 0, len(bundles))
	seenPendingIDs := make(map[string]struct{}, len(bundles))
	seenTaskIDs := make(map[string]struct{}, len(bundles))
	for _, bundle := range bundles {
		if _, seen := seenPendingIDs[bundle.PendingID]; !seen {
			seenPendingIDs[bundle.PendingID] = struct{}{}
			pendingIDs = append(pendingIDs, bundle.PendingID)
		}
		if _, seen := seenTaskIDs[bundle.TaskID]; !seen && bundle.TaskID != "" {
			seenTaskIDs[bundle.TaskID] = struct{}{}
			taskIDs = append(taskIDs, bundle.TaskID)
		}
	}
	messagesByPendingID, err := h.inboxBundles.FindMessagesByPendingIDs(ctx, pendingIDs)
	if err != nil {
		return nil, err
	}
	taskTitles := h.batchInboxTaskTitles(ctx, taskIDs)
	sessionStates := h.batchInboxSessionStates(ctx, taskIDs)

	views := make([]inboxBundleView, 0, len(bundles))
	for _, b := range bundles {
		msgs := messagesByPendingID[b.PendingID]
		if len(msgs) == 0 {
			h.logger.Warn("needs-you inbox bundle has no resolvable messages; omitting from page",
				zap.String("pending_id", b.PendingID))
			continue
		}
		content := bundleContentFromMessages(msgs)
		views = append(views, inboxBundleView{
			PendingID:    b.PendingID,
			TaskID:       b.TaskID,
			SessionID:    b.SessionID,
			CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339Nano),
			Context:      content.Context,
			Messages:     content.Messages,
			TaskTitle:    taskTitles[b.TaskID],
			SessionState: sessionStates[b.SessionID],
		})
	}
	return views, nil
}

func (h *Handlers) batchInboxTaskTitles(ctx context.Context, taskIDs []string) map[string]string {
	titles := make(map[string]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return titles
	}
	tasks, err := h.inboxTasks.GetTasksByIDs(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to batch-load needs-you inbox task titles", zap.Error(err))
		return titles
	}
	for _, task := range tasks {
		if task != nil {
			titles[task.ID] = task.Title
		}
	}
	return titles
}

func (h *Handlers) batchInboxSessionStates(ctx context.Context, taskIDs []string) map[string]string {
	states := make(map[string]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return states
	}
	sessionsByTaskID, err := h.inboxTasks.BatchGetSessionsForTasks(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to batch-load needs-you inbox session states", zap.Error(err))
		return states
	}
	for _, sessions := range sessionsByTaskID {
		for _, session := range sessions {
			if session != nil {
				states[session.ID] = string(session.State)
			}
		}
	}
	return states
}

// orderInboxMessages sorts a copy of msgs into AC .10's canonical order:
// question_index ascending (absent, negative, or non-numeric normalizes to
// zero), ties broken by question_id ascending. This is deliberately its own
// sort rather than a reuse of orderBundleMessages, whose D2/L5 order
// tie-breaks by created_at/message id for MCP resolution semantics and does
// not clamp a negative index to zero; the two consumers have different
// contracts and must not be forced to share one implementation.
func orderInboxMessages(msgs []*taskmodels.Message) []*taskmodels.Message {
	sorted := make([]*taskmodels.Message, len(msgs))
	copy(sorted, msgs)
	sort.SliceStable(sorted, func(i, j int) bool {
		qi, qj := normalizedInboxQuestionIndex(sorted[i].Metadata), normalizedInboxQuestionIndex(sorted[j].Metadata)
		if qi != qj {
			return qi < qj
		}
		return questionIDFromMetadata(sorted[i].Metadata) < questionIDFromMetadata(sorted[j].Metadata)
	})
	return sorted
}

// normalizedInboxQuestionIndex clamps a negative index to zero on top of
// questionIndexFromMetadata's existing absent/non-numeric-to-zero handling,
// so every non-positive-index case collapses to the same rank and the
// question_id tie-break decides order for all of them alike (AC .10).
func normalizedInboxQuestionIndex(meta map[string]any) int {
	if idx := questionIndexFromMetadata(meta); idx > 0 {
		return idx
	}
	return 0
}

// renderInboxMessages projects each message through Message.ToAPI() and
// rewrites its emitted metadata.question_index to its 0-based rank within
// this order (design-01's "rewrite" rule): the shared client sort is then a
// no-op against normalized ranks. A fresh metadata map is
// built rather than written through the map ToAPI() returns, because that
// map may be the SAME reference as the source message's own metadata.
func renderInboxMessages(ordered []*taskmodels.Message) []*v1.Message {
	out := make([]*v1.Message, 0, len(ordered))
	for i, m := range ordered {
		api := m.ToAPI()
		meta := make(map[string]interface{}, len(api.Metadata)+1)
		for k, v := range api.Metadata {
			meta[k] = v
		}
		meta["question_index"] = i
		api.Metadata = meta
		out = append(out, api)
	}
	return out
}

// inboxBundleContext resolves the bundle's context to its FIRST message's
// metadata.context when that is a non-empty string after trimming, else "".
// This mirrors readSharedContext's
// `context?.trim() ? context : null` exactly (clarification-input-overlay.tsx),
// so the row and the panel it expands into can never disagree.
func inboxBundleContext(ordered []*taskmodels.Message) string {
	if len(ordered) == 0 {
		return ""
	}
	v, ok := ordered[0].Metadata["context"].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return ""
	}
	return v
}

// parseInboxListQuery implements the query-validation table shared by the
// main read and the hidden enumeration.
func (h *Handlers) parseInboxListQuery(c *gin.Context) (
	workspaceID string, limit int, cursorCreatedAt time.Time, cursorPendingID string, ok bool,
) {
	workspaceID = strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		respondInboxError(c, http.StatusBadRequest, "workspace_id is required")
		return "", 0, time.Time{}, "", false
	}
	var err error
	limit, err = parseInboxLimit(c.Query("limit"))
	if err != nil {
		respondInboxError(c, http.StatusBadRequest, err.Error())
		return "", 0, time.Time{}, "", false
	}
	if raw := c.Query("cursor"); raw != "" {
		cursorCreatedAt, cursorPendingID, err = decodeInboxCursor(raw)
		if err != nil {
			respondInboxError(c, http.StatusBadRequest, "cursor: "+err.Error())
			return "", 0, time.Time{}, "", false
		}
	}
	return workspaceID, limit, cursorCreatedAt, cursorPendingID, true
}

// parseInboxLimit implements the limit validation rules: absent defaults to
// 50, non-numeric or <= 0 is rejected (not clamped), and > 200 clamps to 200.
func parseInboxLimit(raw string) (int, error) {
	if raw == "" {
		return defaultInboxLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if n > maxInboxLimit {
		return maxInboxLimit, nil
	}
	return n, nil
}

// respondInboxWorkspaceAuthzError maps AuthorizeWorkspaceScope's errors per
// the read's Security rule: an unreachable workspace is 404, and 403 never
// arises on this endpoint (needs-you-inbox design, "Security").
func (h *Handlers) respondInboxWorkspaceAuthzError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		respondInboxError(c, http.StatusNotFound, "workspace not found")
	case errors.Is(err, service.ErrForbidden):
		respondInboxError(c, http.StatusForbidden, "insufficient permissions")
	default:
		h.logger.Error("failed to authorize needs-you inbox workspace access", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
	}
}

// httpUpsertInboxSidecar backs PUT /api/v1/clarification-inbox/sidecar/:pendingID.
func (h *Handlers) httpUpsertInboxSidecar(c *gin.Context) {
	pendingID := c.Param("pendingID")
	var body inboxSidecarPutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respondInboxError(c, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	var state taskmodels.ClarificationSidecarState
	switch body.State {
	case inboxStateDismissed:
		state = taskmodels.ClarificationSidecarDismissed
	case inboxStateSnoozed:
		state = taskmodels.ClarificationSidecarSnoozed
	default:
		respondInboxError(c, http.StatusBadRequest, `state must be "dismissed" or "snoozed"`)
		return
	}

	now := h.now()
	var snoozeUntil *time.Time
	if state == taskmodels.ClarificationSidecarSnoozed {
		duration := body.SnoozeDuration
		if duration == "" {
			duration = "4h"
		}
		d, ok := inboxSnoozeDurations[duration]
		if !ok {
			respondInboxError(c, http.StatusBadRequest, `snooze_duration must be one of "1h", "4h", "24h"`)
			return
		}
		until := now.Add(d)
		snoozeUntil = &until
	}

	if !h.authorizeInboxSidecarAccess(c, pendingID) {
		return
	}
	if err := h.inboxBundles.UpsertClarificationInboxSidecar(c.Request.Context(), InboxUserID(c.Request.Context()), pendingID, state, snoozeUntil, now); err != nil {
		h.logger.Error("failed to upsert needs-you inbox sidecar",
			zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, "failed to update needs-you inbox")
		return
	}
	c.Status(http.StatusNoContent)
}

// httpDeleteInboxSidecar backs DELETE /api/v1/clarification-inbox/sidecar/:pendingID:
// restore is deleting the sidecar row, idempotent by construction.
func (h *Handlers) httpDeleteInboxSidecar(c *gin.Context) {
	pendingID := c.Param("pendingID")
	if !h.authorizeInboxSidecarAccess(c, pendingID) {
		return
	}
	if err := h.inboxBundles.DeleteClarificationInboxSidecar(c.Request.Context(), InboxUserID(c.Request.Context()), pendingID); err != nil {
		h.logger.Error("failed to delete needs-you inbox sidecar",
			zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, "failed to update needs-you inbox")
		return
	}
	c.Status(http.StatusNoContent)
}

const inboxAsyncSidecarPrefix = "async:"

// authorizeInboxSidecarAccess authorizes both blocking bundle IDs and
// namespaced async question IDs without disclosing foreign records.
func (h *Handlers) authorizeInboxSidecarAccess(c *gin.Context, pendingID string) bool {
	if !strings.HasPrefix(pendingID, inboxAsyncSidecarPrefix) {
		return h.authorizeBundleAccessOrRespond(c, pendingID)
	}
	id := strings.TrimPrefix(pendingID, inboxAsyncSidecarPrefix)
	if h.inboxAsync == nil || id == "" {
		respondInboxError(c, http.StatusNotFound, errInboxInternal)
		return false
	}
	workspaceID, found, err := h.inboxAsync.AsyncQuestionWorkspace(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("failed to authorize async question sidecar",
			zap.String("question_id", id), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return false
	}
	if !found {
		respondInboxError(c, http.StatusNotFound, errInboxInternal)
		return false
	}
	if err := h.inboxTasks.AuthorizeWorkspaceScope(c.Request.Context(), workspaceID, authz.ScopeWorkspaceRead); err != nil {
		respondInboxError(c, http.StatusNotFound, errInboxInternal)
		return false
	}
	return true
}

// encodeInboxCursor/decodeInboxCursor implement the opaque (created_at,
// pending_id) cursor, mirroring the MCP list_pending_questions_kandev cursor
// codec (internal/mcp/handlers/question_handlers.go) so the two independent
// callers of the same bundle query encode pages identically.
func encodeInboxCursor(createdAt time.Time, pendingID string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + pendingID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeInboxCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, parts[1], nil
}

// formatOptionalInboxTime renders a *time.Time as an RFC3339 string pointer,
// or nil (JSON null) when absent -- next_snooze_expiry and snooze_until are
// never omitted, always either a string or null.
func earlierInboxTime(first, second *time.Time) *time.Time {
	if first == nil {
		return second
	}
	if second == nil || first.Before(*second) {
		return first
	}
	return second
}

func formatOptionalInboxTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// InboxUserID resolves the calling operator's identity for sidecar scoping.
// A real identity uses its own user id; an unscoped caller (auth disabled, or
// no identity at all) uses the pre-auth default user, keeping single-user
// behavior byte-identical -- the same convention internal/notifications uses
// for pre-auth ownership. Exported so the boot-state builder can resolve the
// same identity for the boot-hydration producer (InboxBootSummary).
func InboxUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return userstore.DefaultUserID
	}
	return identity.UserID
}

// InboxBootSummary computes the Needs-you Inbox boot-hydration producer: the
// same bounded count, truncation flag, and next snooze expiry the list
// endpoint returns, at the same default limit and workspace/sidecar scope,
// for a workspace the boot builder has already resolved as the caller's own.
func InboxBootSummary(
	ctx context.Context,
	bundles inboxBundleStore,
	workspaceID, userID string,
	now time.Time,
) (count int, hasMore bool, nextSnoozeExpiry *time.Time, err error) {
	count, err = bundles.CountVisibleClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:    true,
		WorkspaceID: workspaceID,
		Limit:       defaultInboxLimit,
		Sidecar:     &taskmodels.ClarificationSidecarFilter{UserID: userID, Now: now},
	})
	if err != nil {
		return 0, false, nil, err
	}
	summary, err := bundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1,
		Sidecar: &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now},
	})
	if err != nil {
		return 0, false, nil, err
	}
	return count, count > defaultInboxLimit, summary.NextSnoozeExpiry, nil
}
