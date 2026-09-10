package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
)

// CreateBudgetPolicy creates a new budget policy. Its revision is always
// assigned 1, regardless of any value already set on policy — including a
// nonzero one presented for creation. The assigned value is written back
// onto policy so the caller (the create response) reports what the row
// actually holds rather than leaning on the column's DEFAULT 1.
func (r *Repository) CreateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	policy.CreatedAt = now
	policy.UpdatedAt = now
	policy.Revision = 1

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_budget_policies (
			id, workspace_id, scope_type, scope_id, limit_subcents, period,
			alert_threshold_pct, action_on_exceed, created_at, updated_at, revision
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), policy.ID, policy.WorkspaceID, policy.ScopeType, policy.ScopeID,
		policy.LimitSubcents, policy.Period, policy.AlertThresholdPct,
		policy.ActionOnExceed, policy.CreatedAt, policy.UpdatedAt, policy.Revision)
	return err
}

// GetBudgetPolicy returns a budget policy by ID.
func (r *Repository) GetBudgetPolicy(ctx context.Context, id string) (*models.BudgetPolicy, error) {
	var policy models.BudgetPolicy
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_budget_policies WHERE id = ?`), id).StructScan(&policy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("budget policy not found: %s", id)
	}
	return &policy, err
}

// ListBudgetPolicies returns all budget policies for a workspace, ordered by
// created_at with id as a tiebreak so evaluation order is total and
// reproducible.
func (r *Repository) ListBudgetPolicies(ctx context.Context, workspaceID string) ([]*models.BudgetPolicy, error) {
	var policies []*models.BudgetPolicy
	err := r.ro.SelectContext(ctx, &policies, r.ro.Rebind(
		`SELECT * FROM office_budget_policies WHERE workspace_id = ? ORDER BY created_at ASC, id ASC`), workspaceID)
	if err != nil {
		return nil, err
	}
	if policies == nil {
		policies = []*models.BudgetPolicy{}
	}
	return policies, nil
}

// UpdateBudgetPolicy updates an existing budget policy and discards that
// policy's claims in the same transaction, so either both apply or neither
// does.
func (r *Repository) UpdateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	policy.UpdatedAt = time.Now().UTC()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.updateBudgetPolicyTx(ctx, tx, policy); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w; rollback: %v", err, rollbackErr)
		}
		return err
	}
	return tx.Commit()
}

// updateBudgetPolicyTx discards the policy's claims, bumps its revision by
// exactly 1 in-SQL, and reads the new revision back onto policy, all in the
// caller's transaction. The bump is expressed as revision = revision + 1
// rather than a value computed from a prior read, so two concurrent updates
// can never write the same revision. If no row matches id — the policy was
// deleted between the caller's read and this write — the RETURNING
// read-back yields sql.ErrNoRows, which propagates as an error so the
// caller rolls the whole transaction back rather than committing an empty
// update.
func (r *Repository) updateBudgetPolicyTx(ctx context.Context, tx *sqlx.Tx, policy *models.BudgetPolicy) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM office_budget_claims WHERE policy_id = ?`), policy.ID); err != nil {
		return fmt.Errorf("discard claims: %w", err)
	}
	if r.failBudgetPolicyUpdateErr != nil {
		return r.failBudgetPolicyUpdateErr
	}
	err := tx.QueryRowContext(ctx, tx.Rebind(`
		UPDATE office_budget_policies SET
			scope_type = ?, scope_id = ?, limit_subcents = ?, period = ?,
			alert_threshold_pct = ?, action_on_exceed = ?, updated_at = ?,
			revision = revision + 1
		WHERE id = ?
		RETURNING revision
	`), policy.ScopeType, policy.ScopeID, policy.LimitSubcents, policy.Period,
		policy.AlertThresholdPct, policy.ActionOnExceed, policy.UpdatedAt, policy.ID,
	).Scan(&policy.Revision)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("update policy: not found: %s", policy.ID)
		}
		return fmt.Errorf("update policy: %w", err)
	}
	return nil
}

