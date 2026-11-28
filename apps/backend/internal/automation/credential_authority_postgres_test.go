package automation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresFirstCredentialGrantSerializesDefinitionAndRunAdmission(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(5)
	store, err := NewStore(database, database)
	require.NoError(t, err)
	secondStore, err := NewStore(database, database)
	require.NoError(t, err)
	_, err = store.db.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, ownership_revision BIGINT NOT NULL DEFAULT 1
	)`)
	require.NoError(t, err)
	_, err = store.db.Exec(`INSERT INTO workspaces (id, owner_id, ownership_revision) VALUES ('ws-first-grant', 'owner-1', 1)`)
	require.NoError(t, err)
	a := &Automation{
		WorkspaceID: "ws-first-grant", CreatedByUserID: "creator-1", Name: "first grant race",
		Repositories: []AutomationRepository{{RepositoryID: "repo-1", BaseBranch: "main"}},
	}
	require.NoError(t, store.CreateAutomation(t.Context(), a))

	grantTx, err := database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	_, err = lockCredentialAutomation(t.Context(), grantTx, a.ID)
	require.NoError(t, err)

	updateCtx := withAutomationRunInitiator(credentialIdentity("collaborator-1"))
	newPrompt := "collaborator executable edit"
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- secondStore.UpdateAutomation(updateCtx, a.ID, &UpdateAutomationRequest{Prompt: &newPrompt})
	}()
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		TriggerData: json.RawMessage(`{}`),
	}
	runDone := make(chan error, 1)
	go func() { runDone <- secondStore.CreateRun(updateCtx, run) }()

	require.Eventually(t, func() bool {
		var waiting int
		if err := database.Get(&waiting, `SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`); err != nil {
			return false
		}
		return waiting >= 2
	}, 5*time.Second, 20*time.Millisecond)

	grantedAt := time.Now().UTC()
	_, err = grantTx.ExecContext(t.Context(), grantTx.Rebind(`
		INSERT INTO automation_credential_choices (
			automation_id, kind, profile_id, repository_id, env_key, secret_id,
			source_secret_id, source_updated_at, creator_user_id, grantor_user_id, state, revision,
			secret_name, created_at, updated_at, workspace_owner_revision)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, 1, '', ?, ?, 1)`),
		a.ID, CredentialChoiceWorkspace, "repo-1", "SERVICE_KEY", "workspace-secret", "workspace-secret",
		grantedAt, a.CreatedByUserID, "owner-1", CredentialChoiceGranted, grantedAt, grantedAt)
	require.NoError(t, err)
	require.NoError(t, grantTx.Commit())

	require.ErrorIs(t, <-updateDone, ErrCredentialCreatorRequired)
	require.ErrorIs(t, <-runDone, ErrCredentialCreatorRequired)
	saved, err := store.GetAutomation(t.Context(), a.ID)
	require.NoError(t, err)
	require.Empty(t, saved.Prompt)
	savedRun, err := store.GetRun(t.Context(), run.ID)
	require.NoError(t, err)
	require.Nil(t, savedRun)
}
