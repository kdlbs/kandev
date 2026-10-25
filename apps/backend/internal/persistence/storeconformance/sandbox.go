package storeconformance

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

const sandboxStoreID = "sandbox-policies"
const schemaMetaStoreID = "schema-meta"
const updatedSandboxName = "Updated"

func sandboxSchema(s testconformance.ScenarioContext) error {
	_, err := sandbox.NewService(s.DB, false, nil)
	return err
}

func sandboxActor(s testconformance.ScenarioContext) context.Context {
	return authn.WithIdentity(s.Context, authn.Identity{UserID: "conformance-operator", Role: authn.RoleAdmin, SessionID: "conformance-browser"})
}

func sandboxAction() apiAction {
	action := apiAction{name: "sandbox_policy_revisions"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		svc, err := sandbox.NewService(s.DB, true, nil)
		if err != nil {
			return nil, err
		}
		return svc.Publish(sandboxActor(s), sandbox.Policy{ID: id, Name: "Conformance", InstallationID: "fixture", ProviderKey: "fixture"}, 0)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		svc, err := sandbox.NewService(s.DB, true, nil)
		if err != nil {
			return nil, err
		}
		return svc.GetLatest(sandboxActor(s), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		svc, err := sandbox.NewService(s.DB, true, nil)
		if err != nil {
			return err
		}
		current, err := svc.GetLatest(sandboxActor(s), id)
		if err != nil {
			return err
		}
		policy := current.Policy
		policy.Name = updatedSandboxName
		_, err = svc.Publish(sandboxActor(s), policy, current.Number)
		return err
	}
	action.assertUpdated = func(before, after any, _ string) error {
		previous, current := before.(*sandbox.Revision), after.(*sandbox.Revision)
		if current.Policy.Name != updatedSandboxName || current.Number <= previous.Number || current.ID == previous.ID {
			return fmt.Errorf("sandbox revision was not published")
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		svc, err := sandbox.NewService(s.DB, true, nil)
		if err != nil {
			return err
		}
		current, err := svc.GetLatest(sandboxActor(s), id)
		if err != nil {
			return err
		}
		return svc.Revoke(sandboxActor(s), current.ID, current.Digest)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		value, err := action.read(s, id)
		if err != nil {
			return err
		}
		if !value.(*sandbox.Revision).Revoked {
			return fmt.Errorf("sandbox revision was not revoked")
		}
		return nil
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		_, err := action.create(s, id)
		if !errors.Is(err, sandbox.ErrConflict) {
			return fmt.Errorf("sandbox stale publication: %w", err)
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error { return transactionAPICheck(action, s, id) }
	return action
}
