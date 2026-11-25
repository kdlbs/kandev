package storeconformance

import (
	"fmt"
	"github.com/kandev/kandev/internal/sshconnections"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func sshConnectionSchema(s testconformance.ScenarioContext) error {
	_, err := sshconnections.NewStore(s.DB)
	return err
}
func sshConnectionAction() apiAction {
	action := apiAction{name: "ssh_connections"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := sshconnections.NewStore(s.DB)
		if err != nil {
			return nil, err
		}
		return store.SaveConnection(s.Context, sshconnections.Connection{ID: id, Name: "connection-" + id, Host: "host.example", Port: 22, User: "ssh-fixture-user"}, 0)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := sshconnections.NewStore(s.DB)
		if err != nil {
			return nil, err
		}
		return store.GetConnection(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := sshconnections.NewStore(s.DB)
		if err != nil {
			return err
		}
		c, err := store.GetConnection(s.Context, id)
		if err != nil {
			return err
		}
		c.Name = "updated-" + id
		_, err = store.SaveConnection(s.Context, c, c.Version)
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := sshconnections.NewStore(s.DB)
		if err != nil {
			return err
		}
		c, err := store.GetConnection(s.Context, id)
		if err != nil {
			return err
		}
		return store.DeleteConnection(s.Context, id, c.Version)
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := sshconnections.NewStore(s.DB)
		if err != nil {
			return err
		}
		if _, err = store.SaveConnection(s.Context, sshconnections.Connection{ID: id}, 0); err == nil {
			return fmt.Errorf("duplicate connection succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error { return transactionAPICheck(action, s, id) }
	return action
}
