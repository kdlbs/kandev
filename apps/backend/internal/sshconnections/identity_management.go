package sshconnections

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"strings"
)

func (s *Service) ListIdentities(ctx context.Context) ([]IdentityDetails, error) {
	if _, err := s.authorize(ctx); err != nil {
		return nil, err
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var rows []string
	if err := tx.SelectContext(ctx, &rows, `SELECT data FROM ssh_identities ORDER BY id`); err != nil {
		return nil, err
	}
	out := make([]IdentityDetails, 0, len(rows))
	for _, row := range rows {
		var identity IdentityDetails
		if err := json.Unmarshal([]byte(row), &identity); err != nil {
			return nil, err
		}
		identity.Dependencies, err = identityDependenciesIn(ctx, tx, identity.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, nil
}
func (s *Service) DeleteIdentity(ctx context.Context, id string, version int64) error {
	actor, err := s.authorize(ctx)
	if err != nil {
		return err
	}
	return s.store.write(ctx, func(tx *sqlx.Tx) error {
		identity, err := identityIn(ctx, tx, id)
		if err != nil {
			return err
		}
		if identity.Version != version {
			return ErrConflict
		}
		dependencies, err := identityDependenciesIn(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(dependencies) > 0 {
			return &IdentityInUse{Dependencies: dependencies}
		}
		if identity.Kind == identityManaged {
			if err = s.vault.Delete(ctx, tx, "ssh-identity:"+id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM ssh_identities WHERE id=?`), id); err != nil {
			return err
		}
		return audit(ctx, tx, actor, id, "identity.delete", "deleted")
	})
}
func (s *Service) RenameIdentity(ctx context.Context, id, label string, version int64) (Identity, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Identity{}, err
	}
	if strings.TrimSpace(label) == "" || len(label) > 200 {
		return Identity{}, ErrInvalidIdentity
	}
	var out Identity
	err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		var err error
		out, err = identityIn(ctx, tx, id)
		if err != nil {
			return err
		}
		if out.Version != version {
			return ErrConflict
		}
		out.Label = strings.TrimSpace(label)
		out.Version++
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE ssh_identities SET data=? WHERE id=?`), string(data), id); err != nil {
			return err
		}
		return audit(ctx, tx, actor, id, "identity.rename", "updated")
	})
	return out, err
}
