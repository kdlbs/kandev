package github

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultGitHubUserID = DefaultUserID

const githubNotConfiguredCode = "github_not_configured"

func currentGitHubUserID(_ *gin.Context) string {
	// Replaceable once multi-user request identity is available at the HTTP
	// boundary. Keeping this centralized prevents handlers from inventing IDs.
	return defaultGitHubUserID
}

func (c *Controller) httpGetWorkspaceConnection(ctx *gin.Context) {
	status, err := c.service.GetWorkspaceAuthStatus(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpListCLIAccounts(ctx *gin.Context) {
	accounts, err := c.service.ListGHAccounts(ctx.Request.Context())
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	workspaceID := strings.TrimSpace(ctx.Query("workspace_id"))
	if workspaceID != "" {
		status, statusErr := c.service.GetWorkspaceAuthStatus(
			ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx),
		)
		if statusErr != nil {
			writeGitHubAuthError(ctx, statusErr)
			return
		}
		if status.Automation != nil && status.Automation.Source == ConnectionSourceGHCLI {
			for index := range accounts {
				accounts[index].Selected = strings.EqualFold(accounts[index].Host, status.Automation.GitHubHost) &&
					strings.EqualFold(accounts[index].Login, status.Automation.Login)
			}
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"accounts": accounts})
}

func (c *Controller) httpSetWorkspaceConnection(ctx *gin.Context) {
	var request SetWorkspaceConnectionRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "github_invalid_request", "error": "invalid payload"})
		return
	}
	connection, err := c.service.SetWorkspaceConnection(
		ctx.Request.Context(), workspaceIDFromRouteOrQuery(ctx), request,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, connection)
}

func (c *Controller) httpDeleteWorkspaceConnection(ctx *gin.Context) {
	if err := c.service.DeleteWorkspaceConnection(
		ctx.Request.Context(), workspaceIDFromRouteOrQuery(ctx),
	); err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"disconnected": true})
}

