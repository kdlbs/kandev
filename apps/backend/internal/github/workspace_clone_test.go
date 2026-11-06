package github

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

type workspaceIntegrationCopier interface {
	CopyWorkspaceConfigurationTx(context.Context, *sqlx.Tx, string, string) (bool, error)
}

func TestWorkspaceCloneGitHubSources(t *testing.T) {
	for _, source := range []ConnectionSource{"", ConnectionSourcePAT, ConnectionSourceGHCLI, ConnectionSourceLegacyShared, ConnectionSourceGitHubAppInstallation, ConnectionSourceGitHubAppUser, "unknown"} {
		name := string(source)
		if name == "" {
			name = "none"
		}
		t.Run(name, func(t *testing.T) {
			store := newTestStore(t)
			ctx := t.Context()
			seedConnectionWorkspaces(t, store, "source", "target")
			seedStoreAppRegistration(t, store)
			require.NoError(t, store.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{WorkspaceID: "source", TaskGitCredentialsMode: TaskGitCredentialsModeExecutor, RepoScopeMode: RepoScopeModeAll, SavedPresets: []byte(`[{"id":"mine","default":true}]`), DefaultQueryPresets: []byte(`{"pr":[],"issue":[]}`)}))
			if source != "" {
				installation := int64(42)
				connection := &WorkspaceConnection{WorkspaceID: "source", Source: source, GitHubHost: "github.com", Status: ConnectionStatusActive, CredentialGeneration: 9}
				switch source {
				case ConnectionSourcePAT, ConnectionSourceGHCLI:
					connection.Login = "account"
				case ConnectionSourceGitHubAppInstallation:
					connection.InstallationID = &installation
					connection.AppRegistrationID = "registration-store-test"
					connection.InstallationAccountLogin = "acme"
					connection.InstallationAccountType = "Organization"
				case ConnectionSourceGitHubAppUser, "unknown":
					_, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`)
					require.NoError(t, err)
				}
				require.NoError(t, store.UpsertWorkspaceConnection(ctx, connection))
			}
			copier, ok := any(store).(workspaceIntegrationCopier)
			require.True(t, ok)
			tx, err := store.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			needsPAT, err := copier.CopyWorkspaceConfigurationTx(ctx, tx, "source", "target")
			if source == ConnectionSourceGitHubAppUser || source == "unknown" {
				require.Error(t, err)
				require.NoError(t, tx.Rollback())
				connection, readErr := store.GetWorkspaceConnection(ctx, "target")
				require.NoError(t, readErr)
				require.Nil(t, connection)
				return
			}
			require.NoError(t, err)
			require.Equal(t, source == ConnectionSourcePAT, needsPAT)
			require.NoError(t, tx.Commit())
			settings, err := store.GetWorkspaceSettings(ctx, "target")
			require.NoError(t, err)
			require.JSONEq(t, `[{"id":"mine","default":true}]`, string(settings.SavedPresets))
			require.JSONEq(t, `{"pr":[],"issue":[]}`, string(settings.DefaultQueryPresets))
			connection, err := store.GetWorkspaceConnection(ctx, "target")
			require.NoError(t, err)
			if source == "" {
				require.Nil(t, connection)
				return
			}
			require.Equal(t, source, connection.Source)
			require.EqualValues(t, 1, connection.CredentialGeneration)
			if source == ConnectionSourceGitHubAppInstallation {
				require.Equal(t, "registration-store-test", connection.AppRegistrationID)
				require.EqualValues(t, 42, *connection.InstallationID)
			}
		})
	}
}
