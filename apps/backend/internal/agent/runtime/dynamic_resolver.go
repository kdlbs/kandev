package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

var ErrDynamicRoutingDisabled = errors.New("dynamic agent routing is disabled")

// ProfileExecution is the caller-facing result of resolving a logical
// profile. Concrete callers receive the same ID for both fields. Dynamic
// callers retain their logical ID while the resolver records the concrete
// candidate that owns the downstream launch.
type ProfileExecution struct {
	LogicalProfileID   string
	ExecutionProfileID string
	RouteSessionID     string
	Generation         int64
	ProfileVersion     int64
	Profile            *agentsettingsmodels.AgentProfile
	Decision           dynamic.RouteDecision
}

// ProfileExecutionResolver is the shared profile-kind boundary. Callers pass
// one profile ID and never need to branch on the dynamic family themselves.
// It intentionally returns a route decision rather than launching an agent;
// the conductor/lifecycle layer owns downstream ACP sessions.
type ProfileExecutionResolver struct {
	profiles        store.Repository
	dynamic         store.DynamicProfileRepository
	engine          *dynamic.Engine
	bindingResolver *dynamic.CredentialBindingResolver
	enabled         atomic.Bool
}

func NewProfileExecutionResolver(profiles store.Repository, engine *dynamic.Engine, enabled bool) *ProfileExecutionResolver {
	var dynamicRepo store.DynamicProfileRepository
	if repo, ok := profiles.(store.DynamicProfileRepository); ok {
		dynamicRepo = repo
	}
	resolver := &ProfileExecutionResolver{profiles: profiles, dynamic: dynamicRepo, engine: engine}
	resolver.enabled.Store(enabled)
	return resolver
}

func (r *ProfileExecutionResolver) SetEnabled(enabled bool) { r.enabled.Store(enabled) }

// SetCredentialBindingResolver supplies the installation-scoped fingerprint
// used to share provider health between concrete profiles that prove the same
// credential binding. A missing or incomplete descriptor remains isolated to
// the concrete profile through the resolver's conservative fallback.
func (r *ProfileExecutionResolver) SetCredentialBindingResolver(resolver *dynamic.CredentialBindingResolver) {
	r.bindingResolver = resolver
}

// NewConductor creates the lifecycle-facing conductor with the same engine,
// profile loader, and feature-gate state as this resolver.
func (r *ProfileExecutionResolver) NewConductor(
	downstream dynamic.DownstreamRuntime,
	options ...dynamic.ConductorOption,
) *dynamic.Conductor {
	if persistence := r.engine.ContinuationPersistence(); persistence != nil {
		options = append(options, dynamic.WithContinuationPersistence(persistence))
	}
	return dynamic.NewConductor(r.engine, r, downstream, options...)
}

// ValidateProfile performs the disabled-mode check without claiming a route
// generation or writing any durable state. Callers use it before creating a
// task session so a stored dynamic profile remains inert while the feature is
// disabled.
func (r *ProfileExecutionResolver) ValidateProfile(ctx context.Context, profileID string) error {
	if profileID == "" {
		// The ordinary launch path may resolve a workspace or workflow default
		// later. There is no profile family to gate until that resolution has
		// produced an ID.
		return nil
	}
	if r.profiles == nil {
		return errors.New("profile execution resolver has no profile store")
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("validate profile %s: %w", profileID, err)
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return fmt.Errorf("validate profile family %s: %w", profileID, err)
	}
	if agent.Name == agents.DynamicAgentID && !r.enabled.Load() {
		return ErrDynamicRoutingDisabled
	}
	return nil
}

// ResolveExecution preserves the small utility resolver contract for callers
// that do not carry a session identity.
func (r *ProfileExecutionResolver) ResolveExecution(ctx context.Context, profileID string) (*agentsettingsmodels.AgentProfile, string, error) {
	return r.ResolveExecutionForSession(ctx, "", profileID)
}

// LoadDynamicProfile implements dynamic.ProfileLoader for the conductor. It
// returns the same ordered, fail-closed candidate view used by ordinary
// profile resolution, without claiming a route generation.
func (r *ProfileExecutionResolver) LoadDynamicProfile(ctx context.Context, profileID string) (dynamic.Profile, error) {
	return r.loadDynamicProfile(ctx, profileID)
}

