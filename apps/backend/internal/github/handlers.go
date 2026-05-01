package github

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	errMsgInvalidPayload = "invalid payload"
	errMsgIDRequired     = "id required"
)

// RegisterRoutes registers HTTP and WebSocket routes for GitHub integration.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := NewController(svc, log)
	ctrl.RegisterHTTPRoutes(router)
	registerWSHandlers(dispatcher, svc, log)
}

// RegisterMockRoutes registers mock control endpoints if the GitHub client is a MockClient.
// This is a no-op when the underlying client is not a MockClient.
func RegisterMockRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	mock, ok := svc.Client().(*MockClient)
	if !ok {
		return
	}
	ctrl := NewMockController(mock, svc.TestStore(), svc.TestEventBus(), log)
	ctrl.RegisterRoutes(router)
	log.Info("registered GitHub mock control endpoints")
}

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	dispatcher.RegisterFunc(ws.ActionGitHubStatus, wsStatus(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubTaskPRsList, wsListTaskPRs(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubTaskPRGet, wsGetTaskPR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubPRFeedbackGet, wsGetPRFeedback(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewWatchesList, wsListReviewWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewWatchCreate, wsCreateReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewWatchUpdate, wsUpdateReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewWatchDelete, wsDeleteReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewTrigger, wsTriggerReviewWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubReviewTriggerAll, wsTriggerAllReviewChecks(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubPRWatchesList, wsListPRWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubPRWatchDelete, wsDeletePRWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubPRFilesGet, wsGetPRFiles(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubPRCommitsGet, wsGetPRCommits(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubTaskPRSync, wsSyncTaskPR(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubStats, wsGetStats(svc, log))

	// Issue watch handlers
	dispatcher.RegisterFunc(ws.ActionGitHubIssueWatchesList, wsListIssueWatches(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubIssueWatchCreate, wsCreateIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubIssueWatchUpdate, wsUpdateIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubIssueWatchDelete, wsDeleteIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubIssueTrigger, wsTriggerIssueWatch(svc, log))
	dispatcher.RegisterFunc(ws.ActionGitHubIssueTriggerAll, wsTriggerAllIssueChecks(svc, log))

	// Action preset handlers
	dispatcher.RegisterFunc(ws.ActionGitHubActionPresetsList, wsListActionPresets(svc))
	dispatcher.RegisterFunc(ws.ActionGitHubActionPresetsUpdate, wsUpdateActionPresets(svc))
	dispatcher.RegisterFunc(ws.ActionGitHubActionPresetsReset, wsResetActionPresets(svc))
}

// parseMap parses the WS message payload into a map for simple field lookups.
// Returns the map and any parse error. A nil map is replaced with an empty map.
func parseMap(msg *ws.Message) (map[string]interface{}, error) {
	var m map[string]interface{}
	err := msg.ParsePayload(&m)
	if m == nil {
		m = make(map[string]interface{})
	}
	return m, err
}

// wsWithField returns a WS handler that parses a single named string field from
// the payload and passes it to serviceFn, returning the result as the response.
func wsWithField(field string, serviceFn func(ctx context.Context, val string) (interface{}, error)) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		val, _ := payload[field].(string)
		if val == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, field+" required", nil)
		}
		result, err := serviceFn(ctx, val)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	}
}

// wsDeleteByID returns a WS handler that parses an "id" field from the payload
// and calls deleteFn. Used by both wsDeletePRWatch and wsDeleteReviewWatch.
func wsDeleteByID(deleteFn func(ctx context.Context, id string) error) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgIDRequired, nil)
		}
		if err := deleteFn(ctx, id); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	}
}

// wsUpdateByPayload builds a WS handler that parses an ID + typed request from
// the payload and delegates to updateFn.
func wsUpdateByPayload[T any](updateFn func(ctx context.Context, id string, req *T) error) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		idHolder := struct {
			ID string `json:"id"`
		}{}
		if err := msg.ParsePayload(&idHolder); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		if idHolder.ID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgIDRequired, nil)
		}
		var req T
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		if err := updateFn(ctx, idHolder.ID, &req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"updated": true})
	}
}

// wsTriggerAllByWorkspace builds a WS handler that extracts workspace_id from
// the payload, calls the given trigger function, and returns the count under the
// specified response key.
func wsTriggerAllByWorkspace(
	responseKey string,
	triggerFn func(ctx context.Context, workspaceID string) (int, error),
) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		count, err := triggerFn(ctx, workspaceID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]int{responseKey: count})
	}
}

func wsStatus(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		status, err := svc.GetStatus(ctx)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, status)
	}
}

func wsListTaskPRs(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		taskIDsStr, _ := payload["task_ids"].(string)
		if taskIDsStr == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "task_ids required", nil)
		}
		taskIDs := strings.Split(taskIDsStr, ",")
		result, err := svc.ListTaskPRs(ctx, taskIDs)
		if err != nil {
			log.Error("ws: list task PRs failed", zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	}
}

func wsGetTaskPR(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		taskID, _ := payload["task_id"].(string)
		if taskID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "task_id required", nil)
		}
		tp, err := svc.GetTaskPR(ctx, taskID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if tp == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "no PR for task", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, tp)
	}
}

