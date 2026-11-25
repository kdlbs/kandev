package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresRetiredAzureDevOpsSchemaIsDropped(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	for _, table := range retiredAzureDevOpsTables {
		_, err := db.Exec(`CREATE TABLE ` + table + ` (id TEXT PRIMARY KEY)`)
		require.NoError(t, err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS secrets (id TEXT PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO secrets (id, name) VALUES ('azure_devops:ws-1:pat', 'pat'), ('jira:ws-1:token', 'token')`)
	require.NoError(t, err)

	for range 2 {
		require.NoError(t, repo.runMigrations(context.Background()))
	}

	for _, table := range retiredAzureDevOpsTables {
		var exists bool
		require.NoError(t, db.Get(&exists, `SELECT to_regclass($1) IS NOT NULL`, table))
		require.False(t, exists, table)
	}
	var azureSecrets, otherSecrets int
	require.NoError(t, db.Get(&azureSecrets, `SELECT COUNT(1) FROM secrets WHERE id LIKE 'azure_devops:%'`))
	require.NoError(t, db.Get(&otherSecrets, `SELECT COUNT(1) FROM secrets WHERE id = 'jira:ws-1:token'`))
	require.Zero(t, azureSecrets)
	require.Equal(t, 1, otherSecrets)
}

func TestPostgresRetiredAzureDevOpsRepositoriesRecordTheirProviderHost(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'ws-1', $1, $2)`, now, now)
	require.NoError(t, err)
	for _, row := range retiredAzureRepositoryRows {
		_, err := db.Exec(`INSERT INTO repositories
			(id, workspace_id, name, source_type, provider, provider_host, remote_url, created_at, updated_at)
			VALUES ($1, 'ws-1', $2, 'provider', $3, $4, $5, $6, $7)`,
			row.id, row.id, row.provider, row.host, row.remoteURL, now, now)
		require.NoError(t, err)
	}

	for range 2 {
		require.NoError(t, repo.runMigrations(context.Background()))
	}

	for _, row := range retiredAzureRepositoryRows {
		var host string
		require.NoError(t, db.Get(&host, `SELECT provider_host FROM repositories WHERE id = $1`, row.id))
		require.Equal(t, row.wantHost, host, row.id)
	}
}
