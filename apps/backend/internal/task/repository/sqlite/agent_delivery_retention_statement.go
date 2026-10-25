package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
)

type agentDeliveryReloadStatementCache struct {
	mu        sync.Mutex
	statement *sqlx.Stmt
	hashes    *sqlx.Stmt
	payloads  *sqlx.Stmt
	closed    bool
}

// Only the SQL plan is reused. Its parameters and all evidence rows remain
// transaction-local and are read again after the delivery locks are acquired.
func (r *Repository) prepareAgentDeliveryReloadStatement(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cache := &r.agentDeliveryReloadStatements
	cache.mu.Lock()
	statement, closed := cache.statement, cache.closed
	cache.mu.Unlock()
	if closed {
		return fmt.Errorf("agent delivery reload statement is closed")
	}
	if statement != nil {
		return nil
	}
	prepared, err := r.prepareAgentDeliveryRetentionStatements(ctx)
	if err != nil {
		return err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		_ = prepared.close()
		return fmt.Errorf("agent delivery reload statement is closed")
	}
	if cache.statement != nil {
		_ = prepared.close()
		return nil
	}
	cache.statement, cache.hashes, cache.payloads = prepared.statement, prepared.hashes, prepared.payloads
	return nil
}

func (r *Repository) prepareAgentDeliveryRetentionStatements(ctx context.Context) (*agentDeliveryReloadStatementCache, error) {
	prepared := &agentDeliveryReloadStatementCache{}
	for _, item := range []struct {
		query  string
		target **sqlx.Stmt
	}{
		{agentDeliveryLockedReloadQuery, &prepared.statement},
		{agentDeliveryFixedCompactHashesSQL, &prepared.hashes},
		{agentDeliveryFixedCompactPayloadsSQL, &prepared.payloads},
	} {
		statement, err := r.db.PreparexContext(ctx, r.db.Rebind(item.query))
		if err != nil {
			_ = prepared.close()
			return nil, fmt.Errorf("prepare agent delivery retention statement: %w", err)
		}
		*item.target = statement
		if err := ctx.Err(); err != nil {
			_ = prepared.close()
			return nil, err
		}
	}
	return prepared, nil
}

func (cache *agentDeliveryReloadStatementCache) close() error {
	var errs []error
	for _, statement := range []*sqlx.Stmt{cache.statement, cache.hashes, cache.payloads} {
		if statement != nil {
			errs = append(errs, statement.Close())
		}
	}
	return errors.Join(errs...)
}

func (r *Repository) closeAgentDeliveryReloadStatement() error {
	cache := &r.agentDeliveryReloadStatements
	cache.mu.Lock()
	statements := &agentDeliveryReloadStatementCache{statement: cache.statement, hashes: cache.hashes, payloads: cache.payloads}
	cache.statement, cache.hashes, cache.payloads = nil, nil, nil
	cache.closed = true
	cache.mu.Unlock()
	return statements.close()
}

func (r *Repository) agentDeliveryReloadStatement() (*sqlx.Stmt, error) {
	cache := &r.agentDeliveryReloadStatements
	cache.mu.Lock()
	statement := cache.statement
	cache.mu.Unlock()
	if statement == nil {
		return nil, fmt.Errorf("agent delivery reload statement is unavailable")
	}
	return statement, nil
}

func (r *Repository) agentDeliveryCompactionStatements() (*sqlx.Stmt, *sqlx.Stmt, error) {
	cache := &r.agentDeliveryReloadStatements
	cache.mu.Lock()
	hashes, payloads := cache.hashes, cache.payloads
	cache.mu.Unlock()
	if hashes == nil || payloads == nil {
		return nil, nil, fmt.Errorf("agent delivery compaction statements are unavailable")
	}
	return hashes, payloads, nil
}
