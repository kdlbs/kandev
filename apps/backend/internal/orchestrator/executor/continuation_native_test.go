package executor

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestContinuationNativeOnlyRestoreReturnsStartupFailure(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	startupFailure := errors.New("dial tcp: network is unreachable")
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releasePrompt := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releasePrompt)
	mgr := &mockAgentManager{startAgentProcessFunc: func(context.Context, string) error {
		close(started)
		<-release
		return startupFailure
	}}
	exec := newTestExecutor(t, mgr, repo)
	var projected atomic.Int32
	exec.onAgentStartFailed = func(context.Context, string, string, string, error, bool) bool { projected.Add(1); return true }
	type result struct {
		execution *TaskExecution
		err       error
	}
	completed := make(chan result, 1)
	go func() {
		execution, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true,
			ResumeOptions{RequiredNativeConversationID: "token-abc"})
		completed <- result{execution, err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not start")
	}
	releasePrompt()
	var got result
	select {
	case got = <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not settle")
	}
	require.ErrorIs(t, got.err, startupFailure, "native restore owner must receive the actual initialization failure")
	require.NotNil(t, got.execution, "the owner needs the exact failed execution for teardown")
	require.Equal(t, "exec-123", got.execution.AgentExecutionID)
	require.Zero(t, projected.Load(), "automatic continuation owns failure settlement")
}

func TestContinuationNativeOnlyRestoreRejectsUnconfirmedExecution(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	mgr := &mockAgentManager{launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		return nil, fmt.Errorf("%w: %s", lifecycle.ErrAgentAlreadyRunning, req.SessionID)
	}}
	exec := newTestExecutor(t, mgr, repo)
	_, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true, ResumeOptions{RequiredNativeConversationID: "token-abc"})
	require.Error(t, err)
	require.Equal(t, 1, mgr.launchAgentCallCount, "native-only recovery must not retry an unconfirmed teardown")
	require.Zero(t, mgr.cleanupStaleExecutionCallCount)
}

func TestContinuationNativeOnlyRestoreLaunchContract(t *testing.T) {
	options := ResumeOptions{}
	field := reflect.ValueOf(&options).Elem().FieldByName("RequiredNativeConversationID")
	require.True(t, field.IsValid(), "restore must carry required native identity")
	field.SetString("provider-session")
	req, _ := newResumeLaunchRequest(&v1.Task{ID: "t1"}, &models.TaskSession{ID: "s1"}, true, options)
	got := reflect.ValueOf(req).Elem().FieldByName("RequiredNativeConversationID")
	require.True(t, got.IsValid())
	require.Equal(t, "provider-session", got.String())
}