func (c *Controller) httpStartAppInstallation(ctx *gin.Context) {
	var request struct {
		WorkspaceID       string `json:"workspace_id"`
		AppRegistrationID string `json:"app_registration_id"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil ||
		strings.TrimSpace(request.WorkspaceID) == "" ||
		strings.TrimSpace(request.AppRegistrationID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": "github_app_invalid_request", "error": "workspace and App registration are required",
		})
		return
	}
	result, err := c.service.StartAppInstallation(
		ctx.Request.Context(), strings.TrimSpace(request.WorkspaceID), currentGitHubUserID(ctx),
		strings.TrimSpace(request.AppRegistrationID),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpCompleteAppInstallation(ctx *gin.Context) {
	if !validGitHubCallbackState(ctx.Query("state")) {
		redirectGitHubCallback(ctx, "", "github_invalid_callback")
		return
	}
	workspaceID := c.authFlowWorkspace(ctx, ctx.Query("state"))
	installationID, err := strconv.ParseInt(ctx.Query("installation_id"), 10, 64)
	if err != nil {
		redirectGitHubCallback(ctx, workspaceID, "github_invalid_callback")
		return
	}
	result, err := c.service.CompleteAppInstallation(
		ctx.Request.Context(), ctx.Param("registrationId"), AppInstallationCallback{
			State:          ctx.Query("state"),
			Code:           ctx.Query("code"),
			SetupAction:    ctx.Query("setup_action"),
			InstallationID: installationID,
		})
	if err != nil {
		redirectGitHubCallback(ctx, workspaceID, githubAuthErrorCode(err))
		return
	}
	redirectGitHubCallback(ctx, result.WorkspaceID, "app_connected")
}

func (c *Controller) httpStartPersonalAuth(ctx *gin.Context) {
	workspaceID, ok := setupWorkspaceID(ctx)
	if !ok {
		return
	}
	result, err := c.service.StartPersonalAuth(
		ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpCompletePersonalAuth(ctx *gin.Context) {
	if !validGitHubCallbackState(ctx.Query("state")) {
		redirectGitHubCallback(ctx, "", "github_invalid_callback")
		return
	}
	workspaceID := c.authFlowWorkspace(ctx, ctx.Query("state"))
	result, err := c.service.CompletePersonalAuth(
		ctx.Request.Context(), ctx.Param("registrationId"), PersonalAuthCallback{
			State: ctx.Query("state"),
			Code:  ctx.Query("code"),
		})
	if err != nil {
		redirectGitHubCallback(ctx, workspaceID, githubAuthErrorCode(err))
		return
	}
	redirectGitHubCallback(ctx, result.WorkspaceID, "personal_connected")
}

func (c *Controller) authFlowWorkspace(ctx *gin.Context, state string) string {
	if c == nil || c.service == nil || c.service.store == nil {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(state)))
	flow, err := c.service.store.GetAuthFlow(ctx.Request.Context(), stateDigestString(digest))
	if err != nil || flow == nil {
		return ""
	}
	return flow.WorkspaceID
}

func validGitHubCallbackState(state string) bool {
	trimmed := strings.TrimSpace(state)
	if len(trimmed) != base64.RawURLEncoding.EncodedLen(oauthRandomBytes) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	return err == nil && len(decoded) == oauthRandomBytes
}

func (c *Controller) httpDisconnectPersonalAuth(ctx *gin.Context) {
	if err := c.service.DisconnectPersonalAuth(
		ctx.Request.Context(), workspaceIDFromRouteOrQuery(ctx), currentGitHubUserID(ctx),
	); err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"disconnected": true})
}

func (c *Controller) httpGitHubAppWebhook(ctx *gin.Context) {
	payload, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxGitHubWebhookPayloadSize+1))
	if err != nil || len(payload) > maxGitHubWebhookPayloadSize {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "github_invalid_webhook", "error": "invalid webhook payload"})
		return
	}
	result, err := c.service.HandleAppRegistrationWebhook(
		ctx.Request.Context(), ctx.Param("registrationId"), GitHubWebhookRequest{
			DeliveryID: ctx.GetHeader("X-GitHub-Delivery"),
			Event:      ctx.GetHeader("X-GitHub-Event"),
			Signature:  ctx.GetHeader("X-Hub-Signature-256"),
			Payload:    payload,
		})
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpResolveCredentialLease(ctx *gin.Context) {
	var request struct {
		Lease        string `json:"lease"`
		TaskID       string `json:"task_id"`
		SessionID    string `json:"session_id"`
		RepositoryID string `json:"repository_id"`
		Owner        string `json:"owner"`
		Repo         string `json:"repo"`
		Host         string `json:"host"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "github_invalid_request", "error": "invalid payload"})
		return
	}
	credential, err := c.service.ResolveGitHubCredential(ctx.Request.Context(), BrokerCredentialRequest{
		Lease: request.Lease, TaskID: request.TaskID, SessionID: request.SessionID,
		RepositoryID: request.RepositoryID, Owner: request.Owner, Repo: request.Repo, Host: request.Host,
	})
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, credential)
}

