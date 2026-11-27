package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.1, AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.2
func TestStopExecutionLogsOnlyRealFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		absent  bool
	}{
		{"lifecycle absence", fmt.Errorf("wrapped: %w", lifecycle.ErrExecutionNotFound), true},
		{"public runtime absence", fmt.Errorf("wrapped: %w", runtimeapi.ErrNotFound), true},
		{"transport", errors.New("connection refused"), false},
		{"deadline", context.DeadlineExceeded, false},
		{"untyped matching message", errors.New("execution not found"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestExecutor(t, &mockAgentManager{stopAgentWithReasonFunc: func(context.Context, string, string, bool) error { return tc.failure }}, newMockRepository())
			core, logs := observer.New(zap.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatal(err)
			}
			e.logger = log
			err = e.StopExecution(context.Background(), "execution", "cleanup", true)
			if !errors.Is(err, tc.failure) || !errors.Is(err, ErrExecutionNotFound) {
				t.Fatalf("lost error chain: %v", err)
			}
			if errors.Is(err, runtimeapi.ErrNotFound) != tc.absent {
				t.Fatalf("incorrect absence classification: %v", err)
			}
			warnings := logs.FilterLevelExact(zap.WarnLevel).FilterMessage("failed to stop agent by execution id").Len()
			want := 1
			if tc.absent {
				want = 0
			}
			if warnings != want {
				t.Fatalf("warnings=%d want=%d", warnings, want)
			}
		})
	}
}
