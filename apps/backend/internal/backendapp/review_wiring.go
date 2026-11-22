package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/hostutility"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/review"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
	utilitytemplate "github.com/kandev/kandev/internal/utility/template"
)

// The adapters below live in this package for cycle avoidance: internal/review
// must not import the agent runtime, and the runtime must not import review.
// Both are already visible here.

// reviewComponents is the assembled native code-review subsystem.
type reviewComponents struct {
	service *taskservice.ReviewService
	runner  *review.Runner
}

// buildReviewComponents assembles the review service and runner from the app's
// existing collaborators.
//
// Every lookup is nil-tolerant, so a partially-configured deployment degrades to
// "no capable reviewer" (a clear, actionable failure) instead of panicking.
func buildReviewComponents(p routeParams) reviewComponents {
	service := taskservice.NewReviewService(p.taskRepo, p.eventBus, p.log)
	// Gate cross-task finding writes by workspace ownership, matching the plan
	// and walkthrough services. Under enforced auth an agent may only publish
	// to a task within its reach; unscoped callers (the built-in review runner,
	// auth disabled) are unaffected.
	service.SetTaskAuthorizer(p.taskSvc.AuthorizeTaskAccess)

	resolver := review.NewResolver(
		reviewProfileLookup{profiles: p.agentSettingsRepo, execution: p.services.DynamicProfileResolver},
		reviewUtilityLookup{agents: p.services.Utility},
		reviewDefaultsLookup{settings: p.services.User},
	)
	runner := review.NewRunner(review.RunnerDeps{
		Store:            service,
		Resolver:         resolver,
		Changes:          handlers.NewReviewChangeSource(p.lifecycleMgr, reviewSessionReader{repo: p.taskRepo}),
		Inference:        reviewInference{session: p.lifecycleMgr, host: p.hostUtilityMgr, profiles: p.services.DynamicProfileResolver},
		InferenceContext: reviewInferenceContext(p.taskRepo),
		Prompts:          review.NewTemplatePromptBuilder(reviewTemplateSource{agents: p.services.Utility, engine: utilitytemplate.NewEngine()}),
		TaskContext:      reviewTaskContext{tasks: p.taskSvc},
		Sessions:         reviewSessionLookup{sessions: p.taskRepo},
		Logger:           p.log,
	})
	return reviewComponents{service: service, runner: runner}
}

// reviewSessionReader supplies the base commit and branch a cumulative diff is
// computed against, mirroring sessionReaderAdapter in gateway.go.
type reviewSessionReader struct {
	repo reviewSessionGetter
}

type reviewSessionGetter interface {
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
}

func (r reviewSessionReader) GetSessionBaseCommit(ctx context.Context, sessionID string) string {
	session, err := r.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	return session.BaseCommitSHA
}

func (r reviewSessionReader) GetSessionBaseBranch(ctx context.Context, sessionID string) string {
	session, err := r.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	return session.BaseBranch
}

// reviewProfileLookup resolves an agent profile for the workflow-step review
// path, which is what lets a step review with a different model than it
// implemented with.
type reviewProfileLookup struct {
	profiles  agentProfileGetter
	execution reviewExecutionResolver
}

type reviewExecutionResolver interface {
	utilityservice.ExecutionDetailsResolver
	utilityservice.ExecutionFailureResolver
}

// agentProfileGetter is the slice of the agent-settings store the review
// resolver needs.
type agentProfileGetter interface {
	GetAgentProfile(ctx context.Context, id string) (*agentsettingsmodels.AgentProfile, error)
}

func (l reviewProfileLookup) ReviewerProfile(ctx context.Context, profileID string) (review.ReviewerProfile, bool, error) {
	if l.profiles == nil {
		return review.ReviewerProfile{}, false, nil
	}
	profile, err := l.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		// Only a genuinely absent row is "not found" — a step may reference a
		// profile the user has since deleted. Any other error is a storage fault
		// and must surface as such; collapsing it into "not configured" would
		// tell the user to go configure an agent they already have.
		if errors.Is(err, sql.ErrNoRows) {
			return review.ReviewerProfile{}, false, nil
		}
		return review.ReviewerProfile{}, false, err
	}
	if profile == nil {
		return review.ReviewerProfile{}, false, nil
	}
	var route *review.ReviewerRoute
	if l.execution != nil {
		execution, err := l.execution.ResolveExecutionDetails(ctx, "", profileID)
		if err != nil {
			return review.ReviewerProfile{}, false, err
		}
		profile = execution.Profile
		if profile == nil {
			return review.ReviewerProfile{}, false, fmt.Errorf("review profile resolved no execution")
		}
		copy := *profile
		copy.AgentID = execution.AgentName
		profile = &copy
		route = reviewerRoute(execution)
	}
	return review.ReviewerProfile{
		AgentID:        profile.AgentID,
		Model:          profile.Model,
		Mode:           profile.Mode,
		CLIPassthrough: profile.CLIPassthrough,
		Name:           profile.Name,
		Route:          route,
	}, true, nil
}