// ResolveExecutionForSession resolves one logical profile for a caller that
// has a durable session identity. Concrete profiles pass through unchanged;
// dynamic profiles claim a generation and return the selected concrete row.
func (r *ProfileExecutionResolver) ResolveExecutionForSession(ctx context.Context, sessionID, profileID string) (*agentsettingsmodels.AgentProfile, string, error) {
	execution, err := r.ResolveExecutionDetails(ctx, sessionID, profileID)
	if err != nil {
		return nil, "", err
	}
	return execution.Profile, execution.ExecutionProfileID, nil
}

// ResolveExecutionDetails returns the concrete profile and route metadata for
// a logical profile. Sessionless utility calls receive an isolated route ID.
func (r *ProfileExecutionResolver) ResolveExecutionDetails(ctx context.Context, sessionID, profileID string) (ProfileExecution, error) {
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile %s: %w", profileID, err)
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile family %s: %w", profileID, err)
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profile.ID, Profile: profile}, nil
	}
	if !r.enabled.Load() {
		return ProfileExecution{}, ErrDynamicRoutingDisabled
	}
	// Utility calls always get an isolated route state, even when the template
	// context belongs to an already-routed task session. Reusing that session
	// would consume or fence the task's durable generation.
	routeSessionID := "utility:" + uuid.NewString()
	decision, err := r.Resolve(ctx, routeSessionID, profileID, 0, "")
	if err != nil {
		return ProfileExecution{}, err
	}
	decision.RouteSessionID = routeSessionID
	return decision, nil
}

// ResolveExecutionAfterFailure applies a classified prompt failure and
// returns the next concrete execution profile for the same logical route.
func (r *ProfileExecutionResolver) ResolveExecutionAfterFailure(
	ctx context.Context,
	sessionID, profileID, currentExecutionProfileID string,
	expectedGeneration int64,
	failure *routingerr.Error,
) (ProfileExecution, error) {
	if r.profiles == nil || r.engine == nil {
		return ProfileExecution{}, errors.New("dynamic profile execution is not configured")
	}
	if err := r.ValidateProfile(ctx, profileID); err != nil {
		return ProfileExecution{}, err
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profile.ID, Profile: profile}, nil
	}
	if sessionID == "" {
		sessionID = "utility:" + uuid.NewString()
	}
	profileConfig, err := r.loadDynamicProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	decision, err := r.engine.ApplyFailureContext(ctx, sessionID, profileConfig, expectedGeneration, currentExecutionProfileID, failure)
	if err != nil {
		return ProfileExecution{}, err
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, decision.ExecutionProfileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve execution profile %s: %w", decision.ExecutionProfileID, err)
	}
	return ProfileExecution{
		LogicalProfileID: profileID, ExecutionProfileID: decision.ExecutionProfileID,
		RouteSessionID: sessionID, Generation: decision.Generation,
		ProfileVersion: decision.ProfileVersion, Profile: concrete, Decision: decision,
	}, nil
}

// ResolveExisting returns the persisted concrete execution for a logical
// session without advancing its route generation. It is used by resume paths
// after a restart, where selecting again would either fence a valid session
// or silently move it to another candidate.
func (r *ProfileExecutionResolver) ResolveExisting(
	ctx context.Context,
	sessionID, profileID, executionProfileID string,
	generation, profileVersion int64,
	reason string,
) (ProfileExecution, error) {
	if err := r.ValidateProfile(ctx, profileID); err != nil {
		return ProfileExecution{}, err
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profileID, Profile: profile}, nil
	}
	if executionProfileID == "" || generation <= 0 {
		return ProfileExecution{}, errors.New("dynamic session has no persisted execution profile")
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, executionProfileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve existing execution profile %s: %w", executionProfileID, err)
	}
	if concrete == nil || concrete.DeletedAt != nil || !concrete.Enabled {
		return ProfileExecution{}, fmt.Errorf("existing execution profile %s is unavailable", executionProfileID)
	}
	return ProfileExecution{
		LogicalProfileID: profileID, ExecutionProfileID: executionProfileID,
		Generation: generation, ProfileVersion: profileVersion, Profile: concrete,
		Decision: dynamic.RouteDecision{
			SessionID: sessionID, LogicalProfileID: profileID,
			ExecutionProfileID: executionProfileID, Generation: generation,
			ProfileVersion: profileVersion, Reason: reason,
		},
	}, nil
}

func (r *ProfileExecutionResolver) Resolve(ctx context.Context, sessionID, profileID string, expectedGeneration int64, excludeProfileID string) (ProfileExecution, error) {
	return r.resolve(ctx, sessionID, profileID, expectedGeneration, excludeProfileID, "")
}

