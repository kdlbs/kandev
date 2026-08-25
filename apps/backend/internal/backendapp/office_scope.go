package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Per-user scoping for Office HTTP routes.
//
// Office endpoints are dual-consumed: sandbox agents authenticate with a
// workspace-scoped JWT (validated and workspace-claim-checked by
// AgentAuthMiddleware, which sets an agent caller in context), while browser
// users authenticate with a session cookie and must own the target workspace.
//
// The first version of this middleware only understood `:wsId`, and its doc
// comment said so openly: "routes without a :wsId param ... remain governed by
// AgentAuthMiddleware". That premise was wrong for browser callers.
// AgentAuthMiddleware constrains only JWT callers, and every by-ID handler's
// own check is of the form `if caller := agentCallerFromCtx(c); caller != nil`
// — which is a no-op for a session cookie. So ~50 by-ID routes (agents,
// memory, documents, approvals, agent trees) reached another user's resources
// with nothing but a guessed id.
//
// This is the structural backstop, modelled on the WS gateway's
// dispatch_scope.go: rather than trusting ~50 present and future handlers to
// remember an ownership check, the route's own resource id is resolved to its
// owning workspace here, before dispatch. A newly added Office route is safe
// by default — and if it names a resource kind nobody registered a resolver
// for, it is DENIED rather than silently exempted. TestOfficeRouteScope
// Completeness turns that runtime denial into a build-time failure.

// officeRoutePrefix is the group the Office routes are mounted under. Route
// patterns are matched relative to it so the tables below read like the
// RegisterRoutes calls they mirror.
const officeRoutePrefix = "/api/v1/office"

// officeWorkspaceResolver answers "which workspace owns this id". Every
// implementation must return an error (not "") for an unknown id — see the
// fail-closed note on authorizeOfficeWorkspace.
type officeWorkspaceResolver func(ctx context.Context, id string) (string, error)

// officeParamScopeResolvers maps a `<collection>/:<param>` pair appearing in
// an Office route pattern to the lookup that resolves that id's workspace.
//
// Keying on the pair rather than the whole pattern is what makes this survive
// new routes: `/agents/:id/whatever-ships-next` is covered by the same
// `agents/:id` entry that covers `/agents/:id/memory` today.
//
// Every such pair in a pattern is resolved and authorized, not just the
// first, so `/agents/:id/channels/:channelId` checks the channel as well as
// the agent.
//
// Params naming a child of a resource checked on the same route have no
// entry here; they are listed in officeScopedSubResourceParams instead.
func officeParamScopeResolvers(repo *officesqlite.Repository) map[string]officeWorkspaceResolver {
	return map[string]officeWorkspaceResolver{
		"agents/:id":                  repo.WorkspaceIDForAgent,
		"tasks/:id":                   repo.WorkspaceIDForTask,
		"routines/:id":                repo.WorkspaceIDForRoutine,
		"routine-triggers/:triggerId": repo.WorkspaceIDForRoutineTrigger,
		"routine-triggers/:publicId":  repo.WorkspaceIDForRoutineTriggerPublicID,
		"projects/:id":                repo.WorkspaceIDForProject,
		"skills/:id":                  repo.WorkspaceIDForSkill,
		"budgets/:id":                 repo.WorkspaceIDForBudget,
		"approvals/:id":               repo.WorkspaceIDForApproval,
		"channels/:channelId":         repo.WorkspaceIDForChannel,
		// A reviewer/approver is an agent, and must live in the same
		// workspace as the task it is being attached to.
		"reviewers/:agentId": repo.WorkspaceIDForAgent,
		"approvers/:agentId": repo.WorkspaceIDForAgent,
		// Wrapped rather than taken as a method value: GetRunWorkspaceID is
		// promoted from the embedded *runssqlite.Repository, so `repo.Get...`
		// dereferences repo at map-build time and would panic on the
		// fail-closed nil-repository path below.
		"runs/:id":    func(ctx context.Context, id string) (string, error) { return repo.GetRunWorkspaceID(ctx, id) },
		"runs/:runId": func(ctx context.Context, id string) (string, error) { return repo.GetRunWorkspaceID(ctx, id) },
	}
}

// officeScopedSubResourceParams are `<collection>/:<param>` pairs that name a
// child of a resource whose own pair is checked on the same route, so they
// need no resolver of their own: a document key is reachable only as
// `/tasks/:id/documents/:key`, and every handler reading one of these looks it
// up under its parent (DeleteAgentMemoryOwned(agentID, entryID), and so on).
//
// This list is what keeps "no resolver" from meaning "allowed". Any OTHER
// unresolvable param on an Office route is denied at runtime and fails
// TestOfficeRouteScopeCompleteness at build time.
var officeScopedSubResourceParams = map[string]string{
	"documents/:key":         "a task document, addressed under its task",
	"revisions/:revId":       "a revision of a task document, addressed under its task",
	"instructions/:filename": "an instruction file, addressed under its agent",
	"memory/:entryId":        "a memory entry, addressed under its agent",
	"blockers/:blockerId":    "a task blocker, addressed under its task",
}

