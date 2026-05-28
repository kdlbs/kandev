package gitlab

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	errMsgInvalidPayload = "invalid payload"
	errMsgIDRequired     = "id required"
	respKeyDeleted       = "deleted"
)

// RegisterRoutesWithDispatcher registers HTTP + WS routes for GitLab.
// Preserves the existing RegisterRoutes(router, svc, log) signature by
// adding a sibling function so callers can pass a dispatcher.
func RegisterRoutesWithDispatcher(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	NewController(svc, log).RegisterHTTPRoutes(router)
	if dispatcher != nil {
		registerWSHandlers(dispatcher, svc, log)
	}
}

// RegisterMockRoutes registers mock control endpoints if the underlying
// client is a MockClient. No-op otherwise.
func RegisterMockRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	mock, ok := svc.Client().(*MockClient)
	if !ok {
		return
	}
	NewMockController(mock, svc, log).RegisterRoutes(router)
	log.Info("registered GitLab mock control endpoints")
}

// registerWSHandlers wires the GitLab WS action surface onto the dispatcher.
func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	dispatcher.RegisterFunc(ws.ActionGitLabStatus, wsStatus(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabTaskMRsList, wsListTaskMRs(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabTaskMRGet, wsGetTaskMR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRFeedbackGet, wsGetMRFeedback(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewWatchesList, wsListReviewWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewWatchCreate, wsCreateReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewWatchUpdate, wsUpdateReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewWatchDelete, wsDeleteReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewTrigger, wsTriggerReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabReviewTriggerAll, wsTriggerAllReviewChecks(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRWatchesList, wsListMRWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRWatchDelete, wsDeleteMRWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRFilesGet, wsGetMRFiles(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRCommitsGet, wsGetMRCommits(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabTaskMRSync, wsSyncTaskMR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabStats, wsGetStats(svc, log))

	dispatcher.RegisterFunc(ws.ActionGitLabMRMerge, wsMergeMR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRApprove, wsApproveMR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRUnapprove, wsUnapproveMR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRSetLabels, wsSetMRLabels(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRSetAssignees, wsSetMRAssignees(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRDiscussionNew, wsNewDiscussionNote(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabMRDiscussionResolve, wsResolveDiscussion(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabProjectMergeMethodsGet, wsGetProjectMergeMethods(svc, log))

	dispatcher.RegisterFunc(ws.ActionGitLabIssueWatchesList, wsListIssueWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabIssueWatchCreate, wsCreateIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabIssueWatchUpdate, wsUpdateIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabIssueWatchDelete, wsDeleteIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabIssueTrigger, wsTriggerIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabIssueTriggerAll, wsTriggerAllIssueChecks(svc, log))

	dispatcher.RegisterFunc(ws.ActionGitLabActionPresetsList, wsListActionPresets(svc))
	dispatcher.RegisterFunc(ws.ActionGitLabActionPresetsUpdate, wsUpdateActionPresets(svc))
	dispatcher.RegisterFunc(ws.ActionGitLabActionPresetsReset, wsResetActionPresets(svc))

	dispatcher.RegisterFunc(ws.ActionGitLabListUserProjects, wsListUserProjects(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabSearchProjects, wsSearchProjects(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitLabProjectBranches, wsListProjectBranches(svc, log))

	dispatcher.RegisterFunc(ws.ActionGitLabCleanupReviewTasks, wsCleanupReviewTasks(svc))
	dispatcher.RegisterFunc(ws.ActionGitLabCleanupIssueTasks, wsCleanupIssueTasks(svc))
}

// --- Generic helpers ---

func parseInto[T any](msg *ws.Message, out *T) error { return msg.ParsePayload(out) }

func badRequest(msg *ws.Message, reason string) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, reason, nil)
}

func internalError(msg *ws.Message, err error) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
}

func okResponse(msg *ws.Message, payload interface{}) (*ws.Message, error) {
	return ws.NewResponse(msg.ID, msg.Action, payload)
}

// --- Status / list / get / sync ---

func wsStatus(svc *Service, log *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		status, err := svc.GetStatus(ctx)
		if err != nil {
			log.Warn("gitlab status: " + err.Error())
			return internalError(msg, err)
		}
		return okResponse(msg, status)
	}
}

func wsListTaskMRs(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			WorkspaceID string `json:"workspace_id"`
			TaskID      string `json:"task_id"`
		}
		// Malformed payloads must not silently fall through to the "list
		// everything in workspace" branch — that exposes more data than the
		// caller intended. Empty payload (both fields absent) is rejected
		// further down by the workspace_id check.
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		if req.TaskID != "" {
			mrs, err := svc.ListTaskMRsByTask(ctx, req.TaskID)
			if err != nil {
				return internalError(msg, err)
			}
			return okResponse(msg, map[string]interface{}{"task_mrs": mrs})
		}
		if req.WorkspaceID == "" {
			return badRequest(msg, "workspace_id or task_id required")
		}
		grouped, err := svc.ListTaskMRsByWorkspace(ctx, req.WorkspaceID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, TaskMRsResponse{TaskMRs: grouped})
	}
}

func wsGetTaskMR(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			TaskID string `json:"task_id"`
		}
		if err := parseInto(msg, &req); err != nil || req.TaskID == "" {
			return badRequest(msg, "task_id required")
		}
		mrs, err := svc.ListTaskMRsByTask(ctx, req.TaskID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"task_mrs": mrs})
	}
}

func wsGetMRFeedback(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		feedback, err := svc.GetMRFeedback(ctx, req.Project, req.IID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, feedback)
	}
}

func wsGetMRFiles(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		files, err := svc.GetMRFiles(ctx, req.Project, req.IID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"files": files})
	}
}