// ResolveWithPreference keeps the current concrete candidate for an explicit
// retry when it remains eligible. Try-next callers continue to use Resolve and
// pass the current candidate as the one-time exclusion.
func (r *ProfileExecutionResolver) ResolveWithPreference(
	ctx context.Context,
	sessionID, profileID string,
	expectedGeneration int64,
	excludeProfileID, preferredProfileID string,
) (ProfileExecution, error) {
	return r.resolve(ctx, sessionID, profileID, expectedGeneration, excludeProfileID, preferredProfileID)
}

// ResolveRouteAction applies a manual route action to the durable route
// state. Retry resumes the current generation, even when its policy deadline
// has not elapsed; try-next claims a new generation and excludes the current
// candidate. The caller can then launch the returned concrete profile through
// the normal conductor path.
func (r *ProfileExecutionResolver) ResolveRouteAction(
	ctx context.Context,
	sessionID, profileID, currentExecutionProfileID string,
	expectedGeneration int64,
	action string,
) (ProfileExecution, error) {
	if r.profiles == nil {
		return ProfileExecution{}, errors.New("profile execution resolver has no profile store")
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile %s: %w", profileID, err)
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile family %s: %w", profileID, err)
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profile.ID, Profile: profile}, nil
	}
	if !r.enabled.Load() {
		return ProfileExecution{}, ErrDynamicRoutingDisabled
	}
	if r.dynamic == nil || r.engine == nil {
		return ProfileExecution{}, errors.New("dynamic profile execution is not configured")
	}
	if sessionID == "" {
		return ProfileExecution{}, errors.New("dynamic route action requires a session")
	}

	switch action {
	case "retry":
		return r.resolveRetryRouteAction(ctx, sessionID, profileID, currentExecutionProfileID, expectedGeneration)
	case "try_next", "skip":
		return r.resolveSkipRouteAction(ctx, sessionID, profileID, currentExecutionProfileID, expectedGeneration)
	case "cancel_wait", "stop":
		return r.resolveCancelRouteAction(ctx, sessionID, profileID, expectedGeneration, action)
	default:
		return ProfileExecution{}, fmt.Errorf("unsupported dynamic route action %q", action)
	}
}

