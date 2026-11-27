package sqlite

import (
	"context"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
)

const agentDeliveryReceiptQuery = `SELECT session_id, incarnation_id, harness_generation, submission_id,
	event_type, payload, terminal, projected_at, content_hash, hash_version,
	payload_pruned_at, payload_bytes, canonical_message_id
	FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`

type agentDeliveryReceiptStatementCache struct {
	mu        sync.Mutex
	statement *sqlx.Stmt
	closed    bool
}

// Only SQL metadata is prepared before writer entry. Receipt evidence is read
// from the locked transaction each time the statement is executed.
func (r *Repository) prepareAgentDeliveryReceiptStatement(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cache := &r.agentDeliveryReceiptStatements
	cache.mu.Lock()
	statement, closed := cache.statement, cache.closed
	cache.mu.Unlock()
	if closed {
		return fmt.Errorf("agent delivery receipt statement is closed")
	}
	if statement != nil {
		return nil
	}
	prepared, err := r.db.PreparexContext(ctx, r.db.Rebind(agentDeliveryReceiptQuery))
	if err != nil {
		return fmt.Errorf("prepare agent delivery receipt statement: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = prepared.Close()
		return err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		_ = prepared.Close()
		return fmt.Errorf("agent delivery receipt statement is closed")
	}
	if cache.statement != nil {
		return prepared.Close()
	}
	cache.statement = prepared
	return nil
}

func (r *Repository) agentDeliveryReceiptStatement() (*sqlx.Stmt, error) {
	cache := &r.agentDeliveryReceiptStatements
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		return nil, fmt.Errorf("agent delivery receipt statement is closed")
	}
	return cache.statement, nil
}

func (r *Repository) closeAgentDeliveryReceiptStatement() error {
	cache := &r.agentDeliveryReceiptStatements
	cache.mu.Lock()
	statement := cache.statement
	cache.statement = nil
	cache.closed = true
	cache.mu.Unlock()
	if statement != nil {
		return statement.Close()
	}
	return nil
}
