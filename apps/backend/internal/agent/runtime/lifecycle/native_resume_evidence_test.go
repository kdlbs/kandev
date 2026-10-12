package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeRejectsUnmatchedSQLBeforeRetiringJournal(t *testing.T) {
	for _, condition := range []string{"missing", "hash", "incarnation", "generation", "session", "live SQL", "SQL unavailable", "SQL repository missing", "already retired"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation",
				HarnessGeneration: 2, StreamID: "stream", Hash: "hash", State: journal.SubmissionInterruptedUnknown}
			peer.Retired = condition == "already retired"
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			sql := &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID,
				HarnessGeneration: int64(peer.HarnessGeneration), PayloadHash: peer.Hash, State: models.DeliverySubmissionInterruptedUnknown}
			switch condition {
			case "missing":
				sql.ID = "sql-only"
			case "hash":
				sql.PayloadHash = "different"
			case "incarnation":
				sql.IncarnationID = "different"
			case "generation":
				sql.HarnessGeneration = 1
			case "session":
				sql.SessionID = "different"
			case "live SQL":
				sql.State = models.DeliverySubmissionAccepted
			}
			repository := &adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{sql.ID: sql}}
			if condition == "SQL unavailable" {
				repository.listErr = errors.New("SQL unavailable")
			}
			manager := newTestManager(t)
			originalState := sql.State
			manager.streamManager.setAgentDeliveryRepository(repository)
			if condition == "SQL repository missing" {
				manager.streamManager.setAgentDeliveryRepository(nil)
			}
			execution := &AgentExecution{ID: "execution", SessionID: "session", ACPSessionID: "native",
				DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream",
				agentctl: nativeResumeJournalPeer(t, store, false)}
			execution.setSessionInitialized(true)
			require.NoError(t, manager.executionStore.Add(execution))
			err = manager.AcknowledgeNativeResumeDelivery(ctx, execution.SessionID)
			if condition == "already retired" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrDurableAdoptionBlocked)
			}
			retained, err := store.GetSubmission(ctx, peer.ID)
			require.NoError(t, err)
			require.Equal(t, peer.Retired, retained.Retired, "validation must precede all retirement writes")
			require.Equal(t, originalState, repository.submissions[sql.ID].State, "SQL history must remain unchanged")
		})
	}
}
