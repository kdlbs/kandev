package review

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type boundReviewUtility struct{ fakeUtility }

func (boundReviewUtility) CodeReviewAgentProfile(context.Context) (string, bool, bool, error) {
	return "bound", true, true, nil
}

type boundReviewDefaults struct{ fakeDefaults }

func (boundReviewDefaults) DefaultUtilityProfileID(context.Context) (string, error) {
	return "bound", nil
}
func TestReviewBoundBlankModelAndExplicitValidation(t *testing.T) {
	route := &ReviewerRoute{ExecutionProfileID: "concrete", SessionID: "isolated", Generation: 3, AgentID: "claude-acp"}
	profiles := &fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "claude-acp", Route: route}}
	for _, source := range []string{SourceUtilityAgent, SourceUserDefault} {
		t.Run(source, func(t *testing.T) {
			resolver := NewResolver(profiles, nil, nil)
			if source == SourceUtilityAgent {
				resolver.Utility = &boundReviewUtility{}
			} else {
				resolver.Defaults = &boundReviewDefaults{}
			}
			identity, err := resolver.Resolve(context.Background(), "")
			require.NoError(t, err)
			require.Equal(t, "", identity.Model)
			require.Equal(t, "bound", identity.ProfileID)
			require.Equal(t, source, identity.Source)
			require.Same(t, route, identity.Route)
		})
	}
	_, err := NewResolver(profiles, nil, nil).Resolve(context.Background(), "explicit")
	require.ErrorIs(t, err, ErrAgentUnavailable)
	identity, err := NewResolver(profiles, nil, &fakeDefaults{agentID: "claude-acp", model: "selected"}).Resolve(context.Background(), "explicit")
	require.NoError(t, err)
	require.Equal(t, "selected", identity.Model)
	_, err = NewResolver(profiles, nil, &fakeDefaults{agentID: "codex-acp", model: "wrong-provider"}).Resolve(context.Background(), "explicit")
	require.ErrorIs(t, err, ErrAgentUnavailable)
}

func TestReviewFailureWriterDetachedAndJoinedCause(t *testing.T) {
	for _, storageFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored", true: "storage error"}[storageFails], func(t *testing.T) {
			h, repo, service := newRecoverySQLHarness(t)
			run := seedRecoveryRun(t, repo, "run", models.ReviewTriggerManual, models.ReviewRunRunning, 1)
			storageErr := errors.New("storage unavailable")
			calls := 0
			h.runner.store = recoveryFaultStore{Store: service, fail: func(ctx context.Context, id, code, message string, duration int) (*models.TaskReviewRun, error) {
				calls++
				require.NoError(t, ctx.Err())
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.WithinDuration(t, time.Now().Add(10*time.Second), deadline, time.Second)
				require.Equal(t, run.ID, id)
				require.Equal(t, CodeCancelled, code)
				if storageFails {
					return nil, storageErr
				}
				return service.FailRun(ctx, id, code, message, duration)
			}}
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			cause := errors.Join(ErrExecutionFailed, context.DeadlineExceeded)
			err := h.runner.fail(ctx, run.ID, cause, time.Now())
			require.ErrorIs(t, err, ErrExecutionFailed)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, 1, calls)
			if storageFails {
				require.ErrorIs(t, err, storageErr)
			} else {
				stored, readErr := repo.GetTaskReviewRun(context.Background(), run.ID)
				require.NoError(t, readErr)
				require.Equal(t, models.ReviewRunFailed, stored.Status)
			}
		})
	}
}

func TestReviewExecutionExhaustionWritesOneTerminalFailure(t *testing.T) {
	h, repo, service := newRecoverySQLHarness(t)
	seedRecoveryRun(t, repo, "run", models.ReviewTriggerManual, models.ReviewRunRunning, 1)
	cause := errors.New("candidate policy exhausted")
	h.inference.err = cause
	calls := 0
	h.runner.store = recoveryFaultStore{Store: service, fail: func(ctx context.Context, id, code, message string, duration int) (*models.TaskReviewRun, error) {
		calls++
		return service.FailRun(ctx, id, code, message, duration)
	}}
	files := []ChangedFile{{Path: "a.go", Diff: "@@ -1 +1,2 @@\n old\n+new\n"}}
	err := h.runner.execute(context.Background(), RunRequest{TaskID: "task-1", SessionID: "sess-1"}, "run", ReviewerIdentity{}, files)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, h.inference.promptCount())
	stored, err := repo.GetTaskReviewRun(context.Background(), "run")
	require.NoError(t, err)
	require.Equal(t, models.ReviewRunFailed, stored.Status)
}
