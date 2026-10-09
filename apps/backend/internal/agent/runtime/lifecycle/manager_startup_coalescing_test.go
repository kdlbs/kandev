package lifecycle

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
)

func TestStartAgentProcessDoesNotJoinWorkspacePreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)
		execution := &AgentExecution{ID: "exec-start", SessionID: "session-start"}
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}

		entered, release := make(chan struct{}), make(chan struct{})
		workspace := mgr.ensureExecutionGroup.DoChan(execution.SessionID, func() (interface{}, error) {
			close(entered)
			<-release
			return execution, nil
		})
		<-entered
		started := make(chan error, 1)
		go func() { started <- mgr.StartAgentProcess(context.Background(), execution.ID) }()
		synctest.Wait()

		select {
		case err := <-started:
			if err == nil || !strings.Contains(err.Error(), "no agentctl client") {
				t.Errorf("startup error = %v, want its own missing-client error", err)
			}
		default:
			t.Error("subprocess startup joined the unrelated workspace preparation")
		}
		close(release)
		<-workspace
		synctest.Wait()
	})
}
