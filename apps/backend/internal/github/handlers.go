package github

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes registers HTTP and WebSocket routes for GitHub integration.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := NewController(svc, log)
	ctrl.RegisterHTTPRoutes(router)
	registerWSHandlers(dispatcher, svc, log)
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
	dispatcher.RegisterFunc(ws.ActionGitHubStats, wsGetStats(svc, log))
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
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		if err := deleteFn(ctx, id); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
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
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		watches, err := svc.ListReviewWatches(ctx, workspaceID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, watches)
	}
}

func wsCreateReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateReviewWatchRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		rw, err := svc.CreateReviewWatch(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, rw)
	}
}

func wsUpdateReviewWatch(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var payload struct {
			ID string `json:"id"`
			UpdateReviewWatchRequest
		}
		if err := msg.ParsePayload(&payload); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if payload.ID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		req := payload.UpdateReviewWatchRequest
		if err := svc.UpdateReviewWatch(ctx, payload.ID, &req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"updated": true})
	}
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
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
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
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, parseErr := parseMap(msg)
		if parseErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+parseErr.Error(), nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		count, err := svc.TriggerAllReviewChecks(ctx, workspaceID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]int{"new_prs_found": count})
	}
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
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
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
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
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
