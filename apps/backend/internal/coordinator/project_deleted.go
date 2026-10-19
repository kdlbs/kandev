package coordinator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// ProjectDeleted removes the entries naming a deleted repository set or
// repository (kind is projectKindSet or projectKindRepository) and publishes
// coordinator.updated once per affected coordinator. It is a data repair, not a
// manager save: it never archives a conversation or raises a policy revision,
// and it is idempotent.
func (s *Service) ProjectDeleted(ctx context.Context, kind, id string) error {
	type target struct{ coordinatorID, workspaceID string }
	var targets []target
	rows, err := s.store.ro.QueryContext(ctx, s.store.db.Rebind(`SELECT DISTINCT coordinator_id, workspace_id FROM coordinator_watch_projects WHERE entry_kind = ? AND entry_id = ? ORDER BY coordinator_id`), kind, id)
	if err != nil {
		return fmt.Errorf("read watchers: %w", err)
	}
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.coordinatorID, &t.workspaceID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan watcher: %w", err)
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	var firstErr error
	for _, t := range targets {
		removed := false
		err := s.store.withCoordinatorLock(ctx, t.coordinatorID, func(tx coordinatorExec) error {
			res, err := tx.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM coordinator_watch_projects WHERE coordinator_id = ? AND entry_kind = ? AND entry_id = ?`), t.coordinatorID, kind, id)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			removed = n > 0
			return nil
		})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if removed {
			s.publishCoordinatorUpdated(ctx, t.workspaceID, t.coordinatorID)
		}
	}
	return firstErr
}

// SubscribeProjectDeleted subscribes svc to repository.deleted and
// repository_set.deleted so the stored entries naming them are removed. It is
// best effort: a failure is logged and not retried, and the stale entry stays
// harmless because membership is resolved against live listings.
func SubscribeProjectDeleted(eventBus bus.EventBus, svc *Service, log *logger.Logger) ([]bus.Subscription, error) {
	l := log.WithFields(zap.String("component", "coordinator-project-deleted-subscriber"))
	subjects := []struct{ subject, kind string }{
		{events.RepositoryDeleted, projectKindRepository},
		{events.RepositorySetDeleted, projectKindSet},
	}
	var subs []bus.Subscription
	for _, sub := range subjects {
		kind := sub.kind
		subscription, err := eventBus.Subscribe(sub.subject, func(ctx context.Context, event *bus.Event) error {
			id := deletedEventID(event)
			if id == "" {
				return nil
			}
			if err := svc.ProjectDeleted(ctx, kind, id); err != nil {
				l.Warn("tidy coordinator projects failed", zap.String("kind", kind), zap.String("id", id), zap.Error(err))
			}
			return nil
		})
		if err != nil {
			return subs, err
		}
		subs = append(subs, subscription)
	}
	return subs, nil
}

func deletedEventID(event *bus.Event) string {
	if event == nil {
		return ""
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := data["id"].(string)
	return strings.TrimSpace(id)
}
