package clarification

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

// inboxHistoryBundleView is one row of the History tab's bundle listing: the
// same shape as inboxBundleView (AC .30's message payload carries every
// field a row needs to render a clarification or permission bundle) plus
// AC .2's exclusion reason, AC .10's turn identity, AC .12's kind label, and
// AC .11's step-starts-no-agent flag.
type inboxHistoryBundleView struct {
	inboxBundleView
	// Kind distinguishes a permission bundle from a clarification bundle
	// (AC .12, AC .30), derived from the bundle's own messages rather than
	// carried as a second source of truth.
	Kind string `json:"kind"`
	// Reason is the AC .2 exclusion reason: superseded, session_ended, or
	// unreadable.
	Reason string `json:"reason"`
	// AskingTurnID is the turn that asked the bundle's question(s).
	AskingTurnID string `json:"asking_turn_id"`
	// SupersedingTurnID is set only when Reason is superseded via a
	// turn-level supersession; nil (omitted) for every other case,
	// including a same-turn permission supersession, per AC .10.
	SupersedingTurnID *string `json:"superseding_turn_id,omitempty"`
	// StepStartsNoAgent is AC .11's label: true when the task's current
	// workflow step starts no agent, false when it does, and nil (omitted)
	// when the step cannot be read -- never a guess.
	StepStartsNoAgent *bool `json:"step_starts_no_agent,omitempty"`
}

