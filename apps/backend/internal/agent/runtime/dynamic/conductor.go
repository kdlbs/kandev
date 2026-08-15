package dynamic

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// DownstreamLaunch is the only launch input a dynamic conductor gives to a
// concrete runtime. In particular, a provider-native resume token is never
// sent to a different candidate.
type DownstreamLaunch struct {
	ExecutionProfileID string
	Decision           RouteDecision
	Prompt             string
	PriorACPSession    string
	Continuation       Continuation
}

type DownstreamExecution struct {
	ID                 string
	ExecutionProfileID string
	ACPSessionID       string
}

type DownstreamRuntime interface {
	Launch(context.Context, DownstreamLaunch) (DownstreamExecution, error)
	Resume(context.Context, string, string) error
	Stop(context.Context, string, string) error
}

type ProfileLoader interface {
	LoadDynamicProfile(context.Context, string) (Profile, error)
}

type ConductorOption func(*Conductor)

func WithContinuationBuilder(builder func(context.Context, ContinuationInput) (Continuation, error)) ConductorOption {
	return func(conductor *Conductor) { conductor.continuationBuilder = builder }
}

// Conductor owns the logical session while concrete runtimes own downstream
// process and ACP identities.
type Conductor struct {
	mu                  sync.Mutex
	engine              *Engine
	profiles            ProfileLoader
	downstream          DownstreamRuntime
	continuationBuilder func(context.Context, ContinuationInput) (Continuation, error)
	active              map[string]DownstreamExecution
}

func NewConductor(engine *Engine, profiles ProfileLoader, downstream DownstreamRuntime, options ...ConductorOption) *Conductor {
	conductor := &Conductor{
		engine: engine, profiles: profiles, downstream: downstream,
		active: make(map[string]DownstreamExecution),
	}
	for _, option := range options {
		option(conductor)
	}
	return conductor
}

type ConductorLaunch struct {
	SessionID          string
	LogicalProfileID   string
	ExpectedGeneration int64
	ExcludeProfileID   string
	Prompt             string
	Continuation       ContinuationInput
}

// ConductorSelectedLaunch hands the conductor a route that was already
// claimed by the shared resolver. It is used by lifecycle adapters that must
// preserve the existing session generation while still delegating classified
// pre-result fallback to the conductor.
type ConductorSelectedLaunch struct {
	SessionID        string
	LogicalProfileID string
	Decision         RouteDecision
	Prompt           string
	Continuation     ContinuationInput
}

type ConductorResult struct {
	Decision   RouteDecision
	Execution  DownstreamExecution
	FreshRoute bool
}

func (c *Conductor) Launch(ctx context.Context, request ConductorLaunch) (ConductorResult, error) {
	if c.engine == nil || c.profiles == nil || c.downstream == nil {
		return ConductorResult{}, errors.New("dynamic conductor is not configured")
	}
	profile, err := c.profiles.LoadDynamicProfile(ctx, request.LogicalProfileID)
	if err != nil {
		return ConductorResult{}, err
	}
	decision, err := c.engine.SelectContext(ctx, request.SessionID, profile, request.ExpectedGeneration, request.ExcludeProfileID)
	if err != nil {
		return ConductorResult{}, err
	}
	continuation, err := c.buildContinuation(ctx, request.Continuation)
	if err != nil {
		return ConductorResult{}, err
	}
	execution, decision, err := c.launchWithFallback(ctx, profile, decision, request, continuation)
	if err != nil {
		return ConductorResult{}, err
	}
	if execution.ExecutionProfileID == "" {
		execution.ExecutionProfileID = decision.ExecutionProfileID
	}
	c.mu.Lock()
	c.active[request.SessionID] = execution
	c.mu.Unlock()
	return ConductorResult{Decision: decision, Execution: execution, FreshRoute: true}, nil
}

// LaunchSelected launches a previously claimed route and applies the same
// classified fallback policy as Launch. The caller owns durable session
// attribution; the downstream adapter receives each immutable decision before
// its corresponding launch.
func (c *Conductor) LaunchSelected(ctx context.Context, request ConductorSelectedLaunch) (ConductorResult, error) {
	if c.engine == nil || c.profiles == nil || c.downstream == nil {
		return ConductorResult{}, errors.New("dynamic conductor is not configured")
	}
	profile, err := c.profiles.LoadDynamicProfile(ctx, request.LogicalProfileID)
	if err != nil {
		return ConductorResult{}, err
	}
	continuation, err := c.buildContinuation(ctx, request.Continuation)
	if err != nil {
		return ConductorResult{}, err
	}
	execution, decision, err := c.launchWithFallback(
		ctx,
		profile,
		request.Decision,
		ConductorLaunch{SessionID: request.SessionID, LogicalProfileID: request.LogicalProfileID, Prompt: request.Prompt},
		continuation,
	)
	if err != nil {
		return ConductorResult{}, err
	}
	if execution.ExecutionProfileID == "" {
		execution.ExecutionProfileID = decision.ExecutionProfileID
	}
	c.mu.Lock()
	c.active[request.SessionID] = execution
	c.mu.Unlock()
	return ConductorResult{Decision: decision, Execution: execution}, nil
}

