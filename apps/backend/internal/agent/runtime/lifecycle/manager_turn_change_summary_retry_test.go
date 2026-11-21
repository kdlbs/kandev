package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-TASKS-TURN-CHANGES-003.6
// @covers AC-TASKS-TURN-CHANGES-004.2
func TestTurnChangeSummaryRetryTerminates(t *testing.T) {
	publicationOutage := make([]error, 9)
	for i := range 8 {
		publicationOutage[i] = errors.New("event bus unavailable")
	}
	for _, test := range []struct {
		name         string
		results      []error
		wantAttempts int
		wantStopped  int
	}{
		{"success", []error{nil}, 1, 0},
		{"missing record", []error{repoerrors.ErrTurnChangeSetNotFound}, 1, 1},
		{"invalid ownership", []error{repoerrors.ErrTurnChangeRelationship}, 1, 1},
		{"wrapped missing record", []error{fmt.Errorf("process summary: %w", repoerrors.ErrTurnChangeSetNotFound)}, 1, 1},
		{"joined ownership error", []error{errors.Join(errors.New("checkpoint unavailable"), repoerrors.ErrTurnChangeRelationship)}, 1, 1},
		{"transient recovery", []error{errors.New("database busy"), context.DeadlineExceeded, nil}, 3, 0},
		{"publication recovers after eight failures", publicationOutage, 9, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				core, logs := observer.New(zap.WarnLevel)
				log, err := logger.NewFromZap(zap.New(core))
				require.NoError(t, err)
				handler := &summaryRetryHandler{results: test.results}
				manager := &Manager{stopCh: make(chan struct{}), logger: log, turnChangeCaptureHandler: handler}
				defer func() {
					close(manager.stopCh)
					manager.wg.Wait()
				}()

				manager.scheduleTurnChangeSummary(&AgentExecution{ID: "summary-execution"}, TurnChangeTerminal{})
				// Virtual time checks permanent failures and recovery after extended outages.
				time.Sleep(time.Minute)
				synctest.Wait()
				handler.mu.Lock()
				attempts := handler.attempts
				handler.mu.Unlock()
				require.Equal(t, test.wantAttempts, attempts)
				require.Equal(t, test.wantStopped, logs.FilterMessage("stopped turn-change summary processing").Len())
			})
		})
	}
}

func TestTurnChangeSummaryBeforeTeardownDoesNotRetryPermanentFailure(t *testing.T) {
	for _, failure := range []error{repoerrors.ErrTurnChangeSetNotFound, repoerrors.ErrTurnChangeRelationship} {
		t.Run(failure.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				core, logs := observer.New(zap.WarnLevel)
				log, err := logger.NewFromZap(zap.New(core))
				require.NoError(t, err)
				handler := &summaryRetryHandler{results: []error{failure}}
				manager := &Manager{stopCh: make(chan struct{}), logger: log, turnChangeCaptureHandler: handler}
				defer func() {
					close(manager.stopCh)
					manager.wg.Wait()
				}()

				manager.processTurnChangeSummaryBeforeTeardown(&AgentExecution{ID: "teardown-summary"}, TurnChangeTerminal{})
				time.Sleep(time.Minute)
				synctest.Wait()
				handler.mu.Lock()
				attempts := handler.attempts
				handler.mu.Unlock()
				require.Equal(t, 1, attempts)
				require.Equal(t, 1, logs.FilterMessage("stopped turn-change summary processing").Len())
			})
		})
	}
}

func TestTurnChangeSummaryRetryStopsOnShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		core, logs := observer.New(zap.WarnLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		handler := &summaryRetryHandler{entered: make(chan struct{})}
		manager := &Manager{stopCh: make(chan struct{}), logger: log, turnChangeCaptureHandler: handler}
		manager.scheduleTurnChangeSummary(&AgentExecution{ID: "shutdown-summary"}, TurnChangeTerminal{})
		<-handler.entered
		close(manager.stopCh)
		manager.wg.Wait()
		require.Empty(t, logs.All(), "shutdown cancellation must not start another retry or warn")
	})
}

type summaryRetryHandler struct {
	mu       sync.Mutex
	attempts int
	results  []error
	entered  chan struct{}
}

func (*summaryRetryHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (*summaryRetryHandler) FinishTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error {
	return nil
}

func (h *summaryRetryHandler) ProcessTurnChanges(ctx context.Context, _ TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	if h.entered != nil {
		close(h.entered)
		<-ctx.Done()
		return ctx.Err()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attempts++
	return h.results[min(h.attempts-1, len(h.results)-1)]
}
