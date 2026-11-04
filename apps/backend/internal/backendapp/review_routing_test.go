package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/hostutility"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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

type localReviewSessionRepo struct {
	session          *taskmodels.TaskSession
	executorType     taskmodels.ExecutorType
	taskRepositories []*taskmodels.TaskRepository
}

type unavailableReviewExecution struct{ execution *lifecycle.AgentExecution }

func (e unavailableReviewExecution) GetExecutionBySessionID(string) (*lifecycle.AgentExecution, bool) {
	return e.execution, true
}
func (e unavailableReviewExecution) GetOrEnsureExecution(context.Context, string) (*lifecycle.AgentExecution, error) {
	return e.execution, nil
}
func (unavailableReviewExecution) CheckSessionExecAccess(context.Context, string) error { return nil }

func (r localReviewSessionRepo) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return r.session, nil
}
func (r localReviewSessionRepo) GetExecutor(context.Context, string) (*taskmodels.Executor, error) {
	return &taskmodels.Executor{Type: r.executorType}, nil
}

func (r localReviewSessionRepo) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	return &taskmodels.Repository{ID: id, Name: "repo-" + id}, nil
}

func (r localReviewSessionRepo) ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error) {
	return r.taskRepositories, nil
}

// TestReviewLocalFallbackAnchorsOnLiveTarget keeps changes that already landed
// on the target branch out of a task review when the session stored base is
// older than the target the task merged.
func TestReviewLocalFallbackAnchorsOnLiveTarget(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init")
	git("config", "user.email", "review@example.test")
	git("config", "user.name", "Review Test")
	write("base.txt", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := git("rev-parse", "HEAD")
	git("checkout", "-b", "upstream")
	write("upstream.txt", "upstream\n")
	git("add", ".")
	git("commit", "-m", "upstream")
	git("update-ref", "refs/remotes/origin/main", git("rev-parse", "HEAD"))
	git("checkout", "-b", "task", base)
	git("merge", "--no-ff", "-m", "merge upstream", "upstream")
	write("task.txt", "task\n")
	git("add", ".")
	git("commit", "-m", "task change")
	r := localReviewSessionRepo{session: &taskmodels.TaskSession{
		ID: "s", ExecutorID: "exec", BaseBranch: "main", BaseCommitSHA: base,
		Worktrees: []*taskmodels.TaskEnvironmentRepo{{WorktreePath: dir}},
	}, executorType: taskmodels.ExecutorTypeLocal}
	files, err := review.CollectChanges(context.Background(), handlers.NewReviewChangeSource(unavailableReviewExecution{}, reviewSessionReader{repo: r}), "s", "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if file.Path == "upstream.txt" {
			t.Fatalf("merged upstream change was reviewed: %+v", files)
		}
		if file.Path == "task.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("task change missing: %+v", files)
	}
}

