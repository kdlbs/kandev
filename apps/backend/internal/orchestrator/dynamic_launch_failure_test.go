package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Launch failures raised by Kandev itself (workspace preparation, command
// validation, runtime detection, the local agentctl control plane) say nothing
// about the candidate's provider. They must reach the conductor unclassified,
// so no provider fallback suspends the candidate.
func TestDynamicLaunchFailuresFromKandevAreNotProviderFailures(t *testing.T) {
	for _, launchErr := range []error{
		errors.New("workspace is preparing: retry after the initial workspace launch completes"),
		errors.New("workspace reuse is unsafe: existing task environment is not attachable"),
		errors.New("workspace reuse is unsafe: load task environment while waiting: context deadline exceeded"),
		errors.New("validate agent command: agent command cannot be empty"),
		errors.New("detect native OpenCode runtime: read native OpenCode version: exit status 1"),
		errors.New(`detect native OpenCode runtime: read native OpenCode version: exit status 1; ` +
			`output: "Error: connect ECONNREFUSED 127.0.0.1:4096"`),
		errors.New(`failed to create execution: failed to create standalone instance: failed to create instance: ` +
			`Post "http://127.0.0.1:41044/api/v1/instances": dial tcp 127.0.0.1:41044: i/o timeout`),
		errors.New(`failed to create instance: Post "http://127.0.0.1:41044/api/v1/instances": ` +
			`dial tcp 127.0.0.1:41044: connect: connection refused`),
		errors.New("prepare workspace: git fetch: service temporarily unavailable"),
	} {
		t.Run(launchErr.Error(), func(t *testing.T) {
			err := launchDynamicDownstreamWithError(t, launchErr)
			if err == nil {
				t.Fatal("launch error was swallowed")
			}
			var classified *routingerr.Error
			if errors.As(err, &classified) {
				t.Fatalf("Kandev launch failure classified as provider %s (%s): %v", classified.Code, classified.ClassifierRule, err)
			}
		})
	}
}

// The agent process itself can still report a provider failure while it
// starts; that evidence keeps routing the launch.
func TestDynamicLaunchAgentStartupProviderFailureStillRoutes(t *testing.T) {
	startup := routingerr.NewAgentStartupFailure(routingerr.PhaseProcessStart, "claude-acp",
		errors.New("request to the provider API failed: ECONNREFUSED"))
	err := launchDynamicDownstreamWithError(t, fmt.Errorf("launch agent: %w", startup))
	var classified *routingerr.Error
	if !errors.As(err, &classified) || classified.Code != routingerr.CodeNetworkUnavailable {
		t.Fatalf("startup failure = %v, want a classified network_unavailable failure", err)
	}
}

func TestDynamicLaunchUnknownStartupFailureRemainsUnclassified(t *testing.T) {
	for _, phase := range []routingerr.Phase{routingerr.PhaseProcessStart, routingerr.PhaseSessionInit} {
		t.Run(string(phase), func(t *testing.T) {
			startup := routingerr.NewAgentStartupFailure(phase, "claude-acp",
				errors.New("unrecognized startup error"))
			err := launchDynamicDownstreamWithError(t, fmt.Errorf("launch agent: %w", startup))
			if !errors.Is(err, startup) {
				t.Fatalf("startup failure = %v, want the original startup error preserved", err)
			}
			var classified *routingerr.Error
			if errors.As(err, &classified) {
				t.Fatalf("unknown startup failure classified as provider %s: %v", classified.Code, err)
			}
		})
	}
}

// An error that already carries a classification keeps it.
func TestDynamicLaunchKeepsAnExistingClassification(t *testing.T) {
	existing := &routingerr.Error{
		Code: routingerr.CodeAuthRequired, Confidence: routingerr.ConfHigh, FallbackAllowed: true,
	}
	err := launchDynamicDownstreamWithError(t, fmt.Errorf("launch agent: %w", existing))
	var classified *routingerr.Error
	if !errors.As(err, &classified) || classified != existing {
		t.Fatalf("classified launch failure = %v, want the original classification preserved", err)
	}
}

func launchDynamicDownstreamWithError(t *testing.T, launchErr error) error {
	t.Helper()
	ctx := context.Background()
	const (
		taskID    = "task-dynamic-launch-kandev-failure"
		sessionID = "session-dynamic-launch-kandev-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, sessionID, "")

	downstream := &dynamicTaskDownstream{
		service:   svc,
		task:      &v1.Task{ID: taskID, WorkspaceID: "ws1", Description: "test"},
		sessionID: sessionID,
		options:   executor.LaunchOptions{AgentProfileID: "candidate-1", StartAgent: true},
	}
	_, err := downstream.Launch(ctx, dynamicruntime.DownstreamLaunch{
		ExecutionProfileID: "candidate-1",
		Decision:           decision,
	})
	return err
}
