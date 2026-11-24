package process

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
func TestJournalOpenDiagnosticIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	owner, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	defer func() { _ = owner.Close() }()
	_, openErr := journal.Open(journal.Config{Path: path, OpenTimeout: 25 * time.Millisecond})
	require.Error(t, openErr)

	core, observed := observer.New(zapcore.WarnLevel)
	logDeliveryJournalOpenFailure(zap.New(core), "instance-1", "session-1", path, openErr)
	entries := observed.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, string(journal.OpenReasonLockTimeout), fields["reason"])
	require.Equal(t, path, fields["journal_path"])
	require.Equal(t, "instance-1", fields["instance_id"])
	require.Equal(t, "session-1", fields["session_id"])
	for key := range fields {
		require.Contains(t, []string{
			"instance_id", "session_id", "pid", "reason", "journal_path",
			"configured_timeout", "elapsed",
		}, key, "the diagnostic must not carry request environments or prompt payloads")
	}
}