func (r *ProfileExecutionResolver) resolveRetryRouteAction(
	ctx context.Context,
	sessionID, profileID, currentExecutionProfileID string,
	expectedGeneration int64,
) (ProfileExecution, error) {
	state, exists, err := r.engine.LoadState(ctx, sessionID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if exists && state.Generation != expectedGeneration {
		return ProfileExecution{}, dynamic.ErrStaleGeneration
	}
	if exists {
		decision, resumeErr := r.engine.ResumePendingNow(ctx, sessionID, expectedGeneration)
		if resumeErr == nil {
			return r.executionFromDecision(ctx, profileID, sessionID, decision)
		}
		if !errors.Is(resumeErr, dynamic.ErrRecoveryPending) && !errors.Is(resumeErr, dynamic.ErrRouteStateNotFound) {
			return ProfileExecution{}, resumeErr
		}
	}
	return r.resolve(ctx, sessionID, profileID, expectedGeneration, "", currentExecutionProfileID)
}

func (r *ProfileExecutionResolver) resolveSkipRouteAction(
	ctx context.Context,
	sessionID, profileID, currentExecutionProfileID string,
	expectedGeneration int64,
) (ProfileExecution, error) {
	profileConfig, err := r.loadDynamicProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if currentExecutionProfileID == "" {
		state, exists, stateErr := r.engine.LoadState(ctx, sessionID)
		if stateErr != nil {
			return ProfileExecution{}, stateErr
		}
		if exists {
			currentExecutionProfileID = state.ExecutionProfileID
		}
	}
	decision, err := r.engine.SelectContextWithReason(
		ctx, sessionID, profileConfig, expectedGeneration, currentExecutionProfileID, "manual_skip",
	)
	if err != nil {
		return ProfileExecution{}, err
	}
	return r.executionFromDecision(ctx, profileID, sessionID, decision)
}

func (r *ProfileExecutionResolver) resolveCancelRouteAction(
	ctx context.Context,
	sessionID, profileID string,
	expectedGeneration int64,
	action string,
) (ProfileExecution, error) {
	reason := "manual_cancel_wait"
	if action == "stop" {
		reason = "manual_stop"
	}
	decision, err := r.engine.CancelPending(ctx, sessionID, expectedGeneration, reason)
	if err != nil {
		return ProfileExecution{}, err
	}
	return r.executionFromDecision(ctx, profileID, sessionID, decision)
}

// ResumePendingRoute advances a due durable policy wait/retry without
// selecting another generation. It is used by the orchestrator's recovery
// scheduler after the persisted deadline has elapsed.
func (r *ProfileExecutionResolver) ResumePendingRoute(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
) (ProfileExecution, error) {
	if r.engine == nil || r.profiles == nil {
		return ProfileExecution{}, errors.New("dynamic profile execution is not configured")
	}
	state, exists, err := r.engine.LoadState(ctx, sessionID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if !exists {
		return ProfileExecution{}, dynamic.ErrRouteStateNotFound
	}
	decision, err := r.engine.ResumePending(ctx, sessionID, expectedGeneration)
	if err != nil {
		return ProfileExecution{}, err
	}
	return r.executionFromDecision(ctx, state.LogicalProfileID, sessionID, decision)
}

func (r *ProfileExecutionResolver) resolve(
	ctx context.Context,
	sessionID, profileID string,
	expectedGeneration int64,
	excludeProfileID, preferredProfileID string,
) (ProfileExecution, error) {
	if r.profiles == nil {
		return ProfileExecution{}, errors.New("profile execution resolver has no profile store")
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile %s: %w", profileID, err)
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve profile family %s: %w", profileID, err)
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{
			LogicalProfileID: profileID, ExecutionProfileID: profileID, Profile: profile,
		}, nil
	}
	if !r.enabled.Load() {
		return ProfileExecution{}, ErrDynamicRoutingDisabled
	}
	if r.dynamic == nil || r.engine == nil {
		return ProfileExecution{}, errors.New("dynamic profile execution is not configured")
	}
	profileConfig, err := r.loadDynamicProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	decision, err := r.engine.SelectContextWithPreference(
		ctx, sessionID, profileConfig, expectedGeneration, excludeProfileID, preferredProfileID,
	)
	if err != nil {
		return ProfileExecution{}, err
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, decision.ExecutionProfileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve execution profile %s: %w", decision.ExecutionProfileID, err)
	}
	return ProfileExecution{
		LogicalProfileID: profileID, ExecutionProfileID: decision.ExecutionProfileID,
		RouteSessionID: sessionID,
		Generation:     decision.Generation, ProfileVersion: decision.ProfileVersion,
		Profile:  concrete,
		Decision: decision,
	}, nil
}

func (r *ProfileExecutionResolver) executionFromDecision(
	ctx context.Context,
	profileID, sessionID string,
	decision dynamic.RouteDecision,
) (ProfileExecution, error) {
	concrete, err := r.profiles.GetAgentProfile(ctx, decision.ExecutionProfileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve execution profile %s: %w", decision.ExecutionProfileID, err)
	}
	if concrete == nil || concrete.DeletedAt != nil || !concrete.Enabled {
		return ProfileExecution{}, fmt.Errorf("execution profile %s is unavailable", decision.ExecutionProfileID)
	}
	return ProfileExecution{
		LogicalProfileID: profileID, ExecutionProfileID: decision.ExecutionProfileID,
		RouteSessionID: sessionID, Generation: decision.Generation,
		ProfileVersion: decision.ProfileVersion, Profile: concrete, Decision: decision,
	}, nil
}

func (r *ProfileExecutionResolver) loadDynamicProfile(ctx context.Context, profileID string) (dynamic.Profile, error) {
	if !r.enabled.Load() {
		return dynamic.Profile{}, ErrDynamicRoutingDisabled
	}
	if r.dynamic == nil {
		return dynamic.Profile{}, errors.New("dynamic profile execution is not configured")
	}
	config, routes, err := r.dynamic.GetDynamicAgentProfile(ctx, profileID)
	if err != nil {
		return dynamic.Profile{}, fmt.Errorf("load dynamic profile %s: %w", profileID, err)
	}
	profile := dynamic.Profile{
		ID: profileID, Version: config.Version,
		Candidates: make([]dynamic.Candidate, 0, len(routes)),
	}
	for _, route := range routes {
		candidate := dynamic.Candidate{
			ID: route.ExecutionProfileID, Enabled: route.Enabled,
			BindingKey: dynamic.ResourceKey(dynamic.ScopeProfile, route.ExecutionProfileID),
		}
		if route.RulesJSON != "" {
			policy, legacyRules, policyErr := decodeDynamicRoutePolicy(route.RulesJSON)
			if policyErr != nil {
				return dynamic.Profile{}, fmt.Errorf("decode dynamic route %s: %w", route.ExecutionProfileID, policyErr)
			}
			candidate.Policies = policy
			candidate.Rules = legacyRules
		}
		concrete, profileErr := r.profiles.GetAgentProfile(ctx, route.ExecutionProfileID)
		switch {
		case profileErr != nil:
			if errors.Is(profileErr, sql.ErrNoRows) || errors.Is(profileErr, store.ErrAgentProfileDeleted) {
				candidate.Enabled = false
			} else {
				return dynamic.Profile{}, fmt.Errorf("load dynamic candidate %s: %w", route.ExecutionProfileID, profileErr)
			}
		case concrete == nil || concrete.DeletedAt != nil || !concrete.Enabled:
			candidate.Enabled = false
		case r.bindingResolver != nil:
			binding := profileCredentialBindingDescriptor(concrete)
			candidate.BindingKey = dynamic.ResourceKey(
				dynamic.ScopeCredential,
				r.bindingResolver.Resolve(binding, route.ExecutionProfileID),
			)
		}
		profile.Candidates = append(profile.Candidates, candidate)
	}
	return profile, nil
}

func decodeDynamicRoutePolicy(raw string) (routingpolicy.Document, map[string]dynamic.Action, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return routingpolicy.Document{}, nil, err
	}
	if _, hasVersion := fields["version"]; hasVersion {
		var document routingpolicy.Document
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			return routingpolicy.Document{}, nil, err
		}
		if err := routingpolicy.ValidateDocument(document); err != nil {
			return routingpolicy.Document{}, nil, err
		}
		return document, nil, nil
	}
	var legacy map[string]dynamic.Action
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return routingpolicy.Document{}, nil, err
	}
	document := routingpolicy.DefaultDocument()
	classActions := make(map[routingerr.Class]dynamic.Action)
	for key, action := range legacy {
		if key == "on_provider_error" {
			document.Transient = legacyActionPolicy(action)
			document.Hard = legacyActionPolicy(action)
			continue
		}
		class := routingerr.ClassForCode(routingerr.Code(key))
		if class != routingerr.ClassTransient && class != routingerr.ClassHard {
			return routingpolicy.Document{}, nil, fmt.Errorf("legacy rule %q is not a provider error code", key)
		}
		if previous, ok := classActions[class]; ok && previous != action {
			return routingpolicy.Document{}, nil, fmt.Errorf("legacy rules conflict for %s errors", class)
		}
		classActions[class] = action
		if class == routingerr.ClassTransient {
			document.Transient = legacyActionPolicy(action)
		} else {
			document.Hard = legacyActionPolicy(action)
		}
	}
	if err := routingpolicy.ValidateDocument(document); err != nil {
		return routingpolicy.Document{}, nil, err
	}
	return document, legacy, nil
}

