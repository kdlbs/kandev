package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestConversationSearchSetupDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{errors.New("private context: no such module: fts5"), "sqlite_fts5_unavailable"},
		{errors.New("private context: no such tokenizer: trigram"), "sqlite_trigram_unavailable"},
		{&pgconn.PgError{Code: "42501", Message: "private role"}, "postgres_extension_permission_denied"},
		{&pgconn.PgError{Code: "0A000", Message: "private server"}, "postgres_extension_unavailable"},
		{&pgconn.PgError{Code: "08006", Message: "private connection"}, "database_unavailable"},
		{errors.New("private database details"), "index_setup_failed"},
	} {
		core, logs := observer.New(zap.WarnLevel)
		log, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		reportConversationSearchSetupFailure(t.Context(), log, tc.err)
		require.Equal(t, 1, logs.Len())
		require.Equal(t, map[string]any{"reason": tc.reason}, logs.All()[0].ContextMap())
		require.NotContains(t, logs.All()[0].Message, "private")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		reportConversationSearchSetupFailure(ctx, log, tc.err)
		require.Equal(t, 1, logs.Len(), "shutdown cancellation must not produce a setup warning")
	}
}
