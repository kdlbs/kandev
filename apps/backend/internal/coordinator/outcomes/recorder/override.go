package recorder

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

// Feedback kinds stored in coordinator_feedback.
const (
	FeedbackRejected  = "rejected"
	FeedbackEdited    = "edited"
	FeedbackUndone    = "undone"
	FeedbackMovedBack = "moved_back"
)

// Verdict classifies the actor of an override.
type Verdict int

// Verdicts of an ActorChecker; only VerdictManager records an observation.
const (
	VerdictManager Verdict = iota
	VerdictNotManager
	VerdictPrincipal
	VerdictSystem
)

// ActorChecker decides whether actorID holds workspace.manage in workspaceID,
// the check the automatic approval class uses for its raiser. While
// authentication is disabled the default user and an empty actor are managers.
type ActorChecker interface {
	Check(ctx context.Context, workspaceID, actorID string) (Verdict, error)
}

// Capture stores manager overrides of coordinator decisions as feedback rows.
type Capture struct {
	db      *sqlx.DB
	checker ActorChecker
	log     *zap.Logger
	now     func() time.Time
}

// NewCapture builds a capture writing through db.
func NewCapture(db *sqlx.DB, checker ActorChecker, log *zap.Logger, now func() time.Time) *Capture {
	if log == nil {
		log = zap.NewNop()
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Capture{db: db, checker: checker, log: log, now: now}
}

// OnDecision implements coordinator.DecisionObserver for rejected, edited and
// undone decisions. The hook runs once: a failed authorisation read stores
// nothing and is counted.
func (c *Capture) OnDecision(ctx context.Context, ev coordinator.DecisionEvent) {
	kind := overrideKind(ev)
	if kind == "" {
		return
	}
	if !c.judgeActor(ctx, ev.WorkspaceID, ev.ActorUserID) {
		return
	}
	at := ev.At
	if at.IsZero() {
		at = c.now()
	}
	reason := outcomes.ReasonNone
	if kind == FeedbackRejected {
		reason = outcomes.Code(ev.ReasonCode, "")
	}
	var turn sql.NullString
	if err := c.db.GetContext(ctx, &turn, c.db.Rebind(`SELECT turn_id FROM coordinator_proposals WHERE id = ?`), ev.ProposalID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.log.Warn("coordinator override: turn read failed", zap.Error(err))
	}
	if _, err := c.insertFeedback(ctx, c.db, feedbackRow{
		coordinatorID: ev.CoordinatorID, kind: kind, proposalID: ev.ProposalID, turnID: turn, userID: ev.ActorUserID,
		reasonCode: reason, proposalKind: ev.ProposalKind, createdAt: at,
	}); err != nil {
		c.log.Warn("coordinator override: insert failed", zap.Error(err))
	}
}

func overrideKind(ev coordinator.DecisionEvent) string {
	switch ev.Decision {
	case outcomes.DecisionRejected:
		return FeedbackRejected
	case outcomes.DecisionUndone:
		return FeedbackUndone
	case outcomes.DecisionApproved, outcomes.DecisionEdited:
		if !ev.Automatic && len(ev.EditedFields) > 0 {
			return FeedbackEdited
		}
	}
	return ""
}

// judgeActor runs the manager check and counts every refusal; it reports
// whether the actor is a manager.
func (c *Capture) judgeActor(ctx context.Context, workspaceID, actorID string) bool {
	verdict, err := c.checker.Check(ctx, workspaceID, actorID)
	if err != nil {
		bump(ignoredTotal, IgnoredAuthzError)
		return false
	}
	if reason := verdictReason(verdict); reason != "" {
		bump(ignoredTotal, reason)
		return false
	}
	return true
}

func verdictReason(v Verdict) string {
	switch v {
	case VerdictManager:
		return ""
	case VerdictPrincipal:
		return IgnoredPrincipal
	case VerdictSystem:
		return IgnoredSystem
	default:
		return IgnoredNotManager
	}
}

type feedbackRow struct {
	coordinatorID, kind, proposalID string
	turnID                          sql.NullString
	userID, reasonCode              string
	proposalKind, toStepID, key     string
	createdAt                       time.Time
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertFeedback stores one observation; the unique index makes a second
// delivery a no-op. It reports whether a row was inserted.
func (c *Capture) insertFeedback(ctx context.Context, x execer, f feedbackRow) (bool, error) {
	res, err := x.ExecContext(ctx, c.db.Rebind(`
		INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, turn_id, user_id, reason_code, proposal_kind, to_step_id, transition_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (proposal_id, kind, transition_key) DO NOTHING`),
		uuid.NewString(), f.coordinatorID, f.kind, f.proposalID, f.turnID, f.userID, f.reasonCode, f.proposalKind, f.toStepID, f.key, f.createdAt.UTC())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