func wsGetMRCommits(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		commits, err := svc.GetMRCommits(ctx, req.Project, req.IID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"commits": commits})
	}
}

func wsSyncTaskMR(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			TaskID string `json:"task_id"`
		}
		if err := parseInto(msg, &req); err != nil || req.TaskID == "" {
			return badRequest(msg, "task_id required")
		}
		rows, err := svc.TriggerMRSync(ctx, req.TaskID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"task_mrs": rows})
	}
}

func wsGetStats(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		stats, err := svc.GetStats(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, stats)
	}
}

// --- Review watch handlers ---

func wsListReviewWatches(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		var watches []*ReviewWatch
		var err error
		if req.WorkspaceID == "" {
			watches, err = svc.ListAllReviewWatches(ctx)
		} else {
			watches, err = svc.ListReviewWatches(ctx, req.WorkspaceID)
		}
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"watches": watches})
	}
}

func wsCreateReviewWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateReviewWatchRequest
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		w, err := svc.CreateReviewWatch(ctx, &req)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, w)
	}
}

func wsUpdateReviewWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
			UpdateReviewWatchRequest
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		if err := svc.UpdateReviewWatch(ctx, req.ID, &req.UpdateReviewWatchRequest); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"updated": true})
	}
}

func wsDeleteReviewWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		if err := svc.DeleteReviewWatch(ctx, req.ID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{respKeyDeleted: true})
	}
}

func wsTriggerReviewWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		mrs, err := svc.TriggerReviewWatch(ctx, req.ID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"mrs": mrs})
	}
}

func wsTriggerAllReviewChecks(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		n, err := svc.TriggerReviewWatchAll(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"count": n})
	}
}

// --- MR watch handlers ---

func wsListMRWatches(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			SessionID string `json:"session_id"`
			TaskID    string `json:"task_id"`
		}
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		switch {
		case req.SessionID != "":
			ws, err := svc.ListMRWatchesBySession(ctx, req.SessionID)
			if err != nil {
				return internalError(msg, err)
			}
			return okResponse(msg, map[string]interface{}{"watches": ws})
		case req.TaskID != "":
			ws, err := svc.ListMRWatchesByTask(ctx, req.TaskID)
			if err != nil {
				return internalError(msg, err)
			}
			return okResponse(msg, map[string]interface{}{"watches": ws})
		default:
			ws, err := svc.ListActiveMRWatches(ctx)
			if err != nil {
				return internalError(msg, err)
			}
			return okResponse(msg, map[string]interface{}{"watches": ws})
		}
	}
}

func wsDeleteMRWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		if err := svc.DeleteMRWatch(ctx, req.ID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{respKeyDeleted: true})
	}
}

// --- Issue watch handlers ---

func wsListIssueWatches(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		var watches []*IssueWatch
		var err error
		if req.WorkspaceID == "" {
			watches, err = svc.ListAllIssueWatches(ctx)
		} else {
			watches, err = svc.ListIssueWatches(ctx, req.WorkspaceID)
		}
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"watches": watches})
	}
}

func wsCreateIssueWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateIssueWatchRequest
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		w, err := svc.CreateIssueWatch(ctx, &req)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, w)
	}
}

func wsUpdateIssueWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
			UpdateIssueWatchRequest
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		if err := svc.UpdateIssueWatch(ctx, req.ID, &req.UpdateIssueWatchRequest); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"updated": true})
	}
}

func wsDeleteIssueWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		if err := svc.DeleteIssueWatch(ctx, req.ID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{respKeyDeleted: true})
	}
}