// DeleteBudgetPolicy deletes a budget policy by ID. Its claims are removed by
// the office_budget_claims foreign key's ON DELETE CASCADE, not here.
func (r *Repository) DeleteBudgetPolicy(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_budget_policies WHERE id = ?`), id)
	return err
}

// budgetClaimLevelExceeded and budgetClaimLevelAlert are the claim-level
// literals ClaimExceeded writes for its atomic pair. Level is otherwise
// caller-supplied data, not a package concept, but ClaimExceeded's exported
// signature does not take a level parameter, so its two writes need names
// of their own.
const (
	budgetClaimLevelExceeded = "exceeded"
	budgetClaimLevelAlert    = "alert"
)

// execRebinder is the subset of *sqlx.DB / *sqlx.Tx a fenced claim insert
// needs, so the same statement can run standalone or inside ClaimExceeded's
// transaction.
type execRebinder interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	Rebind(string) string
}

// validateClaimInput reports whether a claim attempt's primary-key
// components are all well-formed. Every failure here is a programming
// error, not a superseded snapshot, so the caller treats it as a
// claim-store error and fails open rather than as the silent miss a
// refused fence produces.
func validateClaimInput(policyID, periodKey, level string, revision int64) error {
	if revision <= 0 {
		return fmt.Errorf("claim: revision must be positive, got %d", revision)
	}
	if policyID == "" {
		return fmt.Errorf("claim: policy id must not be empty")
	}
	if periodKey == "" {
		return fmt.Errorf("claim: period key must not be empty")
	}
	if level == "" {
		return fmt.Errorf("claim: level must not be empty")
	}
	return nil
}

// claimInsertFenced inserts a claim row only if policyID still holds
// exactly revision at the moment the statement runs, checked as part of the
// same statement so no evaluation can observe a matching revision and then
// insert against a superseded one. A policy that no longer exists at all
// fails the same EXISTS check, which is how a deleted policy's claim
// attempt is refused without ever reaching the foreign-key constraint. The
// FK violation classifier is kept as a defensive fallback only: dropping it
// would let a genuine foreign-key error read as an ordinary miss.
func claimInsertFenced(ctx context.Context, ex execRebinder, policyID, periodKey, level string, revision int64) (bool, error) {
	res, err := ex.ExecContext(ctx, ex.Rebind(`
		INSERT INTO office_budget_claims (policy_id, period_key, level, revision, claimed_at)
		SELECT ?, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM office_budget_policies WHERE id = ? AND revision = ?)
		ON CONFLICT(policy_id, period_key, level, revision) DO NOTHING
	`), policyID, periodKey, level, revision, time.Now().UTC(), policyID, revision)
	if err != nil {
		if db.IsForeignKeyViolation(err) {
			return false, nil
		}
		return false, err
	}
	return rowsAffectedOne(res)
}

// claimInsertCompanion inserts the alert-level companion row for an
// already-won exceeded claim, keyed to the same revision with no fence of
// its own — its key already names the revision, so a row written against a
// revision that has since moved is inert rather than wrong. Any error,
// including a foreign-key violation from a policy deleted mid-transaction,
// is returned unmodified so ClaimExceeded rolls the whole pair back rather
// than committing a lone exceeded claim.
func claimInsertCompanion(ctx context.Context, tx *sqlx.Tx, policyID, periodKey string, revision int64) (bool, error) {
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO office_budget_claims (policy_id, period_key, level, revision, claimed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(policy_id, period_key, level, revision) DO NOTHING
	`), policyID, periodKey, budgetClaimLevelAlert, revision, time.Now().UTC())
	if err != nil {
		return false, err
	}
	return rowsAffectedOne(res)
}