// officeWorkspacelessRoutes are the Office routes that legitimately carry no
// workspace, mapped to the reason. Enumerated rather than left as an implicit
// "no id found => allow", so adding a route cannot opt itself out by accident.
var officeWorkspacelessRoutes = map[string]string{
	"/meta": "static enum/metadata payload (statuses, roles, executor types); reads no per-user data",
	"/onboarding-state": "pre-workspace bootstrap: reports whether ANY office workspace exists yet, " +
		"so there is no workspace to scope it to",
	"/onboarding/complete":  "creates the first workspace; nothing to authorize against beforehand",
	"/onboarding/import-fs": "imports office config from the local filesystem into a new workspace",
}

// officeWorkspacelessPrefixes are workspace-less route groups, mapped to the
// reason.
var officeWorkspacelessPrefixes = map[string]string{
	"/runtime/": "agent-runtime callbacks. Every handler calls contextFromRequest, which rejects any " +
		"request without a valid agent JWT and derives workspace/task/run from its claims, so there is " +
		"no session-cookie surface here for this guard to protect",
}

// officeBodyScopeResolvers covers routes that name their resource in the JSON
// body instead of the path. Keyed by route pattern (relative to
// officeRoutePrefix); the resolver reads the parsed body and returns the id
// plus the param key naming the resolver to use.
var officeBodyScopeResolvers = map[string]func(body []byte) (resolverKey, id string, ok bool){
	"/inbox/dismiss": inboxDismissScopeRef,
}

