package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/secrets"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCredentialGrantAndRevokeWebsocketActions(t *testing.T) {
	svc, a := credentialChoiceService(t)
	secretStore := &credentialChoiceSecretStore{
		secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Name: "Billing key",
			Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		},
		value: "secret-value",
	}
	svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{
		"repo-1": credentialChoiceRepository(a.WorkspaceID),
	}, secretStore, testCredentialOwnerContext, testCredentialCreatorAccess)
	makeRequest := func(action, requestID string, request *WorkspaceCredentialGrantRequest) *ws.Message {
		message, err := ws.NewRequest(requestID, action, workspaceCredentialActionRequest{
			ID: a.ID, WorkspaceCredentialGrantRequest: *request,
		})
		require.NoError(t, err)
		return message
	}

	grantRequest := workspaceGrantRequest(a, 0)
	grantResponse, err := wsGrantWorkspaceCredential(svc)(
		credentialIdentity("owner-1"), makeRequest(ws.ActionAutomationCredentialGrant, "grant", grantRequest),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, grantResponse.Type)
	var granted Automation
	require.NoError(t, grantResponse.ParsePayload(&granted))
	require.Len(t, granted.CredentialChoices, 1)
	require.Equal(t, CredentialChoiceGranted, granted.CredentialChoices[0].State)

	revokeRequest := workspaceGrantRequest(a, granted.CredentialChoices[0].Revision)
	revokeRequest.ExpectedWorkspaceOwnerRevision = testWorkspaceOwnerRevisionValue
	revokeResponse, err := wsRevokeWorkspaceCredential(svc)(
		credentialIdentity("owner-1"), makeRequest(ws.ActionAutomationCredentialRevoke, a.ID, revokeRequest),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, revokeResponse.Type)
	var revoked Automation
	require.NoError(t, revokeResponse.ParsePayload(&revoked))
	require.Equal(t, CredentialChoiceRevoked, revoked.CredentialChoices[0].State)
}

func TestWorkspaceCredentialWebsocketActionRejectsMissingIDAndMCP(t *testing.T) {
	svc, _ := credentialChoiceService(t)
	missingID, err := ws.NewRequest("missing", ws.ActionAutomationCredentialGrant, map[string]any{
		"repository_id": "repo-1", "key": "SERVICE_KEY",
	})
	require.NoError(t, err)
	response, err := wsGrantWorkspaceCredential(svc)(context.Background(), missingID)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)

	request, err := ws.NewRequest("mcp", ws.ActionAutomationCredentialGrant, workspaceCredentialActionRequest{
		ID: "automation", WorkspaceCredentialGrantRequest: WorkspaceCredentialGrantRequest{
			RepositoryID: "repo-1", Key: "SERVICE_KEY",
		},
	})
	require.NoError(t, err)
	mcpContext := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		TaskID: "task", SessionID: "session",
	})
	response, err = wsGrantWorkspaceCredential(svc)(mcpContext, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, response.ParsePayload(&payload))
	require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
}

func TestAutomationMutationErrorDoesNotExposeCredentialCause(t *testing.T) {
	message, err := ws.NewRequest("credential-error", ws.ActionAutomationUpdate, map[string]any{})
	require.NoError(t, err)
	validation := &CredentialValidationError{Field: "SERVICE_KEY", Source: "agent profile"}
	privateCause := errors.New("secret resolver rejected private-secret-value")

	response, err := automationMutationError(message, errors.Join(validation, privateCause))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, response.ParsePayload(&payload))
	require.Equal(t, ws.ErrorCodeValidation, payload.Code)
	require.Equal(t, ErrCredentialsUnavailable.Error(), payload.Message)
	require.NotContains(t, string(response.Payload), "private-secret-value")
	require.Equal(t, ErrCredentialsUnavailable.Error(), payload.Details["credential_error_code"])
	require.Equal(t, "SERVICE_KEY", payload.Details["field"])
	require.Equal(t, "agent profile", payload.Details["source"])
}