// wsSyncTaskPR returns ALL PR rows for a task — multi-repo tasks have one
// per repo. Single-repo callers can read `prs[0]` (or treat empty as "no
// PR yet"); multi-repo callers iterate and call setTaskPR for each so the
// per-repo PR icon stays in sync. The legacy single-PR shape would have
// silently dropped every repo's PR except the most-recently-updated one.
func wsSyncTaskPR(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsWithField("task_id", func(ctx context.Context, taskID string) (interface{}, error) {
		prs, err := svc.TriggerPRSyncAll(ctx, taskID)
		if err != nil {
			return nil, err
		}
		// Return an envelope so the frontend always gets a deterministic
		// shape even on empty results (`{prs: []}`); a bare `nil` would
		// confuse the WS handler's success/error branching.
		return map[string]interface{}{"prs": prs}, nil
	})
}

func wsGetPRFeedback(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		owner, _ := payload["owner"].(string)
		repo, _ := payload["repo"].(string)
		numberF, _ := payload["number"].(float64)
		number := int(numberF)
		if owner == "" || repo == "" || number == 0 {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "owner, repo, number required", nil)
		}
		feedback, err := svc.GetPRFeedback(ctx, owner, repo, number)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, feedback)
	}
}

func wsListReviewWatches(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsWithField("workspace_id", func(ctx context.Context, workspaceID string) (interface{}, error) {
		watches, err := svc.ListReviewWatches(ctx, workspaceID)
		return watches, err
	})
}

func wsCreateReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateReviewWatchRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		rw, err := svc.CreateReviewWatch(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, rw)
	}
}

func wsUpdateReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsUpdateByPayload(func(ctx context.Context, id string, req *UpdateReviewWatchRequest) error {
		return svc.UpdateReviewWatch(ctx, id, req)
	})
}

func wsDeleteReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsDeleteByID(svc.DeleteReviewWatch)
}

func wsTriggerReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgIDRequired, nil)
		}
		watch, err := svc.GetReviewWatch(ctx, id)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if watch == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "review watch not found", nil)
		}
		newPRs, err := svc.CheckReviewWatch(ctx, watch)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"new_prs": len(newPRs), "prs": newPRs})
	}
}

func wsTriggerAllReviewChecks(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsTriggerAllByWorkspace("new_prs_found", svc.TriggerAllReviewChecks)
}

func wsListPRWatches(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		watches, err := svc.ListActivePRWatches(ctx)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, watches)
	}
}

func wsDeletePRWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsDeleteByID(svc.DeletePRWatch)
}

func wsGetStats(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req PRStatsRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		stats, err := svc.GetPRStats(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, stats)
	}
}

// parsePRParams extracts owner, repo, and number from a WS message payload.
// Returns a non-nil error response message if the payload is invalid or required fields are missing.
func parsePRParams(msg *ws.Message) (string, string, int, *ws.Message) {
	payload, parseErr := parseMap(msg)
	if parseErr != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		return "", "", 0, resp
	}
	owner, _ := payload["owner"].(string)
	repo, _ := payload["repo"].(string)
	numberF, _ := payload["number"].(float64)
	number := int(numberF)
	if owner == "" || repo == "" || number == 0 {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "owner, repo, number required", nil)
		return "", "", 0, resp
	}
	return owner, repo, number, nil
}

func wsGetPRFiles(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		owner, repo, number, errResp := parsePRParams(msg)
		if errResp != nil {
			return errResp, nil
		}
		files, err := svc.GetPRFiles(ctx, owner, repo, number)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"files": files})
	}
}

func wsGetPRCommits(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		owner, repo, number, errResp := parsePRParams(msg)
		if errResp != nil {
			return errResp, nil
		}
		commits, err := svc.GetPRCommits(ctx, owner, repo, number)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"commits": commits})
	}
}

// --- Issue watch WS handlers ---

func wsListIssueWatches(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsWithField("workspace_id", func(ctx context.Context, workspaceID string) (interface{}, error) {
		return svc.ListIssueWatches(ctx, workspaceID)
	})
}

func wsCreateIssueWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateIssueWatchRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		iw, err := svc.CreateIssueWatch(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, iw)
	}
}

func wsUpdateIssueWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsUpdateByPayload(func(ctx context.Context, id string, req *UpdateIssueWatchRequest) error {
		return svc.UpdateIssueWatch(ctx, id, req)
	})
}

func wsDeleteIssueWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsDeleteByID(svc.DeleteIssueWatch)
}

func wsTriggerIssueWatch(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgIDRequired, nil)
		}
		watch, err := svc.GetIssueWatch(ctx, id)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if watch == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "issue watch not found", nil)
		}
		newIssues, err := svc.CheckIssueWatch(ctx, watch)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		for _, issue := range newIssues {
			svc.publishNewIssueEvent(ctx, watch, issue)
		}
		cleaned, cleanErr := svc.CleanupClosedIssueTasks(ctx, watch)
		if cleanErr != nil {
			log.Warn("cleanup closed issue tasks failed", zap.String("watch_id", id), zap.Error(cleanErr))
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"new_issues_found": len(newIssues), "issues": newIssues, "cleaned": cleaned})
	}
}

func wsTriggerAllIssueChecks(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsTriggerAllByWorkspace("new_issues_found", svc.TriggerAllIssueChecks)
}

// --- Action preset WS handlers ---

func wsListActionPresets(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsWithField("workspace_id", func(ctx context.Context, workspaceID string) (interface{}, error) {
		return svc.GetActionPresets(ctx, workspaceID)
	})
}

func wsUpdateActionPresets(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req UpdateActionPresetsRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, errMsgInvalidPayload, nil)
		}
		if strings.TrimSpace(req.WorkspaceID) == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		presets, err := svc.UpdateActionPresets(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, presets)
	}
}

func wsResetActionPresets(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsWithField("workspace_id", func(ctx context.Context, workspaceID string) (interface{}, error) {
		return svc.ResetActionPresets(ctx, workspaceID)
	})
}
