package sqlite

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestSetSessionMetadataKeyIfJSONValuePostgres exercises the same object
// member identity and serialization-order contract against the PostgreSQL
// JSONB branch. It skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestSetSessionMetadataKeyIfJSONValuePostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresMetadataCASRepo(t, database)

	tests := []struct {
		name       string
		actual     json.RawMessage
		expected   json.RawMessage
		wantStored bool
	}{
		{
			name:       "swapped execution and attempt ownership",
			actual:     json.RawMessage(`{"execution_id":"attempt-1","attempt_id":"execution-1"}`),
			expected:   json.RawMessage(`{"execution_id":"execution-1","attempt_id":"attempt-1"}`),
			wantStored: false,
		},
		{
			name:       "renamed member with duplicate values",
			actual:     json.RawMessage(`{"execution_id":"same-value","renamed_id":"same-value"}`),
			expected:   json.RawMessage(`{"execution_id":"same-value","attempt_id":"same-value"}`),
			wantStored: false,
		},
		{
			name:       "removed member with duplicate values",
			actual:     json.RawMessage(`{"execution_id":"same-value"}`),
			expected:   json.RawMessage(`{"execution_id":"same-value","attempt_id":"same-value"}`),
			wantStored: false,
		},
		{
			name:       "reordered object serialization",
			actual:     json.RawMessage(`{"attempt_id":"attempt-1","execution_id":"execution-1"}`),
			expected:   json.RawMessage(`{"execution_id":"execution-1","attempt_id":"attempt-1"}`),
			wantStored: true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffix := strconv.Itoa(i)
			taskID := "task-metadata-members-pg-" + suffix
			sessionID := "session-metadata-members-pg-" + suffix
			seedPostgresTaskSession(t, repo, taskID, sessionID)
			ctx := context.Background()
			require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "submission", tt.actual))

			stored, err := repo.SetSessionMetadataKeyIfJSONValue(ctx, sessionID, "submission", tt.expected, json.RawMessage(`{"state":"replacement"}`))
			require.NoError(t, err)
			require.Equal(t, tt.wantStored, stored)

			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			var want map[string]interface{}
			if tt.wantStored {
				require.NoError(t, json.Unmarshal([]byte(`{"state":"replacement"}`), &want))
			} else {
				require.NoError(t, json.Unmarshal(tt.actual, &want))
			}
			require.Equal(t, want, session.Metadata["submission"])
		})
	}
}