// inboxDismissScopeRef maps a Mark-fixed request onto the resource its kind
// names: agent_run_failed carries a run id, agent_paused_after_failures an
// agent id (see dashboard.MarkFixedHandler's signatures).
//
// A body it cannot read is refused, so a malformed or unknown-kind dismiss
// answers 404 rather than the handler's 400 once auth is enabled. That is the
// fail-closed side of the trade and only affects invalid input.
func inboxDismissScopeRef(body []byte) (string, string, bool) {
	var req struct {
		Kind   string `json:"kind"`
		ItemID string `json:"item_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.ItemID == "" {
		return "", "", false
	}
	switch req.Kind {
	case "agent_run_failed":
		return "runs/:id", req.ItemID, true
	case "agent_paused_after_failures":
		return "agents/:id", req.ItemID, true
	default:
		return "", "", false
	}
}

// maxOfficeScopeBody bounds the body this middleware buffers to resolve a
// body-keyed route. Comfortably above the two-field dismiss payload; a larger
// body is refused rather than truncated, so the handler never sees a
// silently-shortened request.
const maxOfficeScopeBody = 64 * 1024

// officeWorkspaceScopeMiddleware enforces per-user workspace ownership on
// every Office route, whether it is keyed by `:wsId` or by a resource id.
func officeWorkspaceScopeMiddleware(
	authSvc *auth.Service, taskSvc *taskservice.Service, officeRepo *officesqlite.Repository,
) gin.HandlerFunc {
	resolvers := officeParamScopeResolvers(officeRepo)
	return func(c *gin.Context) {
		if authSvc == nil || authSvc.Mode() == auth.ModeDisabled {
			c.Next()
			return
		}
		// Agent JWT callers are already constrained to their workspace claim.
		if officeagents.CallerFromContext(c) != nil {
			c.Next()
			return
		}
		if err := authorizeOfficeRequest(c, taskSvc, officeRepo, resolvers); err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}
		c.Next()
	}
}

// authorizeOfficeRequest returns nil only when the caller is allowed to reach
// this route: either it is explicitly workspace-less, or every workspace the
// route's ids resolve to is one the caller may access.
func authorizeOfficeRequest(
	c *gin.Context,
	taskSvc *taskservice.Service,
	officeRepo *officesqlite.Repository,
	resolvers map[string]officeWorkspaceResolver,
) error {
	route, ok := officeRelativeRoute(c.FullPath())
	if !ok {
		// Unreachable in the mounted binary (group middleware only runs for
		// routes registered in the group), so an unrecognisable pattern means
		// something is wired wrong. Deny.
		return repoerrors.ErrWorkspaceNotFound
	}
	if _, allowed := officeWorkspacelessRoute(route); allowed {
		return nil
	}
	if wsID := c.Param("wsId"); wsID != "" {
		return taskSvc.AuthorizeWorkspaceAccess(c.Request.Context(), wsID)
	}
	// officeRepo == nil fails closed for the same reason runSubscriptionCheck
	// does: this is a security check, not a visibility fallback.
	if officeRepo == nil {
		return repoerrors.ErrWorkspaceNotFound
	}
	refs, err := officeScopeRefs(c, route)
	if err != nil || len(refs) == 0 {
		return repoerrors.ErrWorkspaceNotFound
	}
	scoped := false
	for key, id := range refs {
		resolve, known := resolvers[key]
		if !known {
			if _, sub := officeScopedSubResourceParams[key]; sub {
				continue
			}
			return repoerrors.ErrWorkspaceNotFound
		}
		if err := authorizeOfficeWorkspace(c.Request.Context(), taskSvc, resolve, id); err != nil {
			return err
		}
		scoped = true
	}
	if !scoped {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// authorizeOfficeWorkspace resolves one id and authorizes the workspace it
// belongs to.
//
// The empty-resolution branch is the trap runSubscriptionCheck documents:
// AuthorizeWorkspaceAccess reads workspaceID == "" as "no workspace scoping
// applies" and returns nil, so handing it an unresolved id would turn this
// guard into an unconditional allow. Deny before it ever gets there.
func authorizeOfficeWorkspace(
	ctx context.Context, taskSvc *taskservice.Service, resolve officeWorkspaceResolver, id string,
) error {
	if id == "" {
		return repoerrors.ErrWorkspaceNotFound
	}
	workspaceID, err := resolve(ctx, id)
	if err != nil {
		return err
	}
	if workspaceID == "" {
		return repoerrors.ErrWorkspaceNotFound
	}
	return taskSvc.AuthorizeWorkspaceAccess(ctx, workspaceID)
}

// officeScopeRefs returns the resource ids this request names, keyed by the
// resolver key that resolves each one.
func officeScopeRefs(c *gin.Context, route string) (map[string]string, error) {
	refs := map[string]string{}
	for _, key := range officeRouteParamKeys(route) {
		refs[key] = c.Param(strings.TrimPrefix(paramOfScopeKey(key), ":"))
	}
	if len(refs) > 0 {
		return refs, nil
	}
	bodyRef, keyed := officeBodyScopeResolvers[route]
	if !keyed {
		return nil, nil
	}
	body, err := bufferRequestBody(c)
	if err != nil {
		return nil, err
	}
	key, id, ok := bodyRef(body)
	if !ok {
		return nil, nil
	}
	refs[key] = id
	return refs, nil
}

// errOfficeScopeBodyTooLarge refuses a body-keyed route whose body exceeds
// what this guard will buffer.
var errOfficeScopeBodyTooLarge = errors.New("office scope: request body too large to authorize")

// bufferRequestBody reads the body so the middleware can inspect it and
// leaves an equivalent reader in place for the handler.
func bufferRequestBody(c *gin.Context) ([]byte, error) {
	if c.Request == nil || c.Request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxOfficeScopeBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxOfficeScopeBody {
		return nil, errOfficeScopeBodyTooLarge
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// officeRouteParamKeys returns every `<collection>/:<param>` pair in a route
// pattern, in path order.
func officeRouteParamKeys(route string) []string {
	segments := strings.Split(strings.Trim(route, "/"), "/")
	var keys []string
	for i := 0; i+1 < len(segments); i++ {
		if strings.HasPrefix(segments[i+1], ":") {
			keys = append(keys, segments[i]+"/"+segments[i+1])
		}
	}
	return keys
}

// paramOfScopeKey returns the `:param` half of a `<collection>/:<param>` key.
func paramOfScopeKey(key string) string {
	_, param, _ := strings.Cut(key, "/")
	return param
}

// officeRelativeRoute strips the Office group prefix off a gin FullPath.
func officeRelativeRoute(fullPath string) (string, bool) {
	if !strings.HasPrefix(fullPath, officeRoutePrefix+"/") {
		return "", false
	}
	return strings.TrimPrefix(fullPath, officeRoutePrefix), true
}

// officeWorkspacelessRoute reports whether a route is on the explicit
// workspace-less allowlist, and why.
func officeWorkspacelessRoute(route string) (string, bool) {
	if reason, ok := officeWorkspacelessRoutes[route]; ok {
		return reason, true
	}
	for prefix, reason := range officeWorkspacelessPrefixes {
		if strings.HasPrefix(route, prefix) {
			return reason, true
		}
	}
	return "", false
}