func rowsAffectedOne(res sql.Result) (bool, error) {
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// Claim atomically records that (policyID, periodKey, level) may emit its
// budget notification, fenced to revision: the claim is recorded only while
// revision is still the policy's stored revision. claimed=true means this
// call won and the caller should emit; claimed=false with a nil error means
// an earlier evaluation already holds the claim, the referenced policy no
// longer exists, or the presented revision has been superseded — all three
// are the same ordinary miss.
func (r *Repository) Claim(ctx context.Context, policyID, periodKey, level string, revision int64) (bool, error) {
	if err := validateClaimInput(policyID, periodKey, level, revision); err != nil {
		return false, err
	}
	return claimInsertFenced(ctx, r.db, policyID, periodKey, level, revision)
}

// ClaimExceeded atomically records the exceeded-level claim and, only when
// that claim is won by this call, its alert-level companion claim, in one
// transaction: once it commits, either both rows exist for that policy,
// period and revision, or this call wrote neither. The fence is evaluated
// once, on the exceeded insert; the companion insert carries no fence of
// its own. Returns whether the exceeded-level row was inserted by this
// call — the companion's own outcome (won, or an ordinary conflict with an
// alert-level claim an earlier evaluation already holds) never changes it.
func (r *Repository) ClaimExceeded(ctx context.Context, policyID, periodKey string, revision int64) (bool, error) {
	if err := validateClaimInput(policyID, periodKey, budgetClaimLevelExceeded, revision); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	claimed, err := r.claimExceededTx(ctx, tx, policyID, periodKey, revision)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return false, fmt.Errorf("%w; rollback: %v", err, rollbackErr)
		}
		return false, err
	}
	if !claimed {
		return false, tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) claimExceededTx(
	ctx context.Context, tx *sqlx.Tx, policyID, periodKey string, revision int64,
) (bool, error) {
	claimed, err := claimInsertFenced(ctx, tx, policyID, periodKey, budgetClaimLevelExceeded, revision)
	if err != nil {
		return false, err
	}
	if !claimed {
		// Refused by the fence, or conflicting with an exceeded claim
		// already held: no emission to protect, so the companion insert
		// must not be attempted.
		return false, nil
	}
	if r.failBudgetExceededCompanionErr != nil {
		return false, r.failBudgetExceededCompanionErr
	}
	if _, err := claimInsertCompanion(ctx, tx, policyID, periodKey, revision); err != nil {
		return false, err
	}
	return true, nil
}

// recreateBudgetClaimsForRevision takes effect at most once per database:
// it probes whether office_budget_claims already declares revision and does
// nothing when it does, which is every boot after the first on a fresh
// database (createCostTables already declares the final shape there) and
// every boot after the first successful recreate on an upgraded one. On a
// database still carrying the pre-revision three-column table, it drops and
// recreates office_budget_claims with the four-column primary key the
// current schema requires, as one transaction: DROP and CREATE either both
// commit or neither does, so a boot interrupted between them leaves the
// table exactly as it was rather than absent, and a retried boot always
// finds the table in a state the probe can classify. It is not expressed as
// an r.migrate.Apply statement: that runner re-executes its SQL on every
// boot and is idempotent only because an already-exists error is
// swallowed, which would destroy claim state on every boot here. It
// returns its error to its caller rather than swallowing it, so a failed
// recreate is visible rather than silently leaving the old three-column
// key in place.
func (r *Repository) recreateBudgetClaimsForRevision() error {
	exists, err := columnExists(r.db, "office_budget_claims", "revision")
	if err != nil {
		return fmt.Errorf("probe office_budget_claims.revision: %w", err)
	}
	if exists {
		return nil
	}
	if r.failBudgetClaimsRecreateErr != nil {
		return r.failBudgetClaimsRecreateErr
	}
	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin office_budget_claims recreate: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS office_budget_claims`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("drop office_budget_claims: %w", err)
	}
	if r.failBudgetClaimsRecreateAfterDropErr != nil {
		_ = tx.Rollback()
		return r.failBudgetClaimsRecreateAfterDropErr
	}
	if _, err := tx.Exec(budgetClaimsDDL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("recreate office_budget_claims: %w", err)
	}
	return tx.Commit()
}
