package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// asyncQuestionSchemaDDL creates the task-owned async question lifecycle.
// These tables are additive: they never convert existing blocking
// clarification messages and never share their pending IDs.
const asyncQuestionSchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_async_questions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL,
		session_incarnation_id TEXT NOT NULL DEFAULT '',
		turn_id TEXT NOT NULL DEFAULT '',
		recipient_kind TEXT NOT NULL,
		parent_task_id TEXT NOT NULL DEFAULT '',
		parent_session_id TEXT NOT NULL DEFAULT '',
		parent_session_incarnation_id TEXT NOT NULL DEFAULT '',
		questions_json TEXT NOT NULL,
		question_context TEXT NOT NULL DEFAULT '',
		retry_key TEXT NOT NULL,
		payload_digest TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		outcome_json TEXT NOT NULL DEFAULT '',
		withdraw_reason TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_async_questions_retry
		ON task_async_questions(session_id, retry_key);
	CREATE INDEX IF NOT EXISTS idx_async_questions_workspace
		ON task_async_questions(workspace_id, status, created_at);
	CREATE INDEX IF NOT EXISTS idx_async_questions_task
		ON task_async_questions(task_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_async_questions_session
		ON task_async_questions(session_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_async_questions_parent
		ON task_async_questions(parent_task_id, status, created_at);

	CREATE TABLE IF NOT EXISTS task_async_question_forwards (
		question_id TEXT PRIMARY KEY,
		parent_task_id TEXT NOT NULL,
		parent_session_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_async_question_forwards_parent
		ON task_async_question_forwards(parent_task_id);

	CREATE TABLE IF NOT EXISTS task_async_question_deliveries (
		admission_id TEXT PRIMARY KEY,
		question_id TEXT NOT NULL,
		outcome_version INTEGER NOT NULL,
		kind TEXT NOT NULL,
		destination_task_id TEXT NOT NULL,
		destination_session_id TEXT NOT NULL,
		destination_incarnation_id TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		state TEXT NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TIMESTAMP NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		admitted_at TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_async_question_deliveries_identity
		ON task_async_question_deliveries(question_id, outcome_version, kind, destination_session_id);
	CREATE INDEX IF NOT EXISTS idx_async_question_deliveries_due
		ON task_async_question_deliveries(state, next_attempt_at);

	CREATE TABLE IF NOT EXISTS task_async_question_waits (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		turn_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'requested',
		reason TEXT NOT NULL DEFAULT '',
		released_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_async_question_waits_session
		ON task_async_question_waits(session_id, status);

	CREATE TABLE IF NOT EXISTS task_async_question_wait_dependencies (
		wait_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		PRIMARY KEY (wait_id, question_id)
	);
	CREATE INDEX IF NOT EXISTS idx_async_question_wait_dependencies_question
		ON task_async_question_wait_dependencies(question_id);
`

func (r *Repository) initAsyncQuestionSchema() error {
	_, err := r.db.Exec(asyncQuestionSchemaDDL)
	return err
}

const asyncQuestionSelectColumns = `
	id, task_id, workspace_id, session_id, session_incarnation_id, turn_id,
	recipient_kind, parent_task_id, parent_session_id, parent_session_incarnation_id, questions_json,
	question_context, retry_key, payload_digest, status, outcome_json,
	withdraw_reason, version, created_at, updated_at, resolved_at`

const asyncQuestionFromClause = ` FROM task_async_questions `

// asyncQuestionSelectSQL adds the forward marker without changing the stored
// row: a forwarded request is a link, not a copied request.
const asyncQuestionSelectSQL = `SELECT ` + asyncQuestionSelectColumns + `,
	CASE WHEN EXISTS (SELECT 1 FROM task_async_question_forwards f WHERE f.question_id = task_async_questions.id) THEN 1 ELSE 0 END` + asyncQuestionFromClause

func scanAsyncQuestion(row rowScanner) (*models.AsyncQuestion, error) {
	var (
		question       models.AsyncQuestion
		recipient      string
		status         string
		questionsJSON  string
		outcomeJSON    string
		resolvedAt     sql.NullTime
		forwardedValue int
	)
	if err := row.Scan(
		&question.ID, &question.TaskID, &question.WorkspaceID, &question.SessionID,
		&question.SessionIncarnationID, &question.TurnID, &recipient,
		&question.ParentTaskID, &question.ParentSessionID, &question.ParentSessionIncarnationID, &questionsJSON,
		&question.Context, &question.RetryKey, &question.PayloadDigest, &status,
		&outcomeJSON, &question.WithdrawReason, &question.Version,
		&question.CreatedAt, &question.UpdatedAt, &resolvedAt, &forwardedValue,
	); err != nil {
		return nil, err
	}
	question.Recipient = models.AsyncQuestionRecipient(recipient)
	question.Status = models.AsyncQuestionStatus(status)
	question.Forwarded = forwardedValue == 1
	if resolvedAt.Valid {
		value := resolvedAt.Time
		question.ResolvedAt = &value
	}
	if err := json.Unmarshal([]byte(questionsJSON), &question.Questions); err != nil {
		return nil, fmt.Errorf("decode async question %s: %w", question.ID, err)
	}
	if outcomeJSON != "" {
		var outcome models.AsyncQuestionOutcome
		if err := json.Unmarshal([]byte(outcomeJSON), &outcome); err != nil {
			return nil, fmt.Errorf("decode async question outcome %s: %w", question.ID, err)
		}
		question.Outcome = &outcome
	}
	return &question, nil
}

// CreateAsyncQuestion persists one request. An exact retry with the same retry
// key returns the recorded request and created=false. A different payload
// digest with the same key is a conflict without a write.
func (r *Repository) CreateAsyncQuestion(ctx context.Context, question *models.AsyncQuestion, deliveries []*models.AsyncQuestionDelivery) (*models.AsyncQuestion, bool, error) {
	questionsJSON, err := prepareAsyncQuestion(question, r.nowUTC())
	if err != nil {
		return nil, false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockAsyncQuestionScopeTx(ctx, tx, "create:"+question.SessionID); err != nil {
		return nil, false, err
	}

	existing, err := r.findExistingAsyncQuestionTx(ctx, tx, question)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.PayloadDigest != question.PayloadDigest || existing.RetryKey != question.RetryKey {
			return nil, false, ErrAsyncQuestionRetryConflict
		}
		return existing, false, tx.Commit()
	}
	if err := r.insertAsyncQuestionTx(ctx, tx, question, questionsJSON); err != nil {
		return nil, false, err
	}
	if err := r.insertAsyncQuestionDeliveriesTx(ctx, tx, question.ID, question.Version, question.CreatedAt, deliveries); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return question, true, nil
}

func prepareAsyncQuestion(question *models.AsyncQuestion, now time.Time) (string, error) {
	if question == nil || question.ID == "" || question.SessionID == "" || question.TaskID == "" {
		return "", errors.New("async question identity is incomplete")
	}
	questionsJSON, err := json.Marshal(question.Questions)
	if err != nil {
		return "", err
	}
	if question.CreatedAt.IsZero() {
		question.CreatedAt = now
	}
	question.UpdatedAt = question.CreatedAt
	if question.Version == 0 {
		question.Version = 1
	}
	if question.Status == "" {
		question.Status = models.AsyncQuestionStatusPending
	}
	return string(questionsJSON), nil
}

func (r *Repository) findExistingAsyncQuestionTx(ctx context.Context, tx *sqlx.Tx, question *models.AsyncQuestion) (*models.AsyncQuestion, error) {
	existing, err := r.getAsyncQuestionTx(ctx, tx, question.ID)
	if err != nil || existing != nil {
		return existing, err
	}
	return r.getAsyncQuestionByRetryTx(ctx, tx, question.SessionID, question.RetryKey)
}

func (r *Repository) insertAsyncQuestionTx(ctx context.Context, tx *sqlx.Tx, question *models.AsyncQuestion, questionsJSON string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_async_questions
			(id, task_id, workspace_id, session_id, session_incarnation_id, turn_id,
			 recipient_kind, parent_task_id, parent_session_id, parent_session_incarnation_id, questions_json,
			 question_context, retry_key, payload_digest, status, outcome_json,
			 withdraw_reason, version, created_at, updated_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, NULL)`),
		question.ID, question.TaskID, question.WorkspaceID, question.SessionID,
		question.SessionIncarnationID, question.TurnID, string(question.Recipient),
		question.ParentTaskID, question.ParentSessionID, question.ParentSessionIncarnationID, questionsJSON,
		question.Context, question.RetryKey, question.PayloadDigest,
		string(question.Status), question.Version, question.CreatedAt, question.UpdatedAt,
	)
	return err
}

// insertAsyncQuestionDeliveriesTx writes immutable outbox intents with the
// supplied outcome version in the caller's transaction.
func (r *Repository) insertAsyncQuestionDeliveriesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	questionID string,
	outcomeVersion int64,
	createdAt time.Time,
	deliveries []*models.AsyncQuestionDelivery,
) error {
	for _, delivery := range deliveries {
		if delivery == nil {
			continue
		}
		delivery.QuestionID = questionID
		delivery.OutcomeVersion = outcomeVersion
		if delivery.CreatedAt.IsZero() {
			delivery.CreatedAt = createdAt
		}
		if delivery.NextAttemptAt.IsZero() {
			delivery.NextAttemptAt = delivery.CreatedAt
		}
		if delivery.State == "" {
			delivery.State = models.AsyncQuestionDeliveryPending
		}
		metadataJSON, err := json.Marshal(delivery.Metadata)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_async_question_deliveries
				(admission_id, question_id, outcome_version, kind, destination_task_id,
				 destination_session_id, destination_incarnation_id, content, metadata_json, state, attempts,
				 next_attempt_at, last_error, created_at, admitted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
			ON CONFLICT (admission_id) DO NOTHING`),
			delivery.AdmissionID, delivery.QuestionID, delivery.OutcomeVersion,
			string(delivery.Kind), delivery.DestinationTaskID, delivery.DestinationSession,
			delivery.DestinationIncarnationID, delivery.Content, string(metadataJSON), string(delivery.State), delivery.Attempts,
			delivery.NextAttemptAt, delivery.LastError, delivery.CreatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

// ErrAsyncQuestionRetryConflict reports a reused retry identity whose content
// differs from the recorded request.
var ErrAsyncQuestionRetryConflict = errors.New("async question retry identity already exists with different content")

// ErrAsyncQuestionNotFound reports an absent request.
var ErrAsyncQuestionNotFound = errors.New("async question not found")

// ErrAsyncQuestionNotPending reports an attempted transition from a terminal
// state; the caller must read the canonical winner instead.
var ErrAsyncQuestionNotPending = errors.New("async question is not pending")

func (r *Repository) getAsyncQuestionTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.AsyncQuestion, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(asyncQuestionSelectSQL+" WHERE task_async_questions.id = ?"), id)
	question, err := scanAsyncQuestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return question, err
}

func (r *Repository) getAsyncQuestionByRetryTx(ctx context.Context, tx *sqlx.Tx, sessionID, retryKey string) (*models.AsyncQuestion, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(asyncQuestionSelectSQL+
		" WHERE task_async_questions.session_id = ? AND task_async_questions.retry_key = ?"),
		sessionID, retryKey)
	question, err := scanAsyncQuestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return question, err
}

// GetAsyncQuestion reads one canonical request.
func (r *Repository) GetAsyncQuestion(ctx context.Context, id string) (*models.AsyncQuestion, error) {
	row := r.db.QueryRowxContext(ctx, r.db.Rebind(asyncQuestionSelectSQL+" WHERE task_async_questions.id = ?"), id)
	question, err := scanAsyncQuestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return question, err
}

// ListAsyncQuestions returns a bounded page ordered by creation time then id.
func (r *Repository) ListAsyncQuestions(ctx context.Context, opts models.ListAsyncQuestionsOptions) (*models.AsyncQuestionPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	where, args := asyncQuestionFilters(opts)
	query := asyncQuestionSelectSQL + where + " ORDER BY task_async_questions.created_at ASC, task_async_questions.id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	page := &models.AsyncQuestionPage{}
	for rows.Next() {
		question, scanErr := scanAsyncQuestion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		page.Questions = append(page.Questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Questions) > limit {
		page.Questions = page.Questions[:limit]
		page.HasMore = true
	}
	return page, nil
}

// CountOpenAsyncQuestions returns the bounded actionable count for one
// task-visible scope. It counts requests, not task groups.
func (r *Repository) CountOpenAsyncQuestions(ctx context.Context, opts models.ListAsyncQuestionsOptions) (int, error) {
	opts.OpenOnly = true
	opts.Limit = 1
	where, args := asyncQuestionFilters(opts)
	var count int
	query := "SELECT COUNT(*) FROM task_async_questions " + where
	if err := r.db.GetContext(ctx, &count, r.db.Rebind(query), args...); err != nil {
		return 0, err
	}
	return count, nil
}

func asyncQuestionFilters(opts models.ListAsyncQuestionsOptions) (string, []any) {
	conditions, args := asyncQuestionIdentityFilters(opts)
	conditions, args = asyncQuestionPopulationFilters(conditions, args, opts)
	// The forward predicate is an operator-population rule. An author-scoped
	// read always owns its requests, including parent-authored ones.
	if !opts.IncludeUnforwardedParents && opts.AuthorSessionID == "" && opts.SessionID == "" {
		conditions = append(conditions, asyncQuestionForwardedPredicate)
	}
	if opts.RequireAnswerable {
		conditions = append(conditions, asyncQuestionAnswerablePredicate)
	}
	if opts.RequireCurrentParent {
		conditions = append(conditions, asyncQuestionCurrentParentPredicate)
	}
	if opts.Sidecar != nil {
		hide := `(s.state = 'dismissed' OR (s.state = 'snoozed' AND s.snooze_until > ?))`
		prefix := "NOT EXISTS"
		if opts.Sidecar.Only {
			prefix = "EXISTS"
		}
		conditions = append(conditions, prefix+` (SELECT 1 FROM clarification_inbox_sidecar s
			WHERE s.user_id = ? AND s.pending_id = 'async:' || task_async_questions.id AND `+hide+`)`)
		args = append(args, opts.Sidecar.UserID, opts.Sidecar.Now)
	}
	if opts.OpenOnly {
		conditions = append(conditions, "status = 'pending'")
	} else if len(opts.Statuses) > 0 {
		placeholders, statusArgs := asyncQuestionPlaceholders(len(opts.Statuses))
		for i, status := range opts.Statuses {
			statusArgs[i] = string(status)
		}
		conditions = append(conditions, "status IN ("+strings.Join(placeholders, ", ")+")")
		args = append(args, statusArgs...)
	}
	if len(opts.QuestionIDs) > 0 {
		placeholders, ids := asyncQuestionPlaceholders(len(opts.QuestionIDs))
		for i, id := range opts.QuestionIDs {
			ids[i] = id
		}
		conditions = append(conditions, "id IN ("+strings.Join(placeholders, ", ")+")")
		args = append(args, ids...)
	}
	if !opts.CursorCreatedAt.IsZero() {
		conditions = append(conditions, "(created_at > ? OR (created_at = ? AND id > ?))")
		args = append(args, opts.CursorCreatedAt, opts.CursorCreatedAt, opts.CursorID)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

const asyncQuestionCurrentParentPredicate = `EXISTS (
	SELECT 1 FROM tasks parent_check
	WHERE parent_check.id = task_async_questions.task_id
	  AND parent_check.parent_id = task_async_questions.parent_task_id
	  AND parent_check.workspace_id = task_async_questions.workspace_id
	  AND parent_check.archived_at IS NULL)`

const asyncQuestionAnswerablePredicate = `EXISTS (
	SELECT 1 FROM task_sessions ts
	JOIN tasks t ON t.id = ts.task_id
	WHERE ts.id = task_async_questions.session_id
	  AND ts.task_id = task_async_questions.task_id
	  AND ts.state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
	  AND t.archived_at IS NULL
	  AND t.workspace_id = task_async_questions.workspace_id
	  AND (task_async_questions.session_incarnation_id = '' OR ts.queue_incarnation_id = task_async_questions.session_incarnation_id))`

const asyncQuestionForwardedPredicate = "((recipient_kind = 'user') OR EXISTS (SELECT 1 FROM task_async_question_forwards f WHERE f.question_id = task_async_questions.id))"

func asyncQuestionIdentityFilters(opts models.ListAsyncQuestionsOptions) ([]string, []any) {
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if opts.WorkspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, opts.WorkspaceID)
	}
	if len(opts.WorkspaceIDs) > 0 {
		placeholders, ids := asyncQuestionPlaceholders(len(opts.WorkspaceIDs))
		for i, id := range opts.WorkspaceIDs {
			ids[i] = id
		}
		conditions = append(conditions, "workspace_id IN ("+strings.Join(placeholders, ", ")+")")
		args = append(args, ids...)
	}
	if opts.TaskID != "" {
		conditions = append(conditions, "task_id = ?")
		args = append(args, opts.TaskID)
	}
	return conditions, args
}

func asyncQuestionPopulationFilters(conditions []string, args []any, opts models.ListAsyncQuestionsOptions) ([]string, []any) {
	if opts.SessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, opts.SessionID)
	}
	if opts.AuthorSessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, opts.AuthorSessionID)
	}
	if opts.ParentTaskID != "" {
		conditions = append(conditions, "parent_task_id = ?")
		args = append(args, opts.ParentTaskID)
	}
	return conditions, args
}

func asyncQuestionPlaceholders(count int) ([]string, []any) {
	placeholders := make([]string, count)
	args := make([]any, count)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return placeholders, args
}

// NextHiddenAsyncQuestionSnooze returns the earliest still-active snooze among
// hidden async questions in one workspace.
func (r *Repository) NextHiddenAsyncQuestionSnooze(ctx context.Context, workspaceID, userID string, now time.Time) (*time.Time, error) {
	query := `SELECT MIN(s.snooze_until)
		FROM clarification_inbox_sidecar s
		JOIN task_async_questions q ON s.pending_id = 'async:' || q.id
		WHERE s.user_id = ? AND s.state = 'snoozed' AND s.snooze_until > ?
		  AND q.workspace_id = ? AND q.status = 'pending'`
	var next time.Time
	var err error
	if dialect.IsPostgres(r.db.DriverName()) {
		var value sql.NullTime
		err = r.db.QueryRowxContext(ctx, r.db.Rebind(query), userID, now, workspaceID).Scan(&value)
		if err == nil && value.Valid {
			next = value.Time
			return &next, nil
		}
	} else {
		var value sql.NullString
		err = r.db.QueryRowxContext(ctx, r.db.Rebind(query), userID, now, workspaceID).Scan(&value)
		if err == nil && value.Valid && value.String != "" {
			next = parseLegacyTimestamp(value.String)
			return &next, nil
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return nil, err
}

// ResolveAsyncQuestion chooses the one terminal outcome. The returned bool is
// true only for the call that won; a losing call receives the winner without
// another delivery. Deliveries supplied by the caller are inserted in the same
// transaction with the winning outcome version.

// lockAsyncQuestionScopeTx serializes the read-before-insert boundaries in the
// async lifecycle for PostgreSQL. SQLite already serializes the writer.
func (r *Repository) lockAsyncQuestionScopeTx(ctx context.Context, tx *sqlx.Tx, key string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "async-question:"+key)
	return err
}
