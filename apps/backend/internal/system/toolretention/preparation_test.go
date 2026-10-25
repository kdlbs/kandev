package toolretention

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"unicode/utf8"
)

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.1, 002.2, 002.6
func TestPreparationGatesInitialCleanup(t *testing.T) {
	for _, choice := range []string{"skip", "backup", "failure"} {
		t.Run(choice, func(t *testing.T) {
			s := testService(t)
			seedAnalysis(t, s)
			ctx := context.Background()
			called := false
			s.opts.CreateBackup = func(context.Context) (string, error) {
				called = true
				if choice == "failure" {
					return "", errors.New("disk full secret path")
				}
				return "verified-receipt", nil
			}
			requested := choice
			if choice == "failure" {
				requested = "backup"
			}
			_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: requested})
			require.NoError(t, err)
			err = s.stepPreparation(ctx)
			status, getErr := s.Get(ctx)
			require.NoError(t, getErr)
			if choice == "failure" {
				require.Error(t, err)
				require.False(t, status.Policy.Enabled)
				require.Equal(t, "failed", status.Preparation.State)
				require.NotContains(t, status.Preparation.Error, "secret")
				return
			}
			require.NoError(t, err)
			require.Equal(t, choice == "backup", called)
			require.True(t, status.Policy.Enabled)
			require.Equal(t, "ready", status.Preparation.State)
			require.Equal(t, "cleanup", status.Operation.Kind)
			require.Equal(t, "running", status.Operation.State)
		})
	}
}

func TestLateBackupCompletionCannotOverrideDisable(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	s.opts.CreateBackup = func(context.Context) (string, error) { close(started); <-release; return "receipt", nil }
	status, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	go func() { done <- s.stepPreparation(ctx) }()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("preparation returned without backup: %v", err)
	}
	_, err = s.Save(ctx, Update{Age: Age{3, "months"}, Revision: status.Policy.Revision})
	require.NoError(t, err)
	close(release)
	require.NoError(t, <-done)
	status, err = s.Get(ctx)
	require.NoError(t, err)
	require.False(t, status.Policy.Enabled)
	require.Equal(t, "none", status.Preparation.State)
}

func storedRetentionRow(t *testing.T, s *Service) string {
	t.Helper()
	var raw string
	require.NoError(t, sqlx.GetContext(context.Background(), s.pool.Reader(), &raw, `SELECT value FROM settings WHERE key=?`, settingsKey))
	return raw
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestPreparationFailureRecordsCause(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	const cause = "vacuum into staged snapshot: disk full"
	backupErr := errors.New(cause)
	s.opts.CreateBackup = func(context.Context) (string, error) { return "", backupErr }
	var loggedMessage string
	var loggedErr error
	s.opts.Log = func(_ context.Context, message string, err error) { loggedMessage, loggedErr = message, err }
	_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	require.EqualError(t, s.stepPreparation(ctx), "backup_failed")

	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "failed", status.Preparation.State)
	require.Equal(t, "backup_failed", status.Preparation.Error)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), cause, "status response must carry only the failure code")
	require.Contains(t, storedRetentionRow(t, s), cause, "stored record keeps the failure cause")
	require.ErrorIs(t, loggedErr, backupErr)
	require.NotEmpty(t, loggedMessage)
	stored, err := readRecord(ctx, s.pool.Reader())
	require.NoError(t, err)
	require.Equal(t, cause, stored.PreparationDetail)
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestDisableClearsPreparationDetail(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	s.opts.CreateBackup = func(context.Context) (string, error) { return "", errors.New("disk full") }
	_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	require.Error(t, s.stepPreparation(ctx))
	status, err := s.Get(ctx)
	require.NoError(t, err)
	_, err = s.Save(ctx, Update{Enabled: false, Age: Age{3, "months"}, Revision: status.Policy.Revision})
	require.NoError(t, err)
	stored, err := readRecord(ctx, s.pool.Reader())
	require.NoError(t, err)
	require.Empty(t, stored.PreparationDetail)
}

func TestLimitPreparationDetail(t *testing.T) {
	require.Equal(t, "short", limitPreparationDetail("short"))
	ascii := strings.Repeat("x", preparationDetailLimit+50)
	require.Equal(t, ascii[:preparationDetailLimit], limitPreparationDetail(ascii))
	// A three-byte rune straddles the limit and must not be split.
	multi := strings.Repeat("x", preparationDetailLimit-1) + string(rune(0x20ac)) + "tail"
	got := limitPreparationDetail(multi)
	require.Equal(t, strings.Repeat("x", preparationDetailLimit-1), got)
	require.True(t, utf8.ValidString(got))
}
