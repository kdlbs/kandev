package sshconnections

import (
	"context"
	"github.com/jmoiron/sqlx"
)

func (s *Service) ListConnections(ctx context.Context) ([]Route, error) {
	if _, err := s.authorize(ctx); err != nil {
		return nil, err
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := connectionsIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]Route, 0, len(rows))
	for _, c := range rows {
		r, err := routeIn(ctx, tx, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *Service) GetConnection(ctx context.Context, id string) (Route, error) {
	if _, err := s.authorize(ctx); err != nil {
		return Route{}, err
	}
	return s.readRoute(ctx, id, "")
}
func (s *Service) GetIdentity(ctx context.Context, id string) (IdentityDetails, error) {
	if _, err := s.authorize(ctx); err != nil {
		return IdentityDetails{}, err
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return IdentityDetails{}, err
	}
	defer func() { _ = tx.Rollback() }()
	identity, err := identityIn(ctx, tx, id)
	if err != nil {
		return IdentityDetails{}, err
	}
	dependencies, err := identityDependenciesIn(ctx, tx, id)
	return IdentityDetails{Identity: identity, Dependencies: dependencies}, err
}
func (s *Service) DeleteConnection(ctx context.Context, id string, version int64) error {
	actor, err := s.authorize(ctx)
	if err != nil {
		return err
	}
	return s.store.write(ctx, func(tx *sqlx.Tx) error {
		if err := deleteConnectionIn(ctx, tx, id, version); err != nil {
			return err
		}
		return audit(ctx, tx, actor, id, "connection.delete", "deleted")
	})
}
