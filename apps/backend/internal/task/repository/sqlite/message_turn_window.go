package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ReadMessageTurnWindow reads messages and only their referenced turns plus
// the active turn in one read snapshot.
func (r *Repository) ReadMessageTurnWindow(
	ctx context.Context,
	sessionID string,
	opts models.ListMessagesOptions,
) (models.MessageTurnWindow, error) {
	tx, err := r.beginConversationSourceRead(ctx)
	if err != nil {
		return models.MessageTurnWindow{}, err
	}
	defer func() { _ = tx.Rollback() }()

	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}
	sortDir := "ASC"
	if strings.EqualFold(opts.Sort, "desc") {
		sortDir = "DESC"
	}
	cursor, cursorKey, err := readMessageTurnWindowPosition(ctx, tx, r.ro.DriverName(), sessionID, opts)
	if err != nil {
		return models.MessageTurnWindow{}, err
	}

	var messages []*models.Message
	var hasMore bool
	if opts.Around != "" {
		messages, hasMore, err = listMessageTurnWindowAround(ctx, tx, r.ro.DriverName(), sessionID, cursor, cursorKey, limit)
	} else {
		query, args := buildListMessagesQuery(r.ro.DriverName(), sessionID, opts, cursor, cursorKey, sortDir, limit)
		var rows *sqlx.Rows
		rows, err = tx.QueryxContext(ctx, tx.Rebind(query), args...)
		if err == nil {
			messages, hasMore, err = scanPromptIndexedMessageRows(rows, limit)
			_ = rows.Close()
		}
	}
	if err != nil {
		return models.MessageTurnWindow{}, err
	}

	activeTurnID, err := readMessageTurnWindowActiveTurnID(ctx, tx, r.ro.DriverName(), sessionID)
	if err != nil {
		return models.MessageTurnWindow{}, err
	}
	turns, err := readMessageTurnWindowTurns(ctx, tx, r.ro.DriverName(), sessionID, messages, activeTurnID)
	if err != nil {
		return models.MessageTurnWindow{}, err
	}
	coverage := &models.MessageTurnCoverage{
		MessageIDs:   make([]string, 0, len(messages)),
		ActiveTurnID: activeTurnID,
	}
	for _, message := range messages {
		if message != nil {
			coverage.MessageIDs = append(coverage.MessageIDs, message.ID)
		}
	}
	if err := tx.Commit(); err != nil {
		return models.MessageTurnWindow{}, fmt.Errorf("commit message turn window read: %w", err)
	}
	return models.MessageTurnWindow{
		Messages: messages,
		Turns:    turns,
		HasMore:  hasMore,
		Coverage: coverage,
	}, nil
}

func readMessageTurnWindowCursor(ctx context.Context, tx *sqlx.Tx, id string) (*models.Message, error) {
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages
		WHERE id = ?
	`
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	messages, _, err := scanMessageRows(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, sql.ErrNoRows
	}
	return messages[0], nil
}

func readMessageTurnWindowCursorKey(ctx context.Context, tx *sqlx.Tx, driver, id string) (string, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM task_session_messages WHERE id = ?",
		dialect.NormalizedMicrosecond(driver, "created_at"),
	)
	if dialect.IsPostgres(driver) {
		var timestamp sql.NullTime
		if err := tx.QueryRowxContext(ctx, tx.Rebind(query), id).Scan(&timestamp); err != nil {
			return "", err
		}
		if !timestamp.Valid {
			return "", fmt.Errorf("message %s has no created_at", id)
		}
		return formatPromptKey(timestamp.Time), nil
	}
	var key string
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), id).Scan(&key); err != nil {
		return "", err
	}
	return key, nil
}

func listMessageTurnWindowAround(
	ctx context.Context,
	tx *sqlx.Tx,
	driver, sessionID string,
	target *models.Message,
	targetKey string,
	limit int,
) ([]*models.Message, bool, error) {
	normalized := dialect.NormalizedMicrosecond(driver, "created_at")
	bound := "?"
	if dialect.IsPostgres(driver) {
		bound = "CAST(? AS timestamp)"
	}
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at,
		       CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END AS prompt_index
		FROM task_session_messages
		WHERE task_session_id = ? AND (%s > %s OR (%s = %s AND id >= ?))
		ORDER BY %s ASC, id ASC`, normalized, bound, normalized, bound, normalized)
	args := []interface{}{sessionID, targetKey, targetKey, target.ID}
	if limit > 0 {
		query += sqlLimitClause
		args = append(args, limit+1)
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	messages, hasMore, scanErr := scanPromptIndexedMessageRows(rows, limit)
	_ = rows.Close()
	if scanErr != nil {
		return nil, false, scanErr
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, hasMore, nil
}

func readMessageTurnWindowActiveTurnID(ctx context.Context, tx *sqlx.Tx, driver, sessionID string) (string, error) {
	predicate, orderBy := currentTurnAuthority(driver, "turn_row")
	query := fmt.Sprintf(`
		SELECT turn_row.id
		FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = ?
		  AND turn_row.completed_at IS NULL
		  AND %s
		ORDER BY %s
		LIMIT 1
	`, predicate, orderBy)
	var activeID string
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), sessionID).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return activeID, err
}