func legacyActionPolicy(action dynamic.Action) routingpolicy.Policy {
	policy := routingpolicy.DefaultPolicy()
	switch action {
	case dynamic.ActionRetrySame:
		policy.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 5}
		policy.OnExhausted = routingpolicy.OutcomeStop
	case dynamic.ActionStop:
		policy.OnExhausted = routingpolicy.OutcomeStop
	}
	return policy
}

func profileCredentialBindingDescriptor(profile *agentsettingsmodels.AgentProfile) dynamic.CredentialBindingDescriptor {
	if profile == nil {
		return dynamic.CredentialBindingDescriptor{}
	}
	descriptor := dynamic.CredentialBindingDescriptor{
		Version:              1,
		AgentFamilyID:        profile.AgentID,
		AuthenticationMethod: strings.TrimSpace(profile.BillingType),
		ExecutorNamespace:    "local",
		AuthorizationScope:   "agent_runtime",
	}
	secretIDs := make([]string, 0, len(profile.EnvVars))
	for _, envVar := range profile.EnvVars {
		if strings.TrimSpace(envVar.SecretID) != "" {
			secretIDs = append(secretIDs, strings.TrimSpace(envVar.SecretID))
		}
	}
	if len(secretIDs) > 0 {
		sort.Strings(secretIDs)
		descriptor.CredentialSourceKind = "profile_secret"
		descriptor.CredentialLocator = strings.Join(secretIDs, ",")
	} else if descriptor.AuthenticationMethod != "" {
		descriptor.CredentialSourceKind = "agent_credentials"
		descriptor.CredentialLocator = profile.AgentID + ":" + descriptor.AuthenticationMethod
	}
	return descriptor
}