func (c *Controller) httpCredentialBrokerReady(ctx *gin.Context) {
	if c == nil || c.service == nil || !c.service.CredentialBrokerReady() {
		ctx.Status(http.StatusServiceUnavailable)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(http.StatusNoContent)
}

func writeGitHubAuthError(ctx *gin.Context, err error) {
	status, code := githubAuthErrorResponse(err)
	message := strings.TrimSpace(err.Error())
	payload := gin.H{"code": code, "error": message}
	var importError *AppRegistrationImportError
	if errors.As(err, &importError) {
		if importError.ExistingRegistrationID != "" {
			payload["existing_registration_id"] = importError.ExistingRegistrationID
		}
		if len(importError.Problems) > 0 {
			payload["problems"] = importError.Problems
		}
	}
	ctx.JSON(status, payload)
}

func githubAuthErrorCode(err error) string {
	_, code := githubAuthErrorResponse(err)
	return code
}

func githubAuthErrorResponse(err error) (int, string) {
	if status, code, ok := deploymentAppHTTPError(err); ok {
		return status, code
	}
	status, code := http.StatusInternalServerError, "github_internal_error"
	switch {
	case errors.Is(err, ErrGitHubWorkspaceRequired):
		status, code = http.StatusBadRequest, "github_workspace_required"
	case errors.Is(err, ErrGitHubNotConfigured):
		status, code = http.StatusConflict, githubNotConfiguredCode
	case errors.Is(err, ErrWorkspaceConnectionStale):
		status, code = http.StatusConflict, "github_connection_changed"
	case errors.Is(err, ErrGitHubPersonalRequired):
		status, code = http.StatusConflict, "github_personal_required"
	case errors.Is(err, ErrGitHubCapabilityDenied):
		status, code = http.StatusForbidden, "github_capability_denied"
	case errors.Is(err, ErrGitHubConnectionInvalid), errors.Is(err, ErrPersonalTokenInvalid):
		status, code = http.StatusUnauthorized, "github_reconnect_required"
	case errors.Is(err, ErrOAuthStateInvalid), errors.Is(err, ErrOAuthStateMismatch),
		errors.Is(err, ErrInstallationAssociationUnverified):
		status, code = http.StatusBadRequest, "github_invalid_callback"
	case errors.Is(err, ErrInvalidWebhookSignature):
		status, code = http.StatusUnauthorized, "github_invalid_webhook_signature"
	case errors.Is(err, ErrCredentialLeaseInvalid), errors.Is(err, ErrCredentialLeaseExpired),
		errors.Is(err, ErrCredentialLeaseRevoked), errors.Is(err, ErrCredentialScopeDenied):
		status, code = http.StatusUnauthorized, "github_credential_denied"
	case errors.Is(err, ErrInvalidToken):
		status, code = http.StatusBadRequest, "github_invalid_token"
	}
	return status, code
}

func isGitHubNotConfiguredError(err error) bool {
	return errors.Is(err, ErrNoClient) || errors.Is(err, ErrGitHubNotConfigured)
}

func writeGitHubOperationalAuthError(ctx *gin.Context, err error) bool {
	if isGitHubNotConfiguredError(err) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"code":  githubNotConfiguredCode,
			"error": "GitHub is not configured. Connect GitHub in workspace settings.",
		})
		return true
	}
	for _, target := range []error{
		ErrGitHubWorkspaceRequired,
		ErrGitHubPersonalRequired,
		ErrGitHubConnectionInvalid,
		ErrPersonalTokenInvalid,
		ErrGitHubCapabilityDenied,
	} {
		if errors.Is(err, target) {
			writeGitHubAuthError(ctx, err)
			return true
		}
	}
	return false
}

func workspaceIDFromRouteOrQuery(ctx *gin.Context) string {
	if workspaceID := strings.TrimSpace(ctx.Param("workspaceId")); workspaceID != "" {
		return workspaceID
	}
	return strings.TrimSpace(ctx.Query("workspace_id"))
}

func setupWorkspaceID(ctx *gin.Context) (string, bool) {
	if workspaceID := workspaceIDFromRouteOrQuery(ctx); workspaceID != "" {
		return workspaceID, true
	}
	var request struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": "github_workspace_required", "error": ErrGitHubWorkspaceRequired.Error(),
		})
		return "", false
	}
	return strings.TrimSpace(request.WorkspaceID), true
}

func redirectGitHubCallback(ctx *gin.Context, workspaceID, result string) {
	query := url.Values{"github_result": []string{result}}
	if workspaceID != "" {
		query.Set("workspace_id", workspaceID)
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Referrer-Policy", "no-referrer")
	ctx.Redirect(http.StatusSeeOther, "/settings/integrations/github?"+query.Encode())
}
