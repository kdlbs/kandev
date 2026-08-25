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
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office"
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

// officeWorkspaceParam is the path param naming a workspace directly.
const officeWorkspaceParam = ":wsId"

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
		// Sibling ids on a `:wsId` route. The label handlers update and
		// delete by label id alone and read task labels by task id alone,
		// ignoring the `:wsId` they are mounted under, so authorizing only
		// the workspace leaves them reachable across workspaces.
		"labels/:id":    repo.WorkspaceIDForLabel,
		"tasks/:taskId": repo.WorkspaceIDForTask,
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
	"labels/:labelName":      "a label name, resolved under its task and workspace by the service",
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
	refs, err := officeScopeRefs(c, route)
	if err != nil {
		return repoerrors.ErrWorkspaceNotFound
	}
	if officeagents.CallerFromContext(c) != nil {
		return authorizeOfficeAgentCaller(c, officeRepo, resolvers, refs)
	}
	return authorizeOfficeUser(c, taskSvc, officeRepo, resolvers, refs)
}

// authorizeOfficeUser authorizes a browser/session caller: every workspace
// the route names, by `:wsId` or by resource id, must be one they may access.
//
// `:wsId` does NOT short-circuit the id checks. It used to, and that left the
// mixed-parameter routes open: the label handlers take `/workspaces/:wsId/
// labels/:id` and then call UpdateLabel/DeleteLabel with the label id alone,
// so pairing an owned workspace id with a foreign label id mutated another
// user's label. `/workspaces/:wsId/tasks/:taskId/...` has the same shape.
func authorizeOfficeUser(
	c *gin.Context,
	taskSvc *taskservice.Service,
	officeRepo *officesqlite.Repository,
	resolvers map[string]officeWorkspaceResolver,
	refs map[string]string,
) error {
	ctx := c.Request.Context()
	route := &officeRouteWorkspaces{}
	scoped := false
	if wsID := c.Param("wsId"); wsID != "" {
		if err := taskSvc.AuthorizeWorkspaceAccess(ctx, wsID); err != nil {
			return err
		}
		_ = route.consistent(wsID) // first entry, cannot conflict
		scoped = true
	}
	// officeRepo == nil fails closed for the same reason runSubscriptionCheck
	// does: this is a security check, not a visibility fallback.
	if len(refs) > 0 && officeRepo == nil {
		return repoerrors.ErrWorkspaceNotFound
	}
	resolved, err := authorizeOfficeRefs(ctx, refs, resolvers, func(workspaceID string) error {
		if err := route.consistent(workspaceID); err != nil {
			return err
		}
		return taskSvc.AuthorizeWorkspaceAccess(ctx, workspaceID)
	})
	if err != nil {
		return err
	}
	if !scoped && !resolved {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// authorizeOfficeAgentCaller authorizes a sandbox agent's JWT against the
// resources the route names.
//
// AgentAuthMiddleware compares the token's workspace claim to `:wsId` only,
// which is no comparison at all on a by-ID route — and the handlers' own
// agent-caller guards check ROLE and self-identity (isAdminRole, caller.ID ==
// target), never workspace. So a CEO-role token minted in workspace A could
// read, update or delete a workspace B agent. Resolving the route's ids and
// requiring them to land in the caller's own workspace is what closes that;
// the token stays confined to the workspace it was minted for.
func authorizeOfficeAgentCaller(
	c *gin.Context,
	officeRepo *officesqlite.Repository,
	resolvers map[string]officeWorkspaceResolver,
	refs map[string]string,
) error {
	callerWorkspace := officeCallerWorkspace(c)
	if callerWorkspace == "" {
		return repoerrors.ErrWorkspaceNotFound
	}
	if len(refs) == 0 {
		// A `:wsId` route (already compared against the claim upstream) or a
		// body-keyed route whose body named nothing resolvable.
		if c.Param("wsId") != "" {
			return nil
		}
		return repoerrors.ErrWorkspaceNotFound
	}
	if officeRepo == nil {
		return repoerrors.ErrWorkspaceNotFound
	}
	// Requiring every id to equal the token's workspace also gives the agent
	// path the same-workspace relationship rule officeRouteWorkspaces gives
	// the user path, by transitivity.
	_, err := authorizeOfficeRefs(c.Request.Context(), refs, resolvers, func(workspaceID string) error {
		if workspaceID != callerWorkspace {
			return repoerrors.ErrWorkspaceNotFound
		}
		return nil
	})
	return err
}

// officeCallerWorkspace is the workspace an agent token is confined to. The
// JWT claim is authoritative (it is what AgentAuthMiddleware compares against
// `:wsId`); the caller agent's own workspace is the fallback for a token
// minted without one. Empty means "cannot be scoped", which the caller denies.
func officeCallerWorkspace(c *gin.Context) string {
	if claims := officeagents.ClaimsFromContext(c); claims != nil && claims.WorkspaceID != "" {
		return claims.WorkspaceID
	}
	if caller := officeagents.CallerFromContext(c); caller != nil {
		return caller.WorkspaceID
	}
	return ""
}

// officeRouteWorkspaces enforces the RELATIONSHIP between a route's ids: they
// must all name the same workspace.
//
// Authorizing each id on its own is not enough. A caller who owns two
// workspaces passes the ownership check on both, so `/workspaces/<A2>/labels/
// <label in A1>` satisfied every individual check while still crossing a
// workspace boundary — and `UpdateLabel` then edits by label id alone. No
// Office route legitimately names two workspaces: a task's reviewers, an
// agent's channels and runs, and a workspace's labels are all intra-workspace
// by construction. The quorum handler had already hand-rolled this check
// (`task.WorkspaceID != workspaceID`), which is what a missing structural
// rule looks like just before it gets forgotten on the next route.
type officeRouteWorkspaces struct{ first string }

// consistent records a workspace the route names and rejects a second,
// different one.
func (w *officeRouteWorkspaces) consistent(workspaceID string) error {
	if w.first == "" {
		w.first = workspaceID
		return nil
	}
	if w.first != workspaceID {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// authorizeOfficeRefs resolves every id the route names and hands each
// resolved workspace to check. Reports whether any id was actually
// authorized, so callers can tell "allowed" from "nothing was checked".
//
// The empty-resolution branch is the trap runSubscriptionCheck documents:
// AuthorizeWorkspaceAccess reads workspaceID == "" as "no workspace scoping
// applies" and returns nil, so handing it an unresolved id would turn this
// guard into an unconditional allow. Deny before it ever gets there.
//
// An id whose `<collection>/:<param>` pair has no resolver denies, unless it
// is a listed child of a resource checked on the same route — "no resolver"
// must never mean "allowed".
func authorizeOfficeRefs(
	ctx context.Context,
	refs map[string]string,
	resolvers map[string]officeWorkspaceResolver,
	check func(workspaceID string) error,
) (bool, error) {
	authorized := false
	for key, id := range refs {
		resolve, known := resolvers[key]
		if !known {
			if _, sub := officeScopedSubResourceParams[key]; sub {
				continue
			}
			return false, repoerrors.ErrWorkspaceNotFound
		}
		if id == "" {
			return false, repoerrors.ErrWorkspaceNotFound
		}
		workspaceID, err := resolve(ctx, id)
		if err != nil {
			return false, err
		}
		if workspaceID == "" {
			return false, repoerrors.ErrWorkspaceNotFound
		}
		if err := check(workspaceID); err != nil {
			return false, err
		}
		authorized = true
	}
	return authorized, nil
}

// officeScopeRefs returns the resource ids this request names, keyed by the
// resolver key that resolves each one.
func officeScopeRefs(c *gin.Context, route string) (map[string]string, error) {
	refs := map[string]string{}
	for _, key := range officeRouteParamKeys(route) {
		if paramOfScopeKey(key) == officeWorkspaceParam {
			continue // authorized directly, not through a resolver
		}
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

// mountOfficeRoutes builds the Office route group with its two middlewares
// and registers every Office handler on it.
//
// Extracted from the caller so TestOfficeRouteGroupMountsScopeGuard can pin
// the `.Use` lines themselves: every other test here mounts the guard by
// hand, so deleting it from the production group would leave all of them
// green while reopening the hole in the shipped binary. That is the same gap
// TestGatewayAuthPolicyWiresRunSubscriptionCheck exists to close for the WS
// gateway.
func mountOfficeRoutes(
	router *gin.Engine,
	svcs *office.Services,
	authSvc *auth.Service,
	taskSvc *taskservice.Service,
	officeRepo *officesqlite.Repository,
	log *logger.Logger,
) {
	api := router.Group(officeRoutePrefix)
	api.Use(officeagents.AgentAuthMiddleware(svcs.Agents))
	api.Use(officeWorkspaceScopeMiddleware(authSvc, taskSvc, officeRepo))
	office.RegisterAllRoutes(api, svcs, log)
}