// reviewUtilityLookup resolves the built-in code-review utility agent, which
// supplies the default reviewer identity for the on-demand path.
type reviewUtilityLookup struct {
	agents utilityAgentGetter
}

type utilityAgentGetter interface {
	GetAgentByID(ctx context.Context, id string) (*utilitymodels.UtilityAgent, error)
}

func (l reviewUtilityLookup) CodeReviewAgent(ctx context.Context) (string, string, bool, bool, error) {
	if l.agents == nil {
		return "", "", false, false, nil
	}
	agent, err := l.agents.GetAgentByID(ctx, utilitystore.CodeReviewAgentID)
	if err != nil || agent == nil {
		return "", "", false, false, nil
	}
	return agent.AgentID, agent.Model, agent.Enabled, true, nil
}

func (l reviewUtilityLookup) CodeReviewAgentProfile(ctx context.Context) (string, bool, bool, error) {
	if l.agents == nil {
		return "", false, false, nil
	}
	agent, err := l.agents.GetAgentByID(ctx, utilitystore.CodeReviewAgentID)
	if err != nil || agent == nil {
		return "", false, false, err
	}
	return agent.AgentProfileID, agent.Enabled, true, nil
}

// reviewDefaultsLookup resolves the user's default utility agent/model pair.
type reviewDefaultsLookup struct {
	settings defaultUtilitySettingsGetter
}

type defaultUtilitySettingsGetter interface {
	GetDefaultUtilitySettings(ctx context.Context) (agentID, model string, err error)
}

func (l reviewDefaultsLookup) DefaultUtilitySettings(ctx context.Context) (string, string, error) {
	if l.settings == nil {
		return "", "", nil
	}
	return l.settings.GetDefaultUtilitySettings(ctx)
}

func (l reviewDefaultsLookup) DefaultUtilityProfileID(ctx context.Context) (string, error) {
	if l.settings == nil {
		return "", nil
	}
	provider, ok := l.settings.(interface {
		GetDefaultUtilityAgentProfileID(context.Context) (string, error)
	})
	if !ok {
		return "", nil
	}
	return provider.GetDefaultUtilityAgentProfileID(ctx)
}

// reviewTemplateSource serves the code-review utility agent's stored prompt, so
// a user who edits that prompt changes how reviews are performed.
type reviewTemplateSource struct {
	agents utilityAgentGetter
	engine *utilitytemplate.Engine
}

