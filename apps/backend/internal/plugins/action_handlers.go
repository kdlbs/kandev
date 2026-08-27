package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// --- Authenticated action relay ---

const (
	actionErrorField              = "error"
	authenticationRequiredMessage = "authentication required"
)

type actionHTTPEnvelope struct {
	WorkspaceID  string          `json:"workspaceId"`
	TaskID       string          `json:"taskId"`
	SessionID    string          `json:"sessionId"`
	RepositoryID string          `json:"repositoryId"`
	Body         json.RawMessage `json:"body"`
}

// action serves POST /api/plugins/:id/actions/:key. Unlike webhooks, this
// route stays behind the normal authentication middleware: it validates a
// manifest-declared action, independently authorizes its declared resource
// scope, and supplies only that verified context to the plugin subprocess.
func (c *Controller) action(ctx *gin.Context) {
	record, ok := c.activeRecord(ctx)
	if !ok {
		return
	}

	declared, ok := manifestAction(record, ctx.Param("key"))
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "plugin action not found"})
		return
	}
	if !authorizeActionAccess(ctx, declared) {
		return
	}

	envelope, ok := readActionEnvelope(ctx, declared.MaxBodyBytes)
	if !ok {
		return
	}
	verified, ok := c.verifyActionContext(ctx, declared.ResourceScope, envelope)
	if !ok {
		return
	}
	if c.actionInvoker == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "plugin action unavailable"})
		return
	}

	invokeCtx, cancel := context.WithTimeout(ctx.Request.Context(), pluginActionTimeout)
	defer cancel()
	resp, err := c.actionInvoker.InvokeAction(invokeCtx, record.ID, dispatchGeneration(record), &pluginsdk.PluginActionRequest{
		ActionKey: ctx.Param("key"),
		Context:   verified,
		Body:      envelope.Body,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(invokeCtx.Err(), context.DeadlineExceeded) {
			ctx.JSON(http.StatusGatewayTimeout, gin.H{actionErrorField: "plugin action timed out"})
			return
		}
		if errors.Is(ctx.Request.Context().Err(), context.Canceled) {
			return
		}
		c.log.Warn("plugin action invocation failed", zap.String("plugin", record.ID), zap.String("action", declared.Key), zap.Error(err))
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "plugin action unavailable"})
		return
	}
	if resp == nil {
		ctx.JSON(http.StatusBadGateway, gin.H{actionErrorField: "plugin returned an empty action response"})
		return
	}
	if len(resp.Body) > maxPluginActionResponseBytes {
		ctx.JSON(http.StatusBadGateway, gin.H{actionErrorField: "plugin action response exceeds maximum size"})
		return
	}
	c.writeActionResponse(ctx, resp)
}

func authorizeActionAccess(ctx *gin.Context, action manifest.Action) bool {
	identity, authenticated := authn.FromGin(ctx)
	if !authenticated || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{actionErrorField: authenticationRequiredMessage})
		return false
	}
	switch action.EffectiveAccess() {
	case manifest.ActionAccessAuthenticated:
		return true
	case manifest.ActionAccessAdmin:
		if identity.IsAdmin() {
			return true
		}
		ctx.JSON(http.StatusForbidden, gin.H{actionErrorField: "admin role required"})
		return false
	default:
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "plugin action has invalid access"})
		return false
	}
}

func manifestAction(record *store.Record, key string) (manifest.Action, bool) {
	for _, action := range record.Actions {
		if action.Key == key {
			return action, true
		}
	}
	return manifest.Action{}, false
}

// readActionEnvelope applies the global hard request cap, decodes one JSON
// envelope, and then applies the smaller manifest-declared cap to its raw
// body. Resource selectors never get forwarded inside the untrusted body.
func readActionEnvelope(ctx *gin.Context, maxBodyBytes int) (actionHTTPEnvelope, bool) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxPluginActionEnvelopeBytes)
	decoder := json.NewDecoder(ctx.Request.Body)
	var envelope actionHTTPEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{actionErrorField: "plugin action request exceeds maximum size"})
		} else {
			ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "invalid plugin action payload"})
		}
		return actionHTTPEnvelope{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "invalid plugin action payload"})
		return actionHTTPEnvelope{}, false
	}
	if len(envelope.Body) > maxBodyBytes {
		ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{actionErrorField: "plugin action body exceeds declared maximum size"})
		return actionHTTPEnvelope{}, false
	}
	return envelope, true
}