func wsTriggerIssueWatch(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := parseInto(msg, &req); err != nil || req.ID == "" {
			return badRequest(msg, errMsgIDRequired)
		}
		issues, err := svc.TriggerIssueWatch(ctx, req.ID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"issues": issues})
	}
}

func wsTriggerAllIssueChecks(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		n, err := svc.TriggerIssueWatchAll(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"count": n})
	}
}

// --- Write actions ---

func wsMergeMR(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project             string `json:"project"`
			IID                 int    `json:"iid"`
			Method              string `json:"method"`
			SquashCommitMessage string `json:"squash_commit_message"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		mr, err := svc.MergeMR(ctx, req.Project, req.IID, req.Method, req.SquashCommitMessage)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, mr)
	}
}

func wsApproveMR(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		if err := svc.SubmitMRApproval(ctx, req.Project, req.IID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"approved": true})
	}
}

func wsUnapproveMR(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		if err := svc.SubmitMRUnapproval(ctx, req.Project, req.IID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"unapproved": true})
	}
}

func wsSetMRLabels(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string   `json:"project"`
			IID     int      `json:"iid"`
			Labels  []string `json:"labels"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		if err := svc.SetMRLabels(ctx, req.Project, req.IID, req.Labels); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"updated": true})
	}
}

func wsSetMRAssignees(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project     string `json:"project"`
			IID         int    `json:"iid"`
			AssigneeIDs []int  `json:"assignee_ids"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 {
			return badRequest(msg, "project and iid required")
		}
		if err := svc.SetMRAssignees(ctx, req.Project, req.IID, req.AssigneeIDs); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"updated": true})
	}
}

func wsNewDiscussionNote(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project      string `json:"project"`
			IID          int    `json:"iid"`
			DiscussionID string `json:"discussion_id"`
			Body         string `json:"body"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 || req.DiscussionID == "" {
			return badRequest(msg, "project, iid, discussion_id required")
		}
		note, err := svc.CreateMRDiscussionNote(ctx, req.Project, req.IID, req.DiscussionID, req.Body)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, note)
	}
}

func wsResolveDiscussion(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project      string `json:"project"`
			IID          int    `json:"iid"`
			DiscussionID string `json:"discussion_id"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" || req.IID <= 0 || req.DiscussionID == "" {
			return badRequest(msg, "project, iid, discussion_id required")
		}
		if err := svc.ResolveMRDiscussion(ctx, req.Project, req.IID, req.DiscussionID); err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"resolved": true})
	}
}

func wsGetProjectMergeMethods(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" {
			return badRequest(msg, "project required")
		}
		methods, err := svc.GetProjectMergeMethods(ctx, req.Project)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, methods)
	}
}

// --- Projects ---

func wsListUserProjects(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		projects, err := svc.ListUserProjects(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"projects": projects})
	}
}

func wsSearchProjects(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := parseInto(msg, &req); err != nil {
			return badRequest(msg, errMsgInvalidPayload)
		}
		projects, err := svc.SearchProjects(ctx, req.Query, req.Limit)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"projects": projects})
	}
}

func wsListProjectBranches(svc *Service, _ *logger.Logger) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			Project string `json:"project"`
		}
		if err := parseInto(msg, &req); err != nil || req.Project == "" {
			return badRequest(msg, "project required")
		}
		branches, err := svc.ListProjectBranches(ctx, req.Project)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]interface{}{"branches": branches})
	}
}

// --- Action presets ---

func wsListActionPresets(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if err := parseInto(msg, &req); err != nil || req.WorkspaceID == "" {
			return badRequest(msg, "workspace_id required")
		}
		presets, err := svc.GetActionPresetsOrDefault(ctx, req.WorkspaceID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, presets)
	}
}

func wsUpdateActionPresets(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req UpdateActionPresetsRequest
		if err := parseInto(msg, &req); err != nil || req.WorkspaceID == "" {
			return badRequest(msg, "workspace_id required")
		}
		presets, err := svc.UpdateActionPresets(ctx, &req)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, presets)
	}
}

func wsResetActionPresets(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if err := parseInto(msg, &req); err != nil || req.WorkspaceID == "" {
			return badRequest(msg, "workspace_id required")
		}
		presets, err := svc.ResetActionPresets(ctx, req.WorkspaceID)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, presets)
	}
}

// --- Cleanup ---

func wsCleanupReviewTasks(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		n, err := svc.CleanupAllReviewTasks(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]int{respKeyDeleted: n})
	}
}

func wsCleanupIssueTasks(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		n, err := svc.CleanupAllIssueTasks(ctx)
		if err != nil {
			return internalError(msg, err)
		}
		return okResponse(msg, map[string]int{respKeyDeleted: n})
	}
}