func (s reviewTemplateSource) ReviewPromptTemplate(ctx context.Context) (string, error) {
	if s.agents == nil {
		return "", fmt.Errorf("utility agents are not available")
	}
	agent, err := s.agents.GetAgentByID(ctx, utilitystore.CodeReviewAgentID)
	if err != nil {
		return "", fmt.Errorf("load code-review prompt: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("the code-review utility agent is missing")
	}
	return agent.Prompt, nil
}

func (s reviewTemplateSource) ResolveTemplate(_ context.Context, template string, values map[string]string) (string, error) {
	// MissingAsEmpty: the review prompt is rendered outside a task session's
	// template context, so an unused placeholder must resolve to empty rather
	// than being sent to the model as a literal "{{Var}}".
	return s.engine.ResolveWithOptions(template, &utilitytemplate.Context{Custom: values},
		utilitytemplate.ResolveOptions{MissingAsEmpty: true})
}

// reviewInference runs one reviewer prompt.
//
// It prefers the session-bound agentctl path so the review executes inside the
// task's own workspace, and falls back to the long-lived host agentctl instance
// when that session has no live execution — a review must still work on a task
// whose agent is not currently running.
type reviewInference struct {
	session  sessionInferenceRunner
	host     *hostutility.Manager
	profiles reviewExecutionResolver
}

type sessionInferenceRunner interface {
	ExecuteInferencePrompt(ctx context.Context, sessionID, agentID, model, prompt string) (*agentctlutil.PromptResponse, error)
}

type sessionProfileInferenceRunner interface {
	ExecuteInferenceProfilePrompt(ctx context.Context, sessionID, profileID, prompt string) (*agentctlutil.PromptResponse, error)
}

func (r reviewInference) Run(ctx context.Context, identity review.ReviewerIdentity, sessionID, prompt string) (*review.PromptResult, error) {
	prepared := reviewPromptRoute(identity)
	response, err := utilityservice.ExecuteProfilePrompt(ctx, r.profiles, prepared, func(attempt context.Context, id string) (*agentctlutil.PromptResponse, error) {
		concrete := identity
		concrete.ProfileID = id
		concrete.AgentID, concrete.Model = prepared.AgentCLI, prepared.Model
		return r.runConcrete(attempt, concrete, sessionID, prompt)
	})
	if identity.Route != nil {
		identity.Route.ExecutionProfileID, identity.Route.Generation = prepared.ExecutionProfileID, prepared.RouteGeneration
		identity.Route.AgentID, identity.Route.Model = prepared.AgentCLI, prepared.Model
	}
	if err != nil {
		return nil, err
	}
	if response == nil || !response.Success {
		message := "review inference returned no successful result"
		if response != nil && response.Error != "" {
			message = response.Error
		}
		return nil, errors.New(message)
	}
	return &review.PromptResult{Response: response.Response, Model: response.Model, PromptTokens: response.PromptTokens,
		ResponseTokens: response.ResponseTokens, DurationMs: response.DurationMs}, nil
}

func (r reviewInference) runConcrete(ctx context.Context, identity review.ReviewerIdentity, sessionID, prompt string) (*agentctlutil.PromptResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.session != nil && sessionID != "" {
		resp, err := r.runSession(ctx, identity, sessionID, prompt)
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
		if reviewResponseFinal(resp, err) {
			return resp, err
		}
		if reviewSessionErrorFinal(err, r.host != nil, identity.AgentID) {
			return resp, err
		}
	}
	if r.host == nil {
		return nil, fmt.Errorf("no inference executor is configured")
	}
	var result *hostutility.PromptResult
	var err error
	if identity.ProfileID != "" {
		result, err = r.host.ExecuteProfilePrompt(ctx, identity.ProfileID, prompt)
	} else {
		result, err = r.host.ExecutePrompt(ctx, identity.AgentID, identity.Model, identity.Mode, prompt)
	}
	if err != nil {
		return nil, err
	}
	return &agentctlutil.PromptResponse{
		Success:        true,
		Response:       result.Response,
		Model:          result.Model,
		PromptTokens:   result.PromptTokens,
		ResponseTokens: result.ResponseTokens,
		DurationMs:     result.DurationMs,
	}, nil
}

func (r reviewInference) runSession(ctx context.Context, identity review.ReviewerIdentity, sessionID, prompt string) (*agentctlutil.PromptResponse, error) {
	if identity.ProfileID == "" {
		return r.session.ExecuteInferencePrompt(ctx, sessionID, identity.AgentID, identity.Model, prompt)
	}
	profileRunner, ok := r.session.(sessionProfileInferenceRunner)
	if !ok {
		return nil, fmt.Errorf("profile-aware review executor is unavailable")
	}
	return profileRunner.ExecuteInferenceProfilePrompt(ctx, sessionID, identity.ProfileID, prompt)
}

func reviewResponseFinal(response *agentctlutil.PromptResponse, err error) bool {
	return response != nil && (response.Response != "" || (err == nil && (response.Success || response.Error != "")))
}

func reviewSessionErrorFinal(err error, hasHost bool, provider string) bool {
	if err == nil {
		return false
	}
	if !hasHost || agentruntime.IsInferenceCapacityError(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	// Classified provider failures belong to the shared candidate policy.
	// A host retry of that candidate would bypass its saved retry/skip decision.
	failure := routingerr.Classify(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: provider, Stderr: err.Error()})
	return failure.FallbackAllowed
}

func reviewerRoute(execution agentruntime.ProfileExecution) *review.ReviewerRoute {
	return &review.ReviewerRoute{ExecutionProfileID: execution.ExecutionProfileID, SessionID: execution.RouteSessionID,
		Generation: execution.Generation, AgentID: execution.AgentName, Model: execution.Profile.Model}
}

func reviewPromptRoute(identity review.ReviewerIdentity) *utilityservice.PromptRequest {
	prepared := &utilityservice.PromptRequest{AgentProfileID: identity.ProfileID, ExecutionProfileID: identity.ProfileID,
		AgentCLI: identity.AgentID, Model: identity.Model}
	if route := identity.Route; route != nil {
		prepared.ExecutionProfileID, prepared.RouteSessionID, prepared.RouteGeneration = route.ExecutionProfileID, route.SessionID, route.Generation
		prepared.AgentCLI, prepared.Model = route.AgentID, route.Model
	}
	return prepared
}

// reviewTaskContext supplies the task-level facts the review prompt interpolates.
type reviewTaskContext struct {
	tasks reviewTaskGetter
}

type reviewTaskGetter interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
}

func (c reviewTaskContext) ReviewPromptContext(ctx context.Context, taskID, _ string) (review.PromptContext, error) {
	if c.tasks == nil {
		return review.PromptContext{}, nil
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return review.PromptContext{}, err
	}
	return review.PromptContext{
		TaskTitle:       task.Title,
		TaskDescription: task.Description,
	}, nil
}

// reviewSessionLookup picks which session's workspace a review reads. A task's
// sessions share one workspace, so any session will do; the primary is preferred
// so the choice is stable across runs.
type reviewSessionLookup struct {
	sessions reviewSessionLister
}

type reviewSessionLister interface {
	ListTaskSessions(ctx context.Context, taskID string) ([]*taskmodels.TaskSession, error)
}

func (l reviewSessionLookup) ReviewSessionID(ctx context.Context, taskID string) (string, error) {
	if l.sessions == nil {
		return "", nil
	}
	sessions, err := l.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", err
	}
	fallback := ""
	for _, s := range sessions {
		if s == nil {
			continue
		}
		if s.IsPrimary {
			return s.ID, nil
		}
		if fallback == "" {
			fallback = s.ID
		}
	}
	return fallback, nil
}