func readMessageTurnWindowTurns(
	ctx context.Context,
	tx *sqlx.Tx,
	driver, sessionID string,
	messages []*models.Message,
	activeTurnID string,
) ([]*models.Turn, error) {
	turnIDs := make([]string, 0, len(messages)+1)
	seen := make(map[string]struct{}, len(messages)+1)
	for _, message := range messages {
		if message == nil || message.TurnID == "" {
			continue
		}
		if _, ok := seen[message.TurnID]; ok {
			continue
		}
		seen[message.TurnID] = struct{}{}
		turnIDs = append(turnIDs, message.TurnID)
	}
	if activeTurnID != "" {
		if _, ok := seen[activeTurnID]; !ok {
			turnIDs = append(turnIDs, activeTurnID)
		}
	}
	if len(turnIDs) == 0 {
		return []*models.Turn{}, nil
	}

	placeholders := make([]string, len(turnIDs))
	args := make([]interface{}, 0, len(turnIDs)+1)
	args = append(args, sessionID)
	for index, id := range turnIDs {
		placeholders[index] = "?"
		args = append(args, id)
	}
	historyPredicate := turnHistoryPredicate(driver, "turn_row")
	condition := "turn_row.id IN (" + strings.Join(placeholders, ",") + ") AND " + historyPredicate
	if activeTurnID != "" {
		condition = "(" + condition + ") OR turn_row.id = ?"
		args = append(args, activeTurnID)
	}
	query := fmt.Sprintf(`
		SELECT turn_row.id, turn_row.task_session_id, turn_row.task_id, turn_row.execution_profile_id, turn_row.route_generation,
		       turn_row.started_at, turn_row.completed_at, turn_row.metadata, turn_row.created_at, turn_row.updated_at
		FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = ? AND (%s)
		ORDER BY turn_row.started_at ASC, turn_row.created_at ASC, turn_row.id ASC
	`, condition)
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	turns := make([]*models.Turn, 0, len(turnIDs))
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

func readMessageTurnWindowPosition(ctx context.Context, tx *sqlx.Tx, driver, sessionID string, opts models.ListMessagesOptions) (*models.Message, string, error) {
	id := opts.Around
	if id == "" {
		id = opts.Before
	}
	if id == "" {
		id = opts.After
	}
	if id == "" {
		return nil, "", nil
	}
	cursor, err := readMessageTurnWindowCursor(ctx, tx, id)
	if err != nil {
		return nil, "", messageTurnWindowCursorError(id, opts.Around != "", err)
	}
	if cursor.TaskSessionID != sessionID {
		if opts.Around != "" {
			return nil, "", fmt.Errorf("%w: %s", ErrMessageNotFound, id)
		}
		return nil, "", fmt.Errorf("message cursor not found: %s", id)
	}
	key, err := readMessageTurnWindowCursorKey(ctx, tx, driver, id)
	return cursor, key, messageTurnWindowCursorError(id, opts.Around != "", err)
}

func messageTurnWindowCursorError(id string, around bool, err error) error {
	if around && errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrMessageNotFound, id)
	}
	return err
}
