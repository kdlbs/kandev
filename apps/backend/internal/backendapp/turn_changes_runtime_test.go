package backendapp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/changes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type summaryTurnChangesRepository struct {
	repository.TurnChangesRepository
	readErr error
}

func (r *summaryTurnChangesRepository) GetTurnChangeSet(context.Context, string, string, string) (*models.TurnChangeSet, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	at := time.Now().UTC()
	return &models.TurnChangeSet{TerminalAt: &at}, nil
}

func (*summaryTurnChangesRepository) ListTurnRepositoryChanges(context.Context, string) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

func (*summaryTurnChangesRepository) SettleTurnChangeContentProgress(context.Context, string) error {
	return nil
}

type runtimeSummaryPublisher struct {
	err error
	ids []string
}

func (p *runtimeSummaryPublisher) PublishTurnChangeSummary(_ context.Context, _, _, id string) error {
	p.ids = append(p.ids, id)
	return p.err
}

// @covers AC-TASKS-TURN-CHANGES-003.6
// @covers AC-TASKS-TURN-CHANGES-007.1
func TestRuntimeTurnChangeSummaryProcessing(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	publicationErr := errors.New("event bus unavailable")
	missing := fmt.Errorf("turn change set not found: turn: %w", repoerrors.ErrTurnChangeSetNotFound)
	for _, tc := range []struct {
		name                         string
		readErr, publishErr, wantErr error
		wantPublications             int
	}{
		{name: "missing set", readErr: missing, publishErr: missing, wantPublications: 2},
		{name: "set removed before publication", publishErr: missing, wantPublications: 2},
		{name: "present set", wantPublications: 2},
		{name: "publication retry", publishErr: publicationErr, wantErr: publicationErr, wantPublications: 1},
		{name: "processing retry", readErr: storageErr, wantErr: storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			publisher := &runtimeSummaryPublisher{err: tc.publishErr}
			handler := &runtimeTurnChangeCaptureHandler{coordinator: changes.NewCoordinator(&summaryTurnChangesRepository{readErr: tc.readErr}, nil, nil, nil, nil), publisher: publisher}
			terminal := agentruntime.TurnChangeTerminal{TurnChangeAdmission: agentruntime.TurnChangeAdmission{TaskID: "task", SessionID: "session", TurnID: "turn", PromptGeneration: 1}}
			err := handler.ProcessTurnChanges(t.Context(), terminal, nil)
			require.ErrorIs(t, err, tc.wantErr)
			require.Len(t, publisher.ids, tc.wantPublications)
			if tc.wantPublications > 0 {
				require.Equal(t, changes.ChangeSetIDForTurn("turn"), publisher.ids[0])
			}
			if tc.publishErr == publicationErr {
				publisher.err = nil
				require.NoError(t, handler.ProcessTurnChanges(t.Context(), terminal, nil))
				require.Len(t, publisher.ids, 3)
			}
		})
	}
}
