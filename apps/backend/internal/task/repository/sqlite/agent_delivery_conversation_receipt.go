package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

type canonicalConversationRow struct {
	entity models.ConversationEntityKind
	id     string
}

type canonicalConversationChanges struct {
	sessionID string
	base      int64
	writes    int64
	rows      []canonicalConversationRow
	seen      map[canonicalConversationRow]bool
}

func (r *Repository) beginCanonicalConversationChanges(ctx context.Context, tx *sqlx.Tx, events []*models.AgentDeliveryEvent) (*canonicalConversationChanges, error) {
	sessionID := events[0].SessionID
	for _, event := range events {
		if event.SessionID != sessionID {
			return nil, fmt.Errorf("canonical delivery batch contains multiple sessions")
		}
	}
	base, err := r.ensureConversationRevisionTx(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	return &canonicalConversationChanges{sessionID: sessionID, base: base, seen: make(map[canonicalConversationRow]bool)}, nil
}

func (c *canonicalConversationChanges) record(entity models.ConversationEntityKind, id string) {
	c.writes++
	row := canonicalConversationRow{entity: entity, id: id}
	if c.seen[row] {
		return
	}
	c.seen[row] = true
	c.rows = append(c.rows, row)
}

func (c *canonicalConversationChanges) receipt(ctx context.Context, r *Repository, tx *sqlx.Tx) (*models.ConversationMutationReceipt, error) {
	if c.writes == 0 {
		return nil, nil
	}
	var revision int64
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT revision FROM conversation_session_revisions WHERE session_id = ?`), c.sessionID).Scan(&revision); err != nil {
		return nil, err
	}
	receipt := &models.ConversationMutationReceipt{
		SessionID: c.sessionID, BaseRevision: c.base, Revision: revision,
		Complete: revision == c.base+c.writes,
	}
	for _, row := range c.rows {
		operation, err := r.canonicalConversationOperation(ctx, tx, row)
		if err != nil {
			return nil, err
		}
		receipt.Operations = append(receipt.Operations, operation)
	}
	return receipt, nil
}

func (r *Repository) canonicalConversationOperation(ctx context.Context, tx *sqlx.Tx, row canonicalConversationRow) (models.ConversationMutationOperation, error) {
	operation := models.ConversationMutationOperation{Kind: models.ConversationMutationUpsert, Entity: row.entity, ID: row.id}
	if row.entity == models.ConversationEntityTurn {
		turn, err := r.readConversationTurnTx(ctx, tx, row.id)
		if err != nil {
			return operation, err
		}
		operation.Turn, operation.SessionID, operation.TaskID = turn, turn.TaskSessionID, turn.TaskID
	} else {
		message, err := r.readConversationMessageTx(ctx, tx, row.id)
		if err != nil {
			return operation, err
		}
		operation.Message, operation.SessionID, operation.TaskID = message, message.TaskSessionID, message.TaskID
		operation.AuthorType = string(message.AuthorType)
	}
	return operation, nil
}
