package dynamic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

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

// DownstreamExecutionLoader restores the concrete execution identity after a
// process restart. Implementations should read the durable task-session row,
// not a process-local cache.
type DownstreamExecutionLoader interface {
	LoadExecution(context.Context, string) (DownstreamExecution, bool, error)
}

type ProfileLoader interface {
	LoadDynamicProfile(context.Context, string) (Profile, error)
}

type ConductorOption func(*Conductor)

func WithContinuationBuilder(builder func(context.Context, ContinuationInput) (Continuation, error)) ConductorOption {
	return func(conductor *Conductor) { conductor.continuationBuilder = builder }
}

func WithContinuationPersistence(persistence ContinuationPersistence) ConductorOption {
	return func(conductor *Conductor) { conductor.continuationPersistence = persistence }
}

// Conductor owns the logical session while concrete runtimes own downstream
// process and ACP identities.
type Conductor struct {
	mu                      sync.Mutex
	engine                  *Engine
	profiles                ProfileLoader
	downstream              DownstreamRuntime
	continuationBuilder     func(context.Context, ContinuationInput) (Continuation, error)
	continuationPersistence ContinuationPersistence
	active                  map[string]DownstreamExecution
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
	PriorACPSession    string
	Continuation       ContinuationInput
}

// ConductorSelectedLaunch hands the conductor a route that was already
// claimed by the shared resolver. It is used by lifecycle adapters that must
// preserve the existing session generation while still delegating classified
// pre-result fallback to the conductor.
type ConductorSelectedLaunch struct {
	SessionID            string
	LogicalProfileID     string
	Decision             RouteDecision
	Prompt               string
	PriorACPSession      string
	Continuation         ContinuationInput
	PrebuiltContinuation *Continuation
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
	var continuation Continuation
	if request.PrebuiltContinuation != nil {
		continuation = *request.PrebuiltContinuation
	} else {
		var err error
		continuation, err = c.buildContinuation(ctx, request.Continuation)
		if err != nil {
			return ConductorResult{}, err
		}
	}
	execution, decision, err := c.launchWithFallback(
		ctx,
		profile,
		request.Decision,
		ConductorLaunch{
			SessionID:        request.SessionID,
			LogicalProfileID: request.LogicalProfileID,
			Prompt:           request.Prompt,
			PriorACPSession:  request.PriorACPSession,
			Continuation:     request.Continuation,
		},
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

// BuildContinuation creates the bounded provider-neutral handoff package used
// by a successor launch. Callers that already classified a failed turn build
// it before advancing the route, then persist it against the successor
// generation before launching that generation.
func (c *Conductor) BuildContinuation(ctx context.Context, input ContinuationInput) (Continuation, error) {
	return c.buildContinuation(ctx, input)
}

// PersistContinuation records a handoff package only after its successor
// generation has been claimed. The repository implementation fences the
// write to that generation.
func (c *Conductor) PersistContinuation(ctx context.Context, decision RouteDecision, continuation Continuation) error {
	return c.persistContinuation(ctx, decision, continuation)
}

func (c *Conductor) launchWithFallback(
	ctx context.Context,
	profile Profile,
	decision RouteDecision,
	request ConductorLaunch,
	continuation Continuation,
) (DownstreamExecution, RouteDecision, error) {
	maxAttempts := len(profile.Candidates)
	if maxAttempts == 0 {
		return DownstreamExecution{}, decision, ErrNoEligibleCandidate
	}
	current := decision
	currentContinuation := continuation
	for attempt := 0; attempt < maxAttempts; attempt++ {
		execution, err := c.downstream.Launch(ctx, DownstreamLaunch{
			ExecutionProfileID: current.ExecutionProfileID,
			Decision:           current,
			Prompt:             ContinuationPrompt(request.Prompt, currentContinuation),
			PriorACPSession:    priorACPSession(request.PriorACPSession, decision, current),
			Continuation:       currentContinuation,
		})
		if err == nil {
			c.engine.ReleaseProbe(current, true)
			return execution, current, nil
		}
		// Every probe attempt has a terminal launch result. Release it before
		// applying policy so unclassified or post-start failures do not leave a
		// half-open circuit lease stranded until its timeout.
		c.engine.ReleaseProbe(current, false)
		next, shouldFallback, fallbackErr := c.nextAfterLaunchFailure(
			ctx, profile, current, request.SessionID, err,
		)
		if fallbackErr != nil {
			if errors.Is(fallbackErr, ErrNoEligibleCandidate) {
				return DownstreamExecution{}, current, err
			}
			return DownstreamExecution{}, current, fallbackErr
		}
		if !shouldFallback || attempt+1 >= maxAttempts {
			return DownstreamExecution{}, current, err
		}
		classified := classifiedLaunchFailure(err)
		if classified != nil {
			currentContinuation = continuationWithFailure(currentContinuation, classified.Error())
		}
		if persistErr := c.persistContinuation(ctx, next, currentContinuation); persistErr != nil {
			return DownstreamExecution{}, current, persistErr
		}
		current = next
	}
	return DownstreamExecution{}, current, ErrNoEligibleCandidate
}

func priorACPSession(
	persisted string,
	initial RouteDecision,
	current RouteDecision,
) string {
	if current.ExecutionProfileID != initial.ExecutionProfileID {
		return ""
	}
	return persisted
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
	if classified.Phase != "" && classified.Phase != routingerr.PhaseAuthCheck &&
		classified.Phase != routingerr.PhaseProcessStart && classified.Phase != routingerr.PhaseSessionInit {
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

func classifiedLaunchFailure(err error) *routingerr.Error {
	var classified *routingerr.Error
	if errors.As(err, &classified) {
		return classified
	}
	return nil
}

func continuationWithFailure(current Continuation, reason string) Continuation {
	current.FailureReason = bounded(reason)
	return current
}

func (c *Conductor) persistContinuation(ctx context.Context, decision RouteDecision, continuation Continuation) error {
	if c.continuationPersistence == nil {
		return nil
	}
	return c.continuationPersistence.SaveRouteContinuation(ctx, ContinuationRecord{
		SessionID: decision.SessionID, Generation: decision.Generation,
		Continuation: continuation, UpdatedAt: time.Now().UTC(),
	})
}

// Resume reuses a downstream ACP session only when its concrete profile owns
// the current logical route. A caller that changes candidates must call Launch
// with the new generation, which always creates a fresh downstream session.
func (c *Conductor) Resume(ctx context.Context, sessionID, prompt string) error {
	c.mu.Lock()
	execution, ok := c.active[sessionID]
	c.mu.Unlock()
	if !ok {
		loader, supportsLoading := c.downstream.(DownstreamExecutionLoader)
		if !supportsLoading {
			return errors.New("dynamic conductor has no active downstream execution")
		}
		loaded, found, err := loader.LoadExecution(ctx, sessionID)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("dynamic conductor has no active downstream execution")
		}
		execution = loaded
		c.mu.Lock()
		c.active[sessionID] = execution
		c.mu.Unlock()
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

// ContinuationPrompt renders a bounded, provider-neutral handoff. The
// original prompt remains first so providers that do not understand the
// optional package still receive the user's request.
func ContinuationPrompt(prompt string, continuation Continuation) string {
	fields := make([]string, 0, 7)
	if continuation.TaskDescription != "" {
		fields = append(fields, "Task: "+continuation.TaskDescription)
	}
	if continuation.WorkflowStep != "" {
		fields = append(fields, "Workflow step: "+continuation.WorkflowStep)
	}
	if continuation.Conversation != "" {
		fields = append(fields, "Durable conversation: "+continuation.Conversation)
	}
	if continuation.ToolSummary != "" {
		fields = append(fields, "Tool summary: "+continuation.ToolSummary)
	}
	if continuation.RepositorySummary != "" {
		fields = append(fields, "Repository summary: "+continuation.RepositorySummary)
	}
	if continuation.PlanSummary != "" {
		fields = append(fields, "Plan summary: "+continuation.PlanSummary)
	}
	if continuation.FailureReason != "" {
		fields = append(fields, "Predecessor failure: "+continuation.FailureReason)
	}
	if len(fields) == 0 {
		return prompt
	}
	return strings.TrimSpace(prompt) + "\n\n[Kandev continuation package]\n" + strings.Join(fields, "\n") +
		"\nVerify durable state before repeating any uncertain action."
}

func bounded(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= continuationFieldLimit {
		return value
	}
	return value[:continuationFieldLimit]
}
