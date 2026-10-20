package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/hostutility"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/review"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type reviewRouteSession struct {
	run func(context.Context, string, string) (*utility.PromptResponse, error)
}

func (s *reviewRouteSession) ExecuteInferencePrompt(ctx context.Context, session, agent, model, prompt string) (*utility.PromptResponse, error) {
	return s.run(ctx, agent, prompt)
}
func (s *reviewRouteSession) ExecuteInferenceProfilePrompt(ctx context.Context, session, id, prompt string) (*utility.PromptResponse, error) {
	return s.run(ctx, id, prompt)
}
func nativeReviewRoute(t *testing.T, retry bool, options ...dynamic.EngineOption) (*agentruntime.ProfileExecutionResolver, *dynamic.Engine, settingsstore.Repository, *taskrepo.Repository) {
	t.Helper()
	_, profiles, tasks := openTasklessDynamicRepo(t)
	ctx := context.Background()
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "claude-row", Name: "claude-acp"}))
	candidate, err := profiles.GetAgentProfile(ctx, "concrete-profile")
	require.NoError(t, err)
	candidate.AgentID = "claude-row"
	require.NoError(t, profiles.UpdateAgentProfile(ctx, candidate))
	require.NoError(t, profiles.CreateAgentProfile(ctx, &settings.AgentProfile{ID: "candidate-b", AgentID: "claude-row", Name: "Second", Model: "second", Enabled: true}))
	repo := profiles.(settingsstore.DynamicProfileRepository)
	policy := routingpolicy.DefaultDocument()
	if retry {
		policy.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 1}
	}
	raw, err := json.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateDynamicAgentProfile(ctx, &settings.DynamicAgentProfile{ProfileID: "profile-dynamic", Version: 2}, 1, []settings.DynamicAgentRoute{
		{DynamicProfileID: "profile-dynamic", ExecutionProfileID: "concrete-profile", Enabled: true, RulesJSON: string(raw)},
		{DynamicProfileID: "profile-dynamic", ExecutionProfileID: "candidate-b", Enabled: true, Position: 1},
	}))
	engine := dynamic.NewEngine(options...)
	return agentruntime.NewProfileExecutionResolver(profiles, engine, true), engine, profiles, tasks
}
func nativeReviewIdentity(t *testing.T, resolver *agentruntime.ProfileExecutionResolver, profiles settingsstore.Repository) review.ReviewerIdentity {
	t.Helper()
	identity, err := review.NewResolver(reviewProfileLookup{profiles: profiles, execution: resolver}, nil, nil).Resolve(context.Background(), "profile-dynamic")
	require.NoError(t, err)
	require.NotNil(t, identity.Route)
	require.Equal(t, "claude-acp", identity.Route.AgentID)
	return identity
}

