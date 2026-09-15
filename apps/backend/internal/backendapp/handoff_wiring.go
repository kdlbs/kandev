package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/logger"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/office/dashboard"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
)

// handoffLaunchAdapter narrows orchestrator.Service.LaunchSession to the
// officeruntime.HandoffLauncher seam, translating the launcher-agnostic
// HandoffLaunchRequest into a real session launch (AC-32).
// internal/office/runtime cannot import internal/orchestrator directly:
// internal/office/dashboard already imports internal/office/runtime, and
// internal/orchestrator imports internal/office/dashboard, so that import
// would close a cycle. This adapter lives in internal/backendapp instead,
// which can safely import both sides.
type handoffLaunchAdapter struct {
	svc *orchestrator.Service
}

func (a handoffLaunchAdapter) LaunchSession(ctx context.Context, req officeruntime.HandoffLaunchRequest) error {
	_, err := a.svc.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:            req.TaskID,
		Intent:            orchestrator.IntentStart,
		AgentProfileID:    req.AgentProfileID,
		ProfileExplicit:   true,
		ExecutorID:        req.ExecutorID,
		ExecutorProfileID: req.ExecutorProfileID,
		WorkflowStepID:    req.WorkflowStepID,
		Prompt:            req.Prompt,
	})
	return err
}

// handoffScopeAdapter narrows *mcpscope.Resolver to the
// officeruntime.HandoffWorkspaceScoper seam via ScopeOverridingIdentity
// rather than Scope. The handoff HTTP route is authenticated by its own
// runtime JWT (Handler.contextFromRequest in internal/office/runtime), so
// its authorization must always be decided against the source task's owner —
// never deferred to a different identity the global auth middleware may
// separately have attached to the same request context (a session cookie or
// PAT belonging to whichever user happens to be logged into the same
// browser, checked ahead of the office-route JWT deferral path). Scope's
// "preserve an existing identity" rule is correct only for in-session MCP
// dispatch, which has no credential of its own; reusing it here was Review
// round 4's codex-found identity-precedence defect.
type handoffScopeAdapter struct {
	resolver *mcpscope.Resolver
}

func (a handoffScopeAdapter) Scope(ctx context.Context, taskID string) (context.Context, error) {
	return a.resolver.ScopeOverridingIdentity(ctx, taskID)
}

// buildHandoffDependencies wires the concrete backend services that satisfy
// the officeruntime.HandoffDependencies seam for the cross-workspace handoff
// runtime action. It builds its own *mcpscope.Resolver (rather than reusing
// the one built for in-session MCP dispatch scoping) because that resolver
// is local to registerMCPAndDebugRoutes and runs before Office routes are
// mounted; the resolver itself is a small, stateless value, so duplicating
// its construction is cheaper than threading it across functions.
func buildHandoffDependencies(
	taskSvc *taskservice.Service,
	taskRepo *sqliterepo.Repository,
	workflowCtrl *workflowcontroller.Controller,
	agentSettingsCtrl *controller.Controller,
	orchestratorSvc *orchestrator.Service,
	dashboardSvc *dashboard.DashboardService,
	authSvc *auth.Service,
	log *logger.Logger,
) officeruntime.HandoffDependencies {
	scopeResolver := mcpscope.NewResolver(
		taskRepo,
		authSvc,
		func() bool { return authSvc != nil && authSvc.Mode() != auth.ModeDisabled },
		log,
	)

	var deps officeruntime.HandoffDependencies
	deps.Workspaces = handoffScopeAdapter{resolver: scopeResolver}
	deps.Tasks = taskSvc
	deps.Workflows = workflowCtrl
	deps.AgentProfiles = agentSettingsCtrl
	deps.ReverseLinks = taskRepo
	if orchestratorSvc != nil {
		deps.Launcher = handoffLaunchAdapter{svc: orchestratorSvc}
	}
	if dashboardSvc != nil {
		deps.Activity = dashboardSvc
	}
	return deps
}