// RouteAfterFailure applies the logical profile's classified failure policy
// without launching a downstream process. Event handlers use it when a
// lifecycle error frame arrives after the initial launch.
func (c *Conductor) RouteAfterFailure(
	ctx context.Context,
	sessionID, logicalProfileID, currentExecutionProfileID string,
	expectedGeneration int64,
	failure *routingerr.Error,
) (RouteDecision, error) {
	if c.engine == nil || c.profiles == nil {
		return RouteDecision{}, errors.New("dynamic conductor is not configured")
	}
	profile, err := c.profiles.LoadDynamicProfile(ctx, logicalProfileID)
	if err != nil {
		return RouteDecision{}, err
	}
	return c.engine.ApplyFailureContext(
		ctx, sessionID, profile, expectedGeneration, currentExecutionProfileID, failure,
	)
}

func (c *Conductor) launchWithFallback(
	ctx context.Context,
	profile Profile,
	decision RouteDecision,
	request ConductorLaunch,
	continuation Continuation,
) (DownstreamExecution, RouteDecision, error) {
	execution, err := c.downstream.Launch(ctx, DownstreamLaunch{
		ExecutionProfileID: decision.ExecutionProfileID,
		Decision:           decision,
		Prompt:             request.Prompt,
		Continuation:       continuation,
	})
	if err == nil {
		return execution, decision, nil
	}
	next, shouldFallback, fallbackErr := c.nextAfterLaunchFailure(
		ctx, profile, decision, request.SessionID, err,
	)
	if fallbackErr != nil {
		return DownstreamExecution{}, decision, fallbackErr
	}
	if !shouldFallback {
		return DownstreamExecution{}, decision, err
	}
	execution, err = c.downstream.Launch(ctx, DownstreamLaunch{
		ExecutionProfileID: next.ExecutionProfileID,
		Decision:           next,
		Prompt:             request.Prompt,
		Continuation:       continuation,
	})
	if err != nil {
		return DownstreamExecution{}, next, err
	}
	return execution, next, nil
}

func (c *Conductor) nextAfterLaunchFailure(
	ctx context.Context,
	profile Profile,
	decision RouteDecision,
	sessionID string,
	launchErr error,
) (RouteDecision, bool, error) {
	var classified *routingerr.Error
	if !errors.As(launchErr, &classified) {
		return RouteDecision{}, false, nil
	}
	if !classified.FallbackAllowed {
		return RouteDecision{}, false, nil
	}
	if c.engine.ActionFor(profile, decision.ExecutionProfileID, classified.Code) != ActionTryNext {
		return RouteDecision{}, false, nil
	}
	next, err := c.engine.ApplyFailureContext(
		ctx, sessionID, profile, decision.Generation, decision.ExecutionProfileID, classified,
	)
	if err != nil {
		return RouteDecision{}, true, err
	}
	return next, true, nil
}

// Resume reuses a downstream ACP session only when its concrete profile owns
// the current logical route. A caller that changes candidates must call Launch
// with the new generation, which always creates a fresh downstream session.
func (c *Conductor) Resume(ctx context.Context, sessionID, prompt string) error {
	c.mu.Lock()
	execution, ok := c.active[sessionID]
	c.mu.Unlock()
	if !ok {
		return errors.New("dynamic conductor has no active downstream execution")
	}
	return c.downstream.Resume(ctx, execution.ID, prompt)
}

func (c *Conductor) Stop(ctx context.Context, sessionID, reason string) error {
	c.mu.Lock()
	execution, ok := c.active[sessionID]
	delete(c.active, sessionID)
	c.mu.Unlock()
	if !ok {
		return nil
	}
	return c.downstream.Stop(ctx, execution.ID, reason)
}

func (c *Conductor) AcceptsGeneration(sessionID string, generation int64) bool {
	state, ok := c.engine.State(sessionID)
	return ok && state.Generation == generation
}

func (c *Conductor) buildContinuation(ctx context.Context, input ContinuationInput) (Continuation, error) {
	if c.continuationBuilder != nil {
		return c.continuationBuilder(ctx, input)
	}
	return BuildBoundedContinuation(input), nil
}

type ContinuationInput struct {
	TaskDescription   string
	WorkflowStep      string
	UserMessages      []string
	Conversation      string
	ToolSummary       string
	RepositorySummary string
	PlanSummary       string
	FailureReason     string
}

type Continuation struct {
	TaskDescription   string
	WorkflowStep      string
	Conversation      string
	ToolSummary       string
	RepositorySummary string
	PlanSummary       string
	FailureReason     string
}

const continuationFieldLimit = 4000

func BuildBoundedContinuation(input ContinuationInput) Continuation {
	return Continuation{
		TaskDescription:   bounded(input.TaskDescription),
		WorkflowStep:      bounded(input.WorkflowStep),
		Conversation:      bounded(strings.Join(input.UserMessages, "\n") + "\n" + input.Conversation),
		ToolSummary:       bounded(input.ToolSummary),
		RepositorySummary: bounded(input.RepositorySummary),
		PlanSummary:       bounded(input.PlanSummary),
		FailureReason:     bounded(input.FailureReason),
	}
}

func bounded(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= continuationFieldLimit {
		return value
	}
	return value[:continuationFieldLimit]
}
