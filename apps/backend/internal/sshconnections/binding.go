package sshconnections

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
)

const ConnectionConfigKey = "ssh_connection_id"

// BindExecutorIn runs inside the executor writer's transaction. The shared graph
// lock serializes binding with connection deletion and transport changes.
func (s *Service) BindExecutorIn(ctx context.Context, tx *sqlx.Tx, executorID, executorType string, config map[string]string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE ssh_connection_lock SET revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	var existing int
	if err := tx.GetContext(ctx, &existing, tx.Rebind(`SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='executor' AND owner_id=?`), executorID); err != nil {
		return err
	}
	id := config[ConnectionConfigKey]
	if !validExecutorBindingSelection(executorType, id, config) {
		return ErrInvalidRoute
	}
	if id == "" && existing == 0 {
		return nil
	}
	actor, err := s.authorize(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		if _, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM ssh_connection_refs WHERE owner_type='executor' AND owner_id=?`), executorID); err != nil {
			return err
		}
		return audit(ctx, tx, actor, executorID, "executor.unbind", "saved")
	}
	preserve, err := preserveMigratedBindingIn(ctx, tx, executorID, config)
	if err != nil || preserve {
		return err
	}
	route, err := readyExecutorRouteIn(ctx, tx, executorID, executorType, config)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connection_refs(owner_type,owner_id,connection_id,route_stamp) VALUES ('executor',?,?,?) ON CONFLICT(owner_type,owner_id) DO UPDATE SET connection_id=excluded.connection_id,route_stamp=excluded.route_stamp`), executorID, id, route.Stamp)
	if err != nil {
		return err
	}
	return audit(ctx, tx, actor, executorID, "executor.bind", "saved")
}

func validExecutorBindingSelection(executorType, id string, config map[string]string) bool {
	if executorType != executorKindSSH {
		return id == ""
	}
	return id != "" && !hasInlineSSHConfig(config)
}

func readyExecutorRouteIn(ctx context.Context, tx *sqlx.Tx, executorID, executorType string, config map[string]string) (Route, error) {
	if executorType != executorKindSSH || executorID == "" {
		return Route{}, ErrInvalidRoute
	}
	if hasInlineSSHConfig(config) {
		return Route{}, ErrInvalidRoute
	}
	route, err := routeIn(ctx, tx, config[ConnectionConfigKey])
	if err != nil {
		return Route{}, err
	}
	if config["ssh_connection_stamp"] != route.Stamp {
		return Route{}, ErrInvalidRoute
	}
	return route, checkVerifiedRouteIn(ctx, tx, route)
}

func checkVerifiedRouteIn(ctx context.Context, tx *sqlx.Tx, route Route) error {
	if route.Destination.VerifiedRoute != route.Stamp {
		return ErrInvalidRoute
	}
	if route.Bastion != nil {
		bastion, err := routeIn(ctx, tx, route.Bastion.ID)
		if err != nil {
			return err
		}
		if bastion.Destination.VerifiedRoute != bastion.Stamp {
			return ErrInvalidRoute
		}
	}
	return nil
}

func hasInlineSSHConfig(config map[string]string) bool {
	for _, key := range migrationInlineKeys {
		if _, present := config[key]; present {
			return true
		}
	}
	return false
}

func preserveMigratedBindingIn(ctx context.Context, tx *sqlx.Tx, executorID string, config map[string]string) (bool, error) {
	migration, err := migrationIn(ctx, tx, executorID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if migration.Verified || migration.ConnectionID != config[ConnectionConfigKey] || migration.RouteStamp != config["ssh_connection_stamp"] {
		return false, nil
	}
	return executorRefMatchesIn(ctx, tx, executorID, migration.ConnectionID, migration.RouteStamp)
}

func executorRefMatchesIn(ctx context.Context, tx *sqlx.Tx, executorID, id, stamp string) (bool, error) {
	var count int
	err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='executor' AND owner_id=? AND connection_id=? AND route_stamp=?`), executorID, id, stamp)
	return count == 1, err
}