// @covers AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.1
func TestNativeReviewRouteRetainedAcrossBatchesAndStaleCAS(t *testing.T) {
	resolver, engine, profiles, _ := nativeReviewRoute(t, false)
	identity := nativeReviewIdentity(t, resolver, profiles)
	original := *identity.Route
	var calls []string
	session := &reviewRouteSession{run: func(ctx context.Context, id, prompt string) (*utility.PromptResponse, error) {
		calls = append(calls, id)
		got := agentruntime.InferenceIdentityFromContext(ctx)
		require.Equal(t, "profile-dynamic", got.LogicalProfileID)
		require.Equal(t, id, got.ExecutionProfileID)
		require.Equal(t, "run", got.ReviewRunID)
		if id == "concrete-profile" {
			return nil, errors.New("rate limit exceeded")
		}
		return &utility.PromptResponse{Success: true, Response: `{"findings":[]}`, Model: "second"}, nil
	}}
	inference := reviewInference{session: session, profiles: resolver}
	ctx := agentruntime.WithInferenceIdentity(context.Background(), agentruntime.InferenceIdentity{LogicalProfileID: "profile-dynamic", ReviewRunID: "run", OwnerKind: "review"})
	for range 2 {
		_, err := inference.Run(ctx, identity, "session", "batch")
		require.NoError(t, err)
	}
	require.Equal(t, []string{"concrete-profile", "candidate-b", "candidate-b"}, calls)
	require.Equal(t, original.SessionID, identity.Route.SessionID)
	require.Equal(t, original.Generation+1, identity.Route.Generation)
	state, ok := engine.State(original.SessionID)
	require.True(t, ok)
	require.Equal(t, "candidate-b", state.ExecutionProfileID)
	failure := routingerr.Classify(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: "claude-acp", Stderr: "rate limit exceeded"})
	_, err := resolver.ResolveExecutionAfterFailure(ctx, original.SessionID, "profile-dynamic", "concrete-profile", original.Generation, failure)
	require.ErrorIs(t, err, dynamic.ErrStaleGeneration)
	after, _ := engine.State(original.SessionID)
	require.Equal(t, state, after)
	second := nativeReviewIdentity(t, resolver, profiles)
	require.NotEqual(t, original.SessionID, second.Route.SessionID)
}
func TestNativeReviewAdapterStopsPartialDeadlineCapacityAndParent(t *testing.T) {
	for _, kind := range []string{"partial", "deadline", "capacity", "parent"} {
		t.Run(kind, func(t *testing.T) {
			resolver, engine, profiles, _ := nativeReviewRoute(t, false)
			identity := nativeReviewIdentity(t, resolver, profiles)
			initial, _ := engine.State(identity.Route.SessionID)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := errors.New("rate limit exceeded")
			calls := 0
			session := &reviewRouteSession{run: func(context.Context, string, string) (*utility.PromptResponse, error) {
				calls++
				switch kind {
				case "partial":
					return &utility.PromptResponse{Response: "visible partial"}, cause
				case "deadline":
					return nil, fmt.Errorf("rate limit exceeded: %w", context.DeadlineExceeded)
				case "capacity":
					return nil, &agentruntime.InferenceCapacityError{Reason: "full", Cause: cause}
				default:
					cancel()
					return nil, context.Canceled
				}
			}}
			_, err := (reviewInference{session: session, host: &hostutility.Manager{}, profiles: resolver}).Run(ctx, identity, "session", "prompt")
			require.Error(t, err)
			require.Equal(t, 1, calls)
			after, _ := engine.State(identity.Route.SessionID)
			require.Equal(t, initial, after)
			if kind == "partial" {
				require.ErrorIs(t, err, cause)
			}
			if kind == "deadline" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			if kind == "capacity" {
				require.True(t, agentruntime.IsInferenceCapacityError(err))
			}
			if kind == "parent" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

type reviewRouteChanges struct{}

func (reviewRouteChanges) UncommittedFiles(context.Context, string) (map[string]any, error) {
	files := map[string]any{}
	for _, path := range []string{"a.go", "b.go"} {
		files[path] = map[string]any{"path": path, "status": "modified", "diff": "@@ -1 +1,2 @@\n old\n+new\n"}
	}
	return files, nil
}
func (reviewRouteChanges) CommittedFiles(context.Context, string) (map[string]any, error) {
	return nil, nil
}

type reviewRoutePrompts struct{}

func (reviewRoutePrompts) Build(_ context.Context, files []review.ChangedFile, _ review.PromptContext) (string, error) {
	return files[0].Path, nil
}
func seedNativeReviewOwner(t *testing.T, repo *taskrepo.Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{ID: "task", Title: "Review"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "session", TaskID: "task"}))
}
func TestNativeReviewRunnerUsesVerifiedIdentityAndSameRoute(t *testing.T) {
	resolver, _, profiles, tasks := nativeReviewRoute(t, false)
	seedNativeReviewOwner(t, tasks)
	var calls []string
	session := &reviewRouteSession{run: func(ctx context.Context, id, prompt string) (*utility.PromptResponse, error) {
		calls = append(calls, id)
		origin := agentruntime.InferenceIdentityFromContext(ctx)
		require.Equal(t, "task", origin.TaskID)
		require.Equal(t, "session", origin.SessionID)
		require.Equal(t, "profile-dynamic", origin.LogicalProfileID)
		require.Equal(t, id, origin.ExecutionProfileID)
		run, err := tasks.GetTaskReviewRun(ctx, origin.ReviewRunID)
		require.NoError(t, err)
		require.Equal(t, taskmodels.ReviewRunRunning, run.Status)
		if id == "concrete-profile" {
			return nil, errors.New("rate limit exceeded")
		}
		return &utility.PromptResponse{Success: true, Response: `{"summary":"reviewed","findings":[]}`}, nil
	}}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	runner := review.NewRunner(review.RunnerDeps{
		Store: taskservice.NewReviewService(tasks, nil, log), Resolver: review.NewResolver(reviewProfileLookup{profiles: profiles, execution: resolver}, nil, nil),
		Inference: reviewInference{session: session, profiles: resolver}, InferenceContext: reviewInferenceContext(tasks), Changes: reviewRouteChanges{}, Prompts: reviewRoutePrompts{}, Logger: log, BudgetBytes: 35,
	})
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)
	run, err := runner.Run(context.Background(), review.RunRequest{TaskID: "task", SessionID: "session", AgentProfileID: "profile-dynamic", Trigger: taskmodels.ReviewTriggerManual})
	require.NoError(t, err)
	stored, err := tasks.GetTaskReviewRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, taskmodels.ReviewRunCompleted, stored.Status)
	require.Equal(t, []string{"concrete-profile", "candidate-b", "candidate-b"}, calls)
	runs, err := tasks.ListTaskReviewRuns(context.Background(), "task", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
}
func TestNativeReviewContextRejectsUnverifiedRelationships(t *testing.T) {
	_, _, _, repo := nativeReviewRoute(t, false)
	seedNativeReviewOwner(t, repo)
	ctx := context.Background()
	require.NoError(t, repo.CreateTaskReviewRun(ctx, &taskmodels.TaskReviewRun{ID: "run", TaskID: "task", SessionID: "session", Status: taskmodels.ReviewRunRunning}))
	verify := reviewInferenceContext(repo)
	for _, req := range []review.RunRequest{{TaskID: "task", SessionID: "session"}, {TaskID: "other", SessionID: "session"}, {TaskID: "task", SessionID: "missing"}} {
		got, err := verify(ctx, req, "run", review.ReviewerIdentity{ProfileID: "logical"})
		if req.TaskID == "task" && req.SessionID == "session" {
			require.NoError(t, err)
			require.Equal(t, agentruntime.InferenceIdentity{TaskID: "task", SessionID: "session", LogicalProfileID: "logical", ReviewRunID: "run", OwnerKind: "review"}, agentruntime.InferenceIdentityFromContext(got))
		} else {
			require.Error(t, err)
			require.Equal(t, agentruntime.InferenceIdentity{}, agentruntime.InferenceIdentityFromContext(got))
		}
	}
	_, err := verify(ctx, review.RunRequest{TaskID: "task", SessionID: "session"}, "missing", review.ReviewerIdentity{})
	require.Error(t, err)
}

func TestNativeReviewPendingRouteCarriesExactResumeIdentity(t *testing.T) {
	now := time.Now()
	resolver, _, profiles, _ := nativeReviewRoute(t, true, dynamic.WithClock(func() time.Time { return now }))
	identity := nativeReviewIdentity(t, resolver, profiles)
	route := identity.Route
	failure := routingerr.Classify(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: "claude-acp", Stderr: "rate limit exceeded"})
	next, err := resolver.ResolveExecutionAfterFailure(context.Background(), route.SessionID, "profile-dynamic", route.ExecutionProfileID, route.Generation, failure)
	require.ErrorIs(t, err, dynamic.ErrRecoveryPending)
	require.Equal(t, route.SessionID, next.RouteSessionID)
	require.Equal(t, route.Generation, next.Generation)
	require.Equal(t, "concrete-profile", next.ExecutionProfileID)
	require.NotNil(t, next.Profile)
	require.NotNil(t, next.Decision.Deadline)
	now = now.Add(time.Second)
	resumed, err := resolver.ResumePendingRoute(context.Background(), next.RouteSessionID, next.Generation)
	require.NoError(t, err)
	require.Equal(t, route.Generation, resumed.Generation)
	require.Equal(t, "concrete-profile", resumed.ExecutionProfileID)
	successor, err := resolver.ResolveExecutionAfterFailure(context.Background(), route.SessionID, "profile-dynamic", resumed.ExecutionProfileID, resumed.Generation, failure)
	require.NoError(t, err)
	require.Equal(t, "candidate-b", successor.ExecutionProfileID)
	require.Equal(t, route.Generation+1, successor.Generation)
}

