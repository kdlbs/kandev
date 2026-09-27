package storeconformance

import (
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/provideraccess"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func providerAccessGrant(id string) provideraccess.Grant {
	return provideraccess.Grant{
		ID: id, CreatedByUserID: "conformance-admin",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		GrantScope: provideraccess.GrantScope{
			PluginInstallationID: "conformance-installation", PluginID: "conformance-plugin",
			WorkspaceID: id, ConversationKey: "conformance-conversation",
			TargetTaskID: "conformance-task", RepositoryID: "owner/repository",
			Provider: "github", Purpose: "actions_write",
		},
	}
}

func providerAccessStore(s testconformance.ScenarioContext) (*provideraccess.Store, error) {
	return provideraccess.NewStore(s.DB)
}

func providerAccessAction() apiAction {
	return apiAction{
		name: "provider-access-grants",
		create: func(s testconformance.ScenarioContext, id string) (any, error) {
			store, err := providerAccessStore(s)
			if err != nil {
				return nil, err
			}
			grant := providerAccessGrant(id)
			if err := store.ReplaceGrant(s.Context, &grant); err != nil {
				return nil, err
			}
			return &grant, nil
		},
		read: func(s testconformance.ScenarioContext, id string) (any, error) {
			store, err := providerAccessStore(s)
			if err != nil {
				return nil, err
			}
			return store.GetGrant(s.Context, id)
		},
		update: func(s testconformance.ScenarioContext, id string, _ any) error {
			store, err := providerAccessStore(s)
			if err != nil {
				return err
			}
			return store.RevokeGrant(s.Context, id, id, time.Now().UTC())
		},
		assertUpdated: func(_, after any, _ string) error {
			grant, ok := after.(*provideraccess.Grant)
			if !ok || grant.RevokedAt == nil {
				return fmt.Errorf("grant revocation did not persist")
			}
			return nil
		},
		delete: func(s testconformance.ScenarioContext, id string) error {
			store, err := providerAccessStore(s)
			if err != nil {
				return err
			}
			_, err = store.FenceWorkspace(s.Context, id)
			return err
		},
		assertDeleted: func(s testconformance.ScenarioContext, id string, _ any) error {
			store, err := providerAccessStore(s)
			if err != nil {
				return err
			}
			grant, err := store.GetGrant(s.Context, id)
			if err == nil && grant != nil {
				return fmt.Errorf("fenced workspace retained an ordinary grant")
			}
			return err
		},
		nullableTimestampUpdates: []string{"RevokedAt"},
		conflict:                 providerAccessConflict,
		transaction:              providerAccessTransaction,
	}
}

func providerAccessConflict(s testconformance.ScenarioContext, id string, _ any) error {
	store, err := providerAccessStore(s)
	if err != nil {
		return err
	}
	duplicate := providerAccessGrant(id)
	if err := store.ReplaceGrant(s.Context, &duplicate); err == nil {
		return fmt.Errorf("duplicate grant identity was accepted")
	}
	return nil
}

func providerAccessTransaction(s testconformance.ScenarioContext, id string) error {
	if err := providerAccessConflict(s, id, nil); err != nil {
		return err
	}
	store, err := providerAccessStore(s)
	if err != nil {
		return err
	}
	grant, err := store.GetActiveGrant(s.Context, providerAccessGrant(id).Scope())
	if err != nil || grant == nil || grant.ID != id {
		return fmt.Errorf("failed replacement changed the active grant: %w", err)
	}
	return nil
}
