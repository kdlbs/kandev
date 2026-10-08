package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBootstrapSelectionEvidenceSurvivesReload(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "bootstrap-selection.db")
	openRepository := func(initialize bool) (*Repository, *sqlx.DB) {
		t.Helper()
		connection, err := db.OpenSQLite(dbPath)
		require.NoError(t, err)
		database := sqlx.NewDb(connection, "sqlite3")
		var repository *Repository
		if initialize {
			repository, err = NewWithDB(database, database, nil)
			require.NoError(t, err)
		} else {
			repository = NewWithInitializedDB(database, database, nil)
		}
		return repository, database
	}

	repo, database := openRepository(true)
	seedForMsgTest(t, repo, "task-bootstrap-evidence", "session-bootstrap-evidence", "turn-bootstrap-evidence")
	promptNotSent := true
	errorValue := models.LastAgentError{
		Message:          "The agent could not start.",
		OccurredAt:       time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC),
		AgentExecutionID: "execution-bootstrap-evidence",
		ExecutionID:      "execution-bootstrap-evidence",
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "attempt-bootstrap-evidence",
		Causes: []models.AgentErrorCause{{
			Operation:      models.AgentErrorCauseOperationStart,
			Code:           models.AgentErrorCauseCodeModelUnavailable,
			Reason:         models.AgentErrorCauseReasonRequestedNotAdvertised,
			RequestedModel: "anthropic/claude-opus-4-8",
			EffectiveModel: "provider-default",
			PromptNotSent:  &promptNotSent,
		}},
	}
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session-bootstrap-evidence", models.SessionMetaKeyLastAgentError, errorValue,
	))
	require.NoError(t, database.Close())

	reopened, reopenedDB := openRepository(false)
	t.Cleanup(func() { _ = reopenedDB.Close() })
	session, err := reopened.GetTaskSession(ctx, "session-bootstrap-evidence")
	require.NoError(t, err)
	reloaded, ok := models.LoadLastAgentError(session.Metadata)
	require.True(t, ok)
	require.Len(t, reloaded.Causes, 1)
	cause := reloaded.Causes[0]
	require.Equal(t, models.AgentErrorCauseOperationStart, cause.Operation)
	require.Equal(t, models.AgentErrorCauseCodeModelUnavailable, cause.Code)
	require.Equal(t, models.AgentErrorCauseReasonRequestedNotAdvertised, cause.Reason)
	require.Equal(t, "anthropic/claude-opus-4-8", cause.RequestedModel)
	require.Equal(t, "provider-default", cause.EffectiveModel)
	require.NotNil(t, cause.PromptNotSent)
	require.True(t, *cause.PromptNotSent)
}

func TestSessionRecoveryResolutionsAreBoundedAndStampSpecific(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-recovery-resolution", "session-recovery-resolution", "turn-recovery-resolution")
	lastError := models.LastAgentError{Message: "successor", StampValue: "successor-failure"}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-recovery-resolution", models.SessionMetaKeyLastAgentError, lastError))
	resolvedAt := time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC)

	for index := 0; index < 18; index++ {
		stored, err := repo.RecordSessionRecoveryResolution(ctx, "session-recovery-resolution", models.SessionRecoveryResolution{
			ErrorStamp: fmt.Sprintf("failure-%02d", index),
			AttemptID:  fmt.Sprintf("resume-%d", index+1),
			ResolvedAt: resolvedAt.Add(time.Duration(index) * time.Minute),
		})
		require.NoError(t, err)
		require.True(t, stored)
	}
	duplicate, err := repo.RecordSessionRecoveryResolution(ctx, "session-recovery-resolution", models.SessionRecoveryResolution{
		ErrorStamp: "failure-17",
		AttemptID:  "resume-18",
		ResolvedAt: resolvedAt.Add(time.Hour),
	})
	require.NoError(t, err)
	require.True(t, duplicate)
	unsafe, err := repo.RecordSessionRecoveryResolution(ctx, "session-recovery-resolution", models.SessionRecoveryResolution{
		ErrorStamp: "failure-invalid",
		AttemptID:  "resume-18446744073709551616",
		ResolvedAt: resolvedAt,
	})
	require.NoError(t, err)
	require.False(t, unsafe)
	unsafeStamp, err := repo.RecordSessionRecoveryResolution(ctx, "session-recovery-resolution", models.SessionRecoveryResolution{
		ErrorStamp: strings.Repeat("s", 257),
		AttemptID:  "resume-19",
		ResolvedAt: resolvedAt,
	})
	require.NoError(t, err)
	require.False(t, unsafeStamp)

	session, err := repo.GetTaskSession(ctx, "session-recovery-resolution")
	require.NoError(t, err)
	resolutions := models.LoadSessionRecoveryResolutions(session.Metadata)
	require.Len(t, resolutions, 16)
	require.Equal(t, "failure-02", resolutions[0].ErrorStamp)
	require.Equal(t, "failure-17", resolutions[len(resolutions)-1].ErrorStamp)
	storedError, ok := models.LoadLastAgentError(session.Metadata)
	require.True(t, ok)
	require.Equal(t, "successor-failure", storedError.Stamp())
}