func TestNativeReviewAdapterHostFallbackReachable(t *testing.T) {
	cause := errors.New("session execution unavailable")
	session := &reviewRouteSession{run: func(context.Context, string, string) (*utility.PromptResponse, error) { return nil, cause }}
	_, err := (reviewInference{session: session, host: &hostutility.Manager{}}).Run(context.Background(), review.ReviewerIdentity{ProfileID: "concrete"}, "session", "prompt")
	require.EqualError(t, err, "utility profile resolver is not configured")
	require.NotErrorIs(t, err, cause)
}

func TestNativeReviewClassifiedSessionErrorUsesPolicyBeforeHostFallback(t *testing.T) {
	for _, channel := range []string{"transport", "response"} {
		t.Run(channel, func(t *testing.T) {
			resolver, _, profiles, _ := nativeReviewRoute(t, false)
			identity := nativeReviewIdentity(t, resolver, profiles)
			var calls []string
			session := &reviewRouteSession{run: func(_ context.Context, id, prompt string) (*utility.PromptResponse, error) {
				calls = append(calls, id)
				if id == "concrete-profile" {
					if channel == "response" {
						return &utility.PromptResponse{Error: "rate limit exceeded"}, nil
					}
					return nil, errors.New("rate limit exceeded")
				}
				return &utility.PromptResponse{Success: true, Response: `{"findings":[]}`}, nil
			}}
			result, err := (reviewInference{session: session, host: &hostutility.Manager{}, profiles: resolver}).Run(context.Background(), identity, "session", "prompt")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, []string{"concrete-profile", "candidate-b"}, calls)
			require.Equal(t, "candidate-b", identity.Route.ExecutionProfileID)
		})
	}
}

func TestNativeReviewHostPolicyUsesSuccessorProviderIdentity(t *testing.T) {
	resolver, _, profiles, _ := nativeReviewRoute(t, false)
	ctx := context.Background()
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "codex-row", Name: "codex-acp"}))
	candidate, err := profiles.GetAgentProfile(ctx, "candidate-b")
	require.NoError(t, err)
	candidate.AgentID = "codex-row"
	require.NoError(t, profiles.UpdateAgentProfile(ctx, candidate))
	identity := nativeReviewIdentity(t, resolver, profiles)
	first := errors.New("rate limit exceeded")
	successor := errors.New("rate_limit_exceeded")
	var calls []string
	session := &reviewRouteSession{run: func(_ context.Context, id, prompt string) (*utility.PromptResponse, error) {
		calls = append(calls, id)
		if id == "concrete-profile" {
			return nil, first
		}
		return nil, successor
	}}
	_, err = (reviewInference{session: session, host: &hostutility.Manager{}, profiles: resolver}).Run(ctx, identity, "session", "prompt")
	require.ErrorIs(t, err, successor)
	require.Equal(t, []string{"concrete-profile", "candidate-b"}, calls)
	require.Equal(t, "codex-acp", identity.Route.AgentID)
}
