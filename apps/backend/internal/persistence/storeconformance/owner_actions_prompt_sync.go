package storeconformance

import (
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/promptsync"
	"github.com/kandev/kandev/internal/reposync"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func promptSyncFactory(s testconformance.ScenarioContext) (*promptsync.Store, error) {
	return promptsync.NewStore(s.DB, s.DB)
}

// promptSyncAction covers the single install-wide prompt sync row. The row
// is keyed by its credential workspace for the shared scenarios.
func promptSyncAction() apiAction {
	action := apiAction{name: "prompt_sync_config", key: providerConfigKey}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		return promptSyncUpsert(s, workspaceID, "prompts")
	}
	action.read = func(s testconformance.ScenarioContext, _ string) (any, error) { return promptSyncRead(s) }
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		_, err := promptSyncUpsert(s, id, "updated")
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, _ string) error {
		store, err := promptSyncFactory(s)
		if err != nil {
			return err
		}
		return store.Delete(s.Context)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, _ string, _ any) error { return promptSyncAssertDeleted(s) }
	action.setBoolean = func(s testconformance.ScenarioContext, _ string, _ any, enabled bool) (any, error) {
		return promptSyncSetBoolean(s, enabled)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

func promptSyncRead(s testconformance.ScenarioContext) (*promptsync.Config, error) {
	store, err := promptSyncFactory(s)
	if err != nil {
		return nil, err
	}
	cfg, err := store.Get(s.Context)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "prompt sync")
}

func promptSyncUpsert(s testconformance.ScenarioContext, workspaceID, project string) (*promptsync.Config, error) {
	store, err := promptSyncFactory(s)
	if err != nil {
		return nil, err
	}
	req := &promptsync.SetConfigRequest{WorkspaceID: workspaceID, Provider: reposync.ProviderGitLab, ProjectPath: "conformance/" + project}
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	return store.Upsert(s.Context, req)
}

func promptSyncAssertDeleted(s testconformance.ScenarioContext) error {
	store, err := promptSyncFactory(s)
	if err != nil {
		return err
	}
	cfg, err := store.Get(s.Context)
	if err != nil {
		return err
	}
	if cfg != nil {
		return fmt.Errorf("prompt sync config remains after delete")
	}
	return nil
}

func promptSyncSetBoolean(s testconformance.ScenarioContext, enabled bool) (*promptsync.Config, error) {
	store, err := promptSyncFactory(s)
	if err != nil {
		return nil, err
	}
	if enabled {
		err = store.RecordSuccess(s.Context, nil, "hash", time.Now().UTC())
	} else {
		err = store.RecordFailure(s.Context, "failed", reposync.FailureDirective{Class: "transient", Consecutive: 1}, time.Now().UTC())
	}
	if err != nil {
		return nil, err
	}
	return promptSyncRead(s)
}
