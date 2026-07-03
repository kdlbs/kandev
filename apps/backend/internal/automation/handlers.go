package automation

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes registers HTTP and WebSocket routes for automations.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	registerWSHandlers(dispatcher, svc, log)
	registerHTTPRoutes(router, svc, log)
}

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	dispatcher.RegisterFunc(ws.ActionAutomationList, wsList(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationGet, wsGet(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationCreate, wsCreate(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationUpdate, wsUpdate(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationDelete, wsDelete(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationEnable, wsEnable(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationDisable, wsDisable(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationTrigger, wsManualTrigger(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationRunsList, wsListRuns(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationTriggerAdd, wsAddTrigger(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationTriggerUpdate, wsUpdateTrigger(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationTriggerDelete, wsDeleteTrigger(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationTriggerTypes, wsTriggerTypes())
	dispatcher.RegisterFunc(ws.ActionAutomationWebhookRevealSecret, wsRevealWebhookSecret(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationRunDelete, wsDeleteRun(svc, log))
	dispatcher.RegisterFunc(ws.ActionAutomationRunsDeleteAll, wsDeleteAllRuns(svc, log))
}

func registerHTTPRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	wh := NewWebhookHandler(svc, log)
	router.POST("/api/v1/automations/webhook/:id", wh.Handle)
}

// parseMap parses the WS message payload into a map.
func parseMap(msg *ws.Message) (map[string]interface{}, error) {
	var m map[string]interface{}
	err := msg.ParsePayload(&m)
	if m == nil {
		m = make(map[string]interface{})
	}
	return m, err
}

func wsList(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		items, err := svc.ListAutomations(ctx, workspaceID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, items)
	}
}

func wsGet(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		a, err := svc.GetAutomation(ctx, id)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if a == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "automation not found", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, a)
	}
}

func wsCreate(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req CreateAutomationRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
		}
		a, err := svc.CreateAutomation(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		// Service.CreateAutomation ends with store.GetAutomation which returns
		// (nil, nil) if the row vanished between insert and select — guard here
		// so we don't dereference a nil pointer building the response.
		if a == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load created automation", nil)
		}
		// One-time reveal of the webhook secret. Service.CreateAutomation
		// re-reads the row before returning, so a.WebhookSecret is already
		// populated — no second DB round-trip needed (and avoiding one keeps
		// us from silently shipping an empty secret on a transient failure).
		// The Automation struct hides it via `json:"-"` so list/get stay safe;
		// the response DTO surfaces the plaintext value for the client to
		// display once.
		return ws.NewResponse(msg.ID, msg.Action, &CreateAutomationResponse{Automation: a, WebhookSecret: a.WebhookSecret})
	}
}

func wsUpdate(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		var req UpdateAutomationRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
		}
		a, err := svc.UpdateAutomation(ctx, id, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, a)
	}
}

func wsDelete(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		if err := svc.DeleteAutomation(ctx, id); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	}
}

func wsEnable(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsToggleEnabled(svc, true)
}

func wsDisable(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsToggleEnabled(svc, false)
}

func wsToggleEnabled(svc *Service, enable bool) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		var err error
		if enable {
			err = svc.EnableAutomation(ctx, id)
		} else {
			err = svc.DisableAutomation(ctx, id)
		}
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		a, _ := svc.GetAutomation(ctx, id)
		return ws.NewResponse(msg.ID, msg.Action, a)
	}
}

func wsManualTrigger(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		a, err := svc.GetAutomation(ctx, id)
		if err != nil || a == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "automation not found", nil)
		}
		data, _ := json.Marshal(map[string]string{triggerDataSourceKey: triggerDataSourceManual})
		triggerID := ""
		if len(a.Triggers) > 0 {
			triggerID = a.Triggers[0].ID
		}
		if fireErr := svc.FireTrigger(ctx, id, triggerID, "manual", data, ""); fireErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fireErr.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"triggered": true})
	}
}

func wsListRuns(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		automationID, _ := payload["automation_id"].(string)
		if automationID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "automation_id required", nil)
		}
		limit := 50
		if l, ok := payload["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		runs, err := svc.ListRuns(ctx, automationID, limit)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, runs)
	}
}

func wsAddTrigger(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req AddTriggerRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
		}
		t, err := svc.AddTrigger(ctx, &req)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, t)
	}
}

func wsUpdateTrigger(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		var req UpdateTriggerRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
		}
		if err := svc.UpdateTrigger(ctx, id, &req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"updated": true})
	}
}

func wsDeleteTrigger(svc *Service, log *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		if err := svc.DeleteTrigger(ctx, id); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	}
}

func wsTriggerTypes() func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		return ws.NewResponse(msg.ID, msg.Action, GetTriggerTypes())
	}
}

func wsRevealWebhookSecret(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		id, _ := payload["id"].(string)
		if id == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id required", nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		// GetAutomation returns nil (not an error) when the row is missing —
		// map that to a NotFound response so the client can surface it cleanly.
		// We also return NotFound (not Forbidden) when the automation belongs
		// to a different workspace — this avoids disclosing whether the id
		// exists at all across workspace boundaries.
		a, err := svc.GetAutomation(ctx, id)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if a == nil || a.WorkspaceID != workspaceID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "automation not found", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, &RevealWebhookSecretResponse{WebhookSecret: a.WebhookSecret})
	}
}

func wsDeleteRun(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		runID, _ := payload["run_id"].(string)
		if runID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "run_id required", nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		// The WS layer is not itself workspace-scoped, so we resolve the run's
		// automation and reject the request when it belongs to a different
		// workspace — otherwise any authenticated client who knows a foreign
		// run_id could permanently delete it. NotFound (not Forbidden) avoids
		// disclosing whether the id exists at all, mirroring wsRevealWebhookSecret.
		run, err := svc.GetRun(ctx, runID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if run == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "run not found", nil)
		}
		a, err := svc.GetAutomation(ctx, run.AutomationID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if a == nil || a.WorkspaceID != workspaceID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "run not found", nil)
		}
		if err := svc.DeleteRun(ctx, runID); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	}
}

func wsDeleteAllRuns(svc *Service, _ *logger.Logger) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		payload, _ := parseMap(msg)
		automationID, _ := payload["automation_id"].(string)
		if automationID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "automation_id required", nil)
		}
		workspaceID, _ := payload["workspace_id"].(string)
		if workspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		// See wsDeleteRun: the WS layer isn't workspace-scoped, so this
		// destructive bulk delete must verify ownership itself.
		a, err := svc.GetAutomation(ctx, automationID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		if a == nil || a.WorkspaceID != workspaceID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "automation not found", nil)
		}
		if err := svc.DeleteAllRuns(ctx, automationID); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	}
}
