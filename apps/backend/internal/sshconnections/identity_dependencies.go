package sshconnections

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type IdentityDetails struct {
	Identity
	Dependencies []IdentityDependency `json:"dependencies"`
}

// IdentityDependency identifies public metadata that prevents key deletion.
type IdentityDependency struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Label        string `json:"label,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
}

type IdentityInUse struct {
	Dependencies []IdentityDependency `json:"dependencies"`
}

func (*IdentityInUse) Error() string { return ErrInUse.Error() }
func (*IdentityInUse) Unwrap() error { return ErrInUse }

func identityDependenciesIn(ctx context.Context, tx *sqlx.Tx, id string) ([]IdentityDependency, error) {
	routes, err := connectionsIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	direct := map[string]bool{}
	for _, c := range routes {
		if c.IdentityID == id {
			direct[c.ID] = true
		}
	}
	used := map[string]bool{}
	deps := make([]IdentityDependency, 0)
	for _, c := range routes {
		if direct[c.ID] || direct[c.BastionID] {
			used[c.ID] = true
			deps = append(deps, IdentityDependency{Kind: "connection", ID: c.ID, Label: c.Name})
		}
	}
	if len(used) == 0 {
		return deps, nil
	}
	var refs []struct {
		Kind         string `db:"owner_type"`
		ID           string `db:"owner_id"`
		ConnectionID string `db:"connection_id"`
	}
	if err := tx.SelectContext(ctx, &refs, `SELECT owner_type,owner_id,connection_id FROM ssh_connection_refs WHERE owner_type IN ('executor','runtime','runtime_pending') ORDER BY owner_type,owner_id`); err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if !used[ref.ConnectionID] {
			continue
		}
		dep := IdentityDependency{Kind: ref.Kind, ID: ref.ID, ConnectionID: ref.ConnectionID}
		if ref.Kind == "runtime" || ref.Kind == "runtime_pending" {
			dep.Kind = "runtime"
			err := tx.GetContext(ctx, &dep.TaskID, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id=?`), ref.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		deps = append(deps, dep)
	}
	return deps, nil
}