// verifyActionContext checks the authenticated actor and resolves exactly the
// selector type declared by the action. The task relationship is host-derived
// and every task/repository lookup runs through the already-authenticated task
// data source, so a caller cannot forge a resource context in action JSON.
func (c *Controller) verifyActionContext(
	ctx *gin.Context, resourceScope string, envelope actionHTTPEnvelope,
) (pluginsdk.VerifiedActionContext, bool) {
	identity, authenticated := authn.FromGin(ctx)
	if !authenticated || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{actionErrorField: authenticationRequiredMessage})
		return pluginsdk.VerifiedActionContext{}, false
	}
	if c.svc.taskData == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "plugin action authorization unavailable"})
		return pluginsdk.VerifiedActionContext{}, false
	}

	verified := pluginsdk.VerifiedActionContext{ActorID: identity.UserID}
	switch resourceScope {
	case manifest.ActionScopeWorkspace:
		return c.verifyWorkspaceAction(ctx, verified, envelope)
	case manifest.ActionScopeTask:
		return c.verifyTaskAction(ctx, verified, envelope)
	case manifest.ActionScopeRepository:
		return c.verifyRepositoryAction(ctx, verified, envelope)
	default:
		// Manifest validation makes this unreachable for persisted plugins, but
		// retain a fail-closed guard for records created by older host versions.
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "plugin action has invalid resource scope"})
		return pluginsdk.VerifiedActionContext{}, false
	}
}

func (c *Controller) verifyWorkspaceAction(
	ctx *gin.Context, verified pluginsdk.VerifiedActionContext, envelope actionHTTPEnvelope,
) (pluginsdk.VerifiedActionContext, bool) {
	if envelope.WorkspaceID == "" || envelope.TaskID != "" || envelope.SessionID != "" || envelope.RepositoryID != "" {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "workspace action requires only workspaceId"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	workspaces, err := c.svc.taskData.ListWorkspaces(ctx.Request.Context())
	if err != nil || !containsWorkspace(workspaces, envelope.WorkspaceID) {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "workspace not found"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	verified.WorkspaceID = envelope.WorkspaceID
	return verified, true
}

func (c *Controller) verifyTaskAction(
	ctx *gin.Context, verified pluginsdk.VerifiedActionContext, envelope actionHTTPEnvelope,
) (pluginsdk.VerifiedActionContext, bool) {
	if envelope.TaskID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "task action requires taskId"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	task, err := c.svc.taskData.GetTask(ctx.Request.Context(), envelope.TaskID)
	if err != nil || task == nil {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "task not found"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	if envelope.WorkspaceID != "" && envelope.WorkspaceID != task.WorkspaceID {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "workspaceId does not match task"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	verified.WorkspaceID = task.WorkspaceID
	verified.TaskID = task.ID
	if envelope.RepositoryID != "" {
		if !taskContainsRepository(task, envelope.RepositoryID) {
			ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "repository not found"})
			return pluginsdk.VerifiedActionContext{}, false
		}
		verified.RepositoryID = envelope.RepositoryID
	}
	if envelope.SessionID == "" {
		return verified, true
	}
	sessionID, headBranch, ok := c.verifyTaskSessionAction(
		ctx, task, envelope.SessionID, verified.RepositoryID,
	)
	if !ok {
		return pluginsdk.VerifiedActionContext{}, false
	}
	verified.SessionID = sessionID
	verified.HeadBranch = headBranch
	return verified, true
}

func (c *Controller) verifyTaskSessionAction(
	ctx *gin.Context, task *taskmodels.Task, sessionID, repositoryID string,
) (string, string, bool) {
	sessions, err := c.svc.taskData.ListTaskSessions(ctx.Request.Context(), task.ID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "session not found"})
		return "", "", false
	}
	session := findTaskSession(sessions, task.ID, sessionID)
	if session == nil {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "session not found"})
		return "", "", false
	}
	if repositoryID == "" {
		return session.ID, "", true
	}
	headBranch := sessionRepositoryBranch(session, repositoryID)
	if headBranch == "" {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "session repository checkout not found"})
		return "", "", false
	}
	return session.ID, headBranch, true
}

