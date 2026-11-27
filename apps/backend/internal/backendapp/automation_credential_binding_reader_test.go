package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/automation"
)

func credentialBindingContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "creator-1", Role: authn.RoleMember})
}

func TestLifecycleAutomationCredentialBindingReaderAllowsUnprotectedRun(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store, err := automation.NewStore(database, database)
	require.NoError(t, err)
	service := automation.NewService(store, nil, nil)
	ctx := credentialBindingContext()
	definition := &automation.Automation{
		WorkspaceID: "workspace-1", Name: "ordinary automation", CreatedByUserID: "creator-1",
	}
	require.NoError(t, store.CreateAutomation(ctx, definition))
	run := &automation.AutomationRun{
		AutomationID: definition.ID, TaskID: "task-1", TriggerType: automation.TriggerTypeManual,
		Status: automation.RunStatusTriggered,
	}
	require.NoError(t, store.CreateRun(ctx, run))

	bindings, err := (lifecycleAutomationCredentialBindingReader{service: service}).ResolveAutomationCredentialBindings(
		ctx, run.TaskID, definition.ID, run.ID,
	)
	require.NoError(t, err)
	require.Empty(t, bindings)
}

func TestLifecycleAutomationCredentialBindingReaderRejectsInvalidProtectedAssociation(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store, err := automation.NewStore(database, database)
	require.NoError(t, err)
	service := automation.NewService(store, nil, nil)
	ctx := credentialBindingContext()
	definition := &automation.Automation{
		WorkspaceID: "workspace-1", Name: "credential automation", CreatedByUserID: "creator-1",
	}
	require.NoError(t, store.CreateAutomation(ctx, definition))
	run := &automation.AutomationRun{
		AutomationID: definition.ID, TaskID: "task-1", TriggerType: automation.TriggerTypeManual,
		Status: automation.RunStatusTriggered, ProtectedCreatorUserID: "different-creator",
	}
	require.NoError(t, store.CreateRun(ctx, run))

	reader := lifecycleAutomationCredentialBindingReader{service: service}
	_, err = reader.ResolveAutomationCredentialBindings(ctx, "different-task", definition.ID, run.ID)
	require.ErrorIs(t, err, automation.ErrCredentialChoiceUnavailable)
	_, err = reader.ResolveAutomationCredentialBindings(ctx, run.TaskID, "different-automation", run.ID)
	require.ErrorIs(t, err, automation.ErrCredentialChoiceUnavailable)
	_, err = reader.ResolveAutomationCredentialBindings(ctx, run.TaskID, definition.ID, run.ID)
	require.ErrorIs(t, err, automation.ErrCredentialChoiceUnavailable)

	protectedWithoutChoices := &automation.AutomationRun{
		AutomationID: definition.ID, TaskID: "task-2", TriggerType: automation.TriggerTypeManual,
		Status: automation.RunStatusTriggered, ProtectedCreatorUserID: definition.CreatedByUserID,
	}
	require.NoError(t, store.CreateRun(ctx, protectedWithoutChoices))
	_, err = reader.ResolveAutomationCredentialBindings(
		ctx, protectedWithoutChoices.TaskID, definition.ID, protectedWithoutChoices.ID,
	)
	require.ErrorIs(t, err, automation.ErrCredentialChoiceUnavailable)
}
