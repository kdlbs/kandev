package automation

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type workspaceCredentialActionRequest struct {
	ID string `json:"id"`
	WorkspaceCredentialGrantRequest
}

func wsGrantWorkspaceCredential(svc *Service) func(context.Context, *ws.Message) (*ws.Message, error) {
	return workspaceCredentialAction(svc, true)
}

func wsRevokeWorkspaceCredential(svc *Service) func(context.Context, *ws.Message) (*ws.Message, error) {
	return workspaceCredentialAction(svc, false)
}

func workspaceCredentialAction(svc *Service, grant bool) func(context.Context, *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		if isMCPAutomationContext(ctx) {
			return automationMCPMutationDenied(msg)
		}
		var req workspaceCredentialActionRequest
		if err := msg.ParsePayload(&req); err != nil || req.ID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid workspace credential request", nil)
		}
		var (
			a   *Automation
			err error
		)
		if grant {
			a, err = svc.GrantWorkspaceSecret(ctx, req.ID, &req.WorkspaceCredentialGrantRequest)
		} else {
			a, err = svc.RevokeWorkspaceSecret(ctx, req.ID, &req.WorkspaceCredentialGrantRequest)
		}
		if err != nil {
			return automationMutationError(msg, err)
		}
		return ws.NewResponse(msg.ID, msg.Action, a)
	}
}