func findTaskSession(sessions []*taskmodels.TaskSession, taskID, sessionID string) *taskmodels.TaskSession {
	for _, session := range sessions {
		if session != nil && session.ID == sessionID && session.TaskID == taskID {
			return session
		}
	}
	return nil
}

func sessionRepositoryBranch(session *taskmodels.TaskSession, repositoryID string) string {
	for _, worktree := range session.Worktrees {
		if worktree != nil && worktree.RepositoryID == repositoryID {
			return strings.TrimSpace(worktree.WorktreeBranch)
		}
	}
	return ""
}

func taskContainsRepository(task *taskmodels.Task, repositoryID string) bool {
	for _, repository := range task.Repositories {
		if repository != nil && repository.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

func (c *Controller) verifyRepositoryAction(
	ctx *gin.Context, verified pluginsdk.VerifiedActionContext, envelope actionHTTPEnvelope,
) (pluginsdk.VerifiedActionContext, bool) {
	if envelope.WorkspaceID == "" || envelope.RepositoryID == "" || envelope.TaskID != "" || envelope.SessionID != "" {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "repository action requires workspaceId and repositoryId"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	repositories, err := c.svc.taskData.ListRepositories(ctx.Request.Context(), envelope.WorkspaceID)
	if err != nil || !containsRepository(repositories, envelope.RepositoryID) {
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "repository not found"})
		return pluginsdk.VerifiedActionContext{}, false
	}
	verified.WorkspaceID = envelope.WorkspaceID
	verified.RepositoryID = envelope.RepositoryID
	return verified, true
}

func containsWorkspace(workspaces []*taskmodels.Workspace, workspaceID string) bool {
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID == workspaceID {
			return true
		}
	}
	return false
}

func containsRepository(repositories []*taskmodels.Repository, repositoryID string) bool {
	for _, repository := range repositories {
		if repository != nil && repository.ID == repositoryID {
			return true
		}
	}
	return false
}

var allowedActionResponseHeaders = map[string]struct{}{
	"Cache-Control":                        {},
	contentTypeHeader:                      {},
	http.CanonicalHeaderKey("ETag"):        {},
	http.CanonicalHeaderKey("Retry-After"): {},
}

// writeActionResponse exposes only the small, browser-safe header surface an
// authenticated action needs. It deliberately excludes redirects, cookies,
// content length/encoding, and arbitrary X-* headers from plugin control.
func (c *Controller) writeActionResponse(ctx *gin.Context, response *pluginsdk.PluginActionResponse) {
	responseStatus := response.Status
	if responseStatus == 0 {
		responseStatus = http.StatusOK
	}
	if responseStatus < 200 || responseStatus > 599 {
		ctx.JSON(http.StatusBadGateway, gin.H{actionErrorField: "plugin returned an invalid action status"})
		return
	}
	for key, value := range response.Headers {
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, allowed := allowedActionResponseHeaders[canonicalKey]; !allowed || strings.ContainsAny(value, "\r\n") {
			continue
		}
		ctx.Writer.Header().Set(canonicalKey, value)
	}
	if ctx.Writer.Header().Get(contentTypeHeader) == "" {
		ctx.Writer.Header().Set(contentTypeHeader, "application/json; charset=utf-8")
	}
	ctx.Writer.WriteHeader(responseStatus)
	_, _ = ctx.Writer.Write(response.Body)
}