func TestReviewLocalFallbackUsesQualifiedComparisonTarget(t *testing.T) {
	for _, materialized := range []bool{true, false} {
		name := "materialized target"
		if !materialized {
			name = "unavailable target does not fall back to origin"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(name, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			git("init")
			git("config", "user.email", "review@example.test")
			git("config", "user.name", "Review Test")
			write("base.txt", "base\n")
			git("add", ".")
			git("commit", "-m", "base")
			base := git("rev-parse", "HEAD")

			git("checkout", "-b", "origin-main")
			write("fork.txt", "fork-only\n")
			git("add", ".")
			git("commit", "-m", "fork main")
			git("update-ref", "refs/remotes/origin/main", git("rev-parse", "HEAD"))

			git("checkout", "-b", "upstream-main", base)
			write("upstream.txt", "upstream target\n")
			git("add", ".")
			git("commit", "-m", "upstream main")
			upstream := git("rev-parse", "HEAD")

			target := taskmodels.ComparisonTarget{
				Version: taskmodels.ComparisonTargetVersion, Provider: taskmodels.ComparisonTargetProviderGitHub,
				Kind: taskmodels.ComparisonTargetKindPullRequest, Number: 12, HeadBranch: "feature/review",
				TargetBranch: "main",
				HeadRepository: taskmodels.ComparisonTargetRepository{
					Host: "github.com", Path: "contributor/project", RemoteURL: "https://github.com/contributor/project.git",
				},
				TargetRepository: taskmodels.ComparisonTargetRepository{
					Host: "github.com", Path: "upstream/project", RemoteURL: "https://github.com/upstream/project.git",
				},
			}
			metadata := map[string]interface{}{}
			if err := taskmodels.PutComparisonTarget(metadata, &target); err != nil {
				t.Fatal(err)
			}
			if materialized {
				git("update-ref", target.ComparisonRef(), upstream)
			}

			git("checkout", "-b", "task", upstream)
			write("task.txt", "task change\n")
			git("add", ".")
			git("commit", "-m", "task change")

			repo := localReviewSessionRepo{
				session: &taskmodels.TaskSession{
					ID: "s", TaskID: "task", ExecutorID: "exec", RepositoryID: "repo",
					BaseBranch: "main", BaseCommitSHA: base,
					Worktrees: []*taskmodels.TaskEnvironmentRepo{{RepositoryID: "repo", WorktreePath: dir}},
				},
				executorType:     taskmodels.ExecutorTypeLocal,
				taskRepositories: []*taskmodels.TaskRepository{{TaskID: "task", RepositoryID: "repo", BaseBranch: "main", Metadata: metadata}},
			}
			files, err := review.CollectChanges(context.Background(), handlers.NewReviewChangeSource(unavailableReviewExecution{}, reviewSessionReader{repo: repo}), "s", "")
			if !materialized {
				if !errors.Is(err, review.ErrWorkspaceUnavailable) {
					t.Fatalf("missing comparison target error = %v, want workspace comparison unavailable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 || files[0].Path != "task.txt" {
				t.Fatalf("files = %+v, want only task.txt relative to the upstream target", files)
			}
		})
	}
}

func TestReviewAgentctlUnavailableUsesLocalGit(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init")
	git("config", "user.name", "Review Test")
	git("config", "user.email", "review@example.test")
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	git("tag", "review-base")
	if err := os.WriteFile(file, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "committed.go"), []byte("committed change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "committed.go")
	git("commit", "-m", "branch change")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatal(err)
	}
	execution := &lifecycle.AgentExecution{SessionID: "s"}
	execution.SetAgentCtlClientForTesting(agentctl.NewClient(u.Hostname(), port, log))
	for _, typ := range []taskmodels.ExecutorType{taskmodels.ExecutorTypeLocal, taskmodels.ExecutorTypeWorktree, taskmodels.ExecutorTypeSSH} {
		t.Run(string(typ), func(t *testing.T) {
			r := localReviewSessionRepo{session: &taskmodels.TaskSession{ID: "s", ExecutorID: "exec", BaseBranch: "review-base", Worktrees: []*taskmodels.TaskEnvironmentRepo{{RepositoryID: "primary", WorktreePath: dir}}}, executorType: typ}
			src := handlers.NewReviewChangeSource(unavailableReviewExecution{execution}, reviewSessionReader{repo: r})
			files, err := review.CollectChanges(context.Background(), src, "s", "")
			if typ == taskmodels.ExecutorTypeSSH {
				if !errors.Is(err, review.ErrWorkspaceUnavailable) {
					t.Fatalf("remote executor must not read host disk: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("local git must work without agentctl: %v", err)
			}
			if len(files) != 2 || files[0].Path != "committed.go" || !strings.Contains(files[1].Diff, "+new") {
				t.Fatalf("files = %+v", files)
			}
			for _, file := range files {
				if file.RepositoryName != "" || file.RepositoryID != "" || file.Key() != file.Path {
					t.Fatalf("single-repo scope must be the legacy bare path: %+v", file)
				}
			}
		})
	}
	t.Run("repository scopes", func(t *testing.T) {
		r := localReviewSessionRepo{session: &taskmodels.TaskSession{ID: "s", TaskID: "task", ExecutorID: "exec", RepositoryID: "one", Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{RepositoryID: "one", WorkspaceRelativePath: "one", WorktreePath: dir},
			{RepositoryID: "two", WorkspaceRelativePath: "two", WorktreePath: dir},
		}}, executorType: taskmodels.ExecutorTypeLocal, taskRepositories: []*taskmodels.TaskRepository{
			{TaskID: "task", RepositoryID: "one", BaseBranch: "main"},
			{TaskID: "task", RepositoryID: "two", BaseBranch: "main"},
		}}
		files, err := review.CollectChanges(context.Background(), handlers.NewReviewChangeSource(unavailableReviewExecution{}, reviewSessionReader{repo: r}), "s", "")
		if err != nil {
			t.Fatal(err)
		}
		keys := make(map[string]bool, len(files))
		for _, file := range files {
			keys[file.Key()] = true
		}
		if len(files) != 2 || !keys["one\x00main.go"] || !keys["two\x00main.go"] {
			t.Fatalf("multi-repo scope keys = %+v", files)
		}
	})
	t.Run("map key matches panel scope", func(t *testing.T) {
		single := reviewSessionReader{repo: localReviewSessionRepo{session: &taskmodels.TaskSession{ID: "s", ExecutorID: "exec", Worktrees: []*taskmodels.TaskEnvironmentRepo{{RepositoryID: "primary", WorktreePath: dir}}}, executorType: taskmodels.ExecutorTypeLocal}}
		uncommitted, err := single.LocalReviewFiles(context.Background(), "s", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := uncommitted["main.go"]; !ok {
			t.Fatalf("single-repo map key must be the bare path: %v", uncommitted)
		}
		multi := reviewSessionReader{repo: localReviewSessionRepo{session: &taskmodels.TaskSession{ID: "m", ExecutorID: "exec", Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{RepositoryID: "one", WorkspaceRelativePath: "one", WorktreePath: dir},
			{RepositoryID: "two", WorkspaceRelativePath: "two", WorktreePath: dir},
		}}, executorType: taskmodels.ExecutorTypeLocal}}
		scoped, err := multi.LocalReviewFiles(context.Background(), "m", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := scoped["one\x00main.go"]; !ok {
			t.Fatalf("multi-repo map key must be the panel scope key: %v", scoped)
		}
		rooted := reviewSessionReader{repo: localReviewSessionRepo{session: &taskmodels.TaskSession{ID: "r", ExecutorID: "exec", Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{RepositoryID: "root", WorktreePath: dir},
			{RepositoryID: "two", WorkspaceRelativePath: "two", WorktreePath: dir},
		}}, executorType: taskmodels.ExecutorTypeLocal}}
		pair, err := rooted.LocalReviewFiles(context.Background(), "r", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := pair["\x00main.go"]; !ok {
			t.Fatalf("multi-repo root key must be the empty scope key: %v", pair)
		}
	})
}
