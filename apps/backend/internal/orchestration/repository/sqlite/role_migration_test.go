package sqlite

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGlobalRoleMigrationPreservesDistinctAssignments(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)
	_, err = database.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY);
 CREATE TABLE agent_profiles(id TEXT PRIMARY KEY,workspace_id TEXT,name TEXT,role TEXT,settings TEXT,deleted_at TEXT);
 INSERT INTO workspaces VALUES('personal'),('work');
 INSERT INTO agent_profiles VALUES('chief1','personal','Personal chief','assistant','{"orchestrator_icon":"🌱","delegation_context":"Personal projects"}',NULL),('chief2','work','Work chief','assistant','{"orchestrator_icon":"💼","delegation_context":"Work projects"}',NULL);
 CREATE TABLE orchestration_roles(id TEXT PRIMARY KEY,name TEXT,instructions TEXT);
 INSERT INTO orchestration_roles VALUES('chief-of-staff','Chief of staff','Original template');
 CREATE TABLE workspace_orchestrators(agent_id TEXT PRIMARY KEY,workspace_id TEXT,role_id TEXT);
 INSERT INTO workspace_orchestrators VALUES('chief1','personal','chief-of-staff'),('chief2','work','chief-of-staff');`)
	require.NoError(t, err)
	repo := New(database, database)
	require.NoError(t, repo.migratePersonaStorage())
	require.NoError(t, repo.UpsertInstruction(context.Background(), "chief1", "ROLE.md", "Personal policy", false))
	require.NoError(t, repo.UpsertInstruction(context.Background(), "chief2", "ROLE.md", "Work policy", false))
	require.NoError(t, repo.Migrate())
	first, err := repo.OrchestratorRoleID(context.Background(), "chief1")
	require.NoError(t, err)
	second, err := repo.OrchestratorRoleID(context.Background(), "chief2")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	role, err := repo.GetOrchestratorRole(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, "Personal chief", role.Name)
	require.Equal(t, "🌱", role.Icon)
	require.Equal(t, "Personal policy", role.Instructions)
	role.Instructions = "Updated globally"
	require.NoError(t, repo.SaveOrchestratorRole(context.Background(), role))
	require.NoError(t, repo.Migrate())
	role, err = repo.GetOrchestratorRole(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, "Updated globally", role.Instructions)
	var settings string
	require.NoError(t, database.Get(&settings, `SELECT settings FROM agent_profiles WHERE id='chief2'`))
	require.Contains(t, settings, "Work projects")
}
