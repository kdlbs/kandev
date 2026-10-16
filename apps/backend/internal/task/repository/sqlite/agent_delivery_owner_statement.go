package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

type agentDeliveryOwnerStatementCache struct {
	mu         sync.Mutex
	unbound    *sqlx.Stmt
	associated *sqlx.Stmt
	closed     bool
}

// Preparation happens before writer entry. Only SQL plans are retained;
// owner and retirement evidence is read under the event's database locks.
func (r *Repository) prepareAgentDeliveryOwnerStatement(ctx context.Context, event *models.AgentDeliveryEvent) error {
	if isLegacyAgentDeliveryEventOwner(event) || !hasValidAgentDeliveryEventOwnerIdentity(event) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cache := &r.agentDeliveryOwnerStatements
	cache.mu.Lock()
	target := cache.target(event.SubmissionID != "")
	statement, closed := *target, cache.closed
	cache.mu.Unlock()
	if closed {
		return fmt.Errorf("agent delivery owner statement is closed")
	}
	if statement != nil {
		return nil
	}
	query := agentDeliveryOwnerStatementQuery(event.SubmissionID != "")
	prepared, err := r.db.PreparexContext(ctx, r.db.Rebind(query))
	if err != nil {
		return fmt.Errorf("prepare agent delivery owner statement: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = prepared.Close()
		return err
	}
	return cache.publish(prepared, event.SubmissionID != "")
}

func agentDeliveryOwnerStatementQuery(associated bool) string {
	if associated {
		return agentDeliveryLiveOwnerQuery + agentDeliveryAssociatedOwnerQuery + agentDeliveryOwnerRetirementQuery
	}
	return agentDeliveryLiveOwnerQuery + agentDeliveryOwnerRetirementQuery
}

func (cache *agentDeliveryOwnerStatementCache) target(associated bool) **sqlx.Stmt {
	if associated {
		return &cache.associated
	}
	return &cache.unbound
}

func (cache *agentDeliveryOwnerStatementCache) publish(prepared *sqlx.Stmt, associated bool) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		_ = prepared.Close()
		return fmt.Errorf("agent delivery owner statement is closed")
	}
	target := cache.target(associated)
	if *target != nil {
		return prepared.Close()
	}
	*target = prepared
	return nil
}

func (r *Repository) agentDeliveryOwnerStatement(associated bool) (*sqlx.Stmt, error) {
	cache := &r.agentDeliveryOwnerStatements
	cache.mu.Lock()
	statement := *cache.target(associated)
	cache.mu.Unlock()
	if statement == nil {
		return nil, fmt.Errorf("agent delivery owner statement is unavailable")
	}
	return statement, nil
}

func (r *Repository) closeAgentDeliveryOwnerStatements() error {
	cache := &r.agentDeliveryOwnerStatements
	cache.mu.Lock()
	unbound, associated := cache.unbound, cache.associated
	cache.unbound, cache.associated = nil, nil
	cache.closed = true
	cache.mu.Unlock()
	var errs []error
	for _, statement := range []*sqlx.Stmt{unbound, associated} {
		if statement != nil {
			errs = append(errs, statement.Close())
		}
	}
	return errors.Join(errs...)
}