// inboxHistoryListResponse is GET /api/v1/clarification-inbox/history's
// response envelope. total is the AC .16/.20 workspace-wide bundle count,
// distinct from count, the page size.
type inboxHistoryListResponse struct {
	Bundles    []inboxHistoryBundleView `json:"bundles"`
	Count      int                      `json:"count"`
	Total      int                      `json:"total"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

// httpListInboxHistory backs GET /api/v1/clarification-inbox/history,
// modeled on httpListInboxHidden: parse and authorize identically, then read
// through the additive history path instead of the operational one.
func (h *Handlers) httpListInboxHistory(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}

	page, err := h.inboxBundles.ListInboxHistoryBundles(ctx, taskmodels.ListClarificationHistoryOptions{
		WorkspaceID:     workspaceID,
		CursorCreatedAt: cursorCreatedAt,
		CursorPendingID: cursorPendingID,
		Limit:           limit,
	})
	if err != nil {
		h.logger.Error("failed to list inbox history bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	views, err := h.buildInboxHistoryBundleViews(ctx, page.Bundles)
	if err != nil {
		h.logger.Error("failed to hydrate inbox history bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	// The workspace-wide total is its own query (AC .16/.20): a failure
	// here fails the whole read rather than defaulting the badge to zero or
	// a stale value, matching AC .25's "no count rather than a stale one".
	total, err := h.inboxBundles.CountInboxHistoryBundles(ctx, taskmodels.ListClarificationHistoryOptions{WorkspaceID: workspaceID})
	if err != nil {
		h.logger.Error("failed to count inbox history bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	resp := inboxHistoryListResponse{Bundles: views, Count: len(views), Total: total}
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	c.JSON(http.StatusOK, resp)
}

// buildInboxHistoryBundleViews hydrates each bundle's durable messages and
// task_title/session_state/step-starts-no-agent enrichment. A bundle whose
// messages cannot be read fails the whole page (matching
// buildInboxBundleViews); a bundle with zero resolvable messages is skipped
// and logged.
func (h *Handlers) buildInboxHistoryBundleViews(
	ctx context.Context, bundles []taskmodels.ClarificationHistoryBundleSummary,
) ([]inboxHistoryBundleView, error) {
	if len(bundles) == 0 {
		return []inboxHistoryBundleView{}, nil
	}
	pendingIDs, taskIDs := inboxHistoryBundleKeys(bundles)
	messagesByPendingID, err := h.inboxBundles.FindMessagesByPendingIDs(ctx, pendingIDs)
	if err != nil {
		return nil, err
	}
	taskTitles, stepStartsNoAgent := h.batchInboxHistoryTaskInfo(ctx, taskIDs)
	sessionStates := h.batchInboxSessionStates(ctx, taskIDs)

	views := make([]inboxHistoryBundleView, 0, len(bundles))
	for _, b := range bundles {
		msgs := messagesByPendingID[b.PendingID]
		if len(msgs) == 0 {
			h.logger.Warn("inbox history bundle has no resolvable messages; omitting from page",
				zap.String("pending_id", b.PendingID))
			continue
		}
		views = append(views, buildInboxHistoryBundleView(b, msgs, taskTitles, sessionStates, stepStartsNoAgent))
	}
	return views, nil
}

func inboxHistoryBundleKeys(bundles []taskmodels.ClarificationHistoryBundleSummary) (pendingIDs, taskIDs []string) {
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
	return pendingIDs, taskIDs
}

func buildInboxHistoryBundleView(
	b taskmodels.ClarificationHistoryBundleSummary,
	msgs []*taskmodels.Message,
	taskTitles, sessionStates map[string]string,
	stepStartsNoAgent map[string]*bool,
) inboxHistoryBundleView {
	ordered := orderInboxHistoryMessages(msgs)
	view := inboxHistoryBundleView{
		inboxBundleView: inboxBundleView{
			PendingID:    b.PendingID,
			TaskID:       b.TaskID,
			SessionID:    b.SessionID,
			CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339),
			Context:      inboxBundleContext(ordered),
			Messages:     renderInboxMessages(ordered),
			TaskTitle:    taskTitles[b.TaskID],
			SessionState: sessionStates[b.SessionID],
		},
		Kind:         inboxHistoryBundleKind(ordered),
		Reason:       string(b.Reason),
		AskingTurnID: b.AskingTurnID,
	}
	if b.SupersedingTurnID != "" {
		superseding := b.SupersedingTurnID
		view.SupersedingTurnID = &superseding
	}
	if flag, ok := stepStartsNoAgent[b.TaskID]; ok {
		view.StepStartsNoAgent = flag
	}
	return view
}

// inboxHistoryBundleKind derives AC .12/.30's kind label from the bundle's
// own first message rather than carrying it as a second source of truth.
func inboxHistoryBundleKind(ordered []*taskmodels.Message) string {
	if len(ordered) == 0 {
		return ""
	}
	if ordered[0].Type == taskmodels.MessageTypePermissionRequest {
		return "permission"
	}
	return "clarification"
}

// orderInboxHistoryMessages implements AC .19's three-key order:
// question_index ascending (normalized per normalizedInboxQuestionIndex),
// then question_id ascending, then message id ascending. The third key is
// NOT part of orderInboxMessages: it only breaks a remaining tie for the
// empty-question_id bundles AC .13 uniquely admits to History, which
// Needs-you never lists, so the two Inbox tabs still never order a shared
// bundle differently.
func orderInboxHistoryMessages(msgs []*taskmodels.Message) []*taskmodels.Message {
	sorted := make([]*taskmodels.Message, len(msgs))
	copy(sorted, msgs)
	sort.SliceStable(sorted, func(i, j int) bool {
		qi, qj := normalizedInboxQuestionIndex(sorted[i].Metadata), normalizedInboxQuestionIndex(sorted[j].Metadata)
		if qi != qj {
			return qi < qj
		}
		qidI, qidJ := questionIDFromMetadata(sorted[i].Metadata), questionIDFromMetadata(sorted[j].Metadata)
		if qidI != qidJ {
			return qidI < qidJ
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// batchInboxHistoryTaskInfo fetches every bundle's owning task once, and
// resolves AC .11's step-starts-no-agent flag per unique workflow step
// (cached; History pages are bounded, and a task's steps repeat across
// bundles more often than not). A read failure at either layer omits the
// affected field rather than guessing.
func (h *Handlers) batchInboxHistoryTaskInfo(ctx context.Context, taskIDs []string) (titles map[string]string, stepStartsNoAgent map[string]*bool) {
	titles = make(map[string]string, len(taskIDs))
	stepStartsNoAgent = make(map[string]*bool, len(taskIDs))
	if len(taskIDs) == 0 {
		return titles, stepStartsNoAgent
	}
	tasks, err := h.inboxTasks.GetTasksByIDs(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to batch-load inbox history task info", zap.Error(err))
		return titles, stepStartsNoAgent
	}
	stepCache := make(map[string]*bool)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		titles[task.ID] = task.Title
		if task.WorkflowStepID == "" {
			continue
		}
		flag, cached := stepCache[task.WorkflowStepID]
		if !cached {
			flag = h.resolveStepStartsNoAgent(ctx, task.WorkflowStepID)
			stepCache[task.WorkflowStepID] = flag
		}
		stepStartsNoAgent[task.ID] = flag
	}
	return titles, stepStartsNoAgent
}

// resolveStepStartsNoAgent implements AC .11: presence of an
// auto_start_agent on_enter action is the only test -- there is no
// disabled-entry concept to discount. A read failure (step not found, or
// the workflow cannot be read) omits the label rather than guessing.
func (h *Handlers) resolveStepStartsNoAgent(ctx context.Context, stepID string) *bool {
	step, err := h.inboxTasks.GetWorkflowStep(ctx, stepID)
	if err != nil || step == nil {
		return nil
	}
	startsNoAgent := !step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)
	return &startsNoAgent
}
