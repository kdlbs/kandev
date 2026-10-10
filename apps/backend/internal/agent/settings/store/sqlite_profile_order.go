package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/db/dialect"
)

// AgentProfileOrderSnapshot keeps rows and their order revision from one database snapshot.
type AgentProfileOrderSnapshot struct {
	Profiles []*models.AgentProfile
	Revision int64
}

// lockAgentProfileMembership serializes writes that can change global profile membership or order.
func lockAgentProfileMemberships(ctx context.Context, tx *sqlx.Tx, driver string, agentIDs []string) error {
	ordered := append([]string(nil), agentIDs...)
	sort.Strings(ordered)
	last := ""
	for _, agentID := range ordered {
		if agentID == "" || agentID == last {
			continue
		}
		last = agentID
		if dialect.IsPostgres(driver) {
			if _, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`), "agent-profile-order:"+agentID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agents SET id = id WHERE id = ?`), agentID); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) lockMembership(ctx context.Context, tx *sqlx.Tx, operation string, agentIDs ...string) error {
	if err := lockAgentProfileMemberships(ctx, tx, r.db.DriverName(), agentIDs); err != nil {
		return err
	}
	if r.profileOrderAfterLock == nil {
		return nil
	}
	ordered := append([]string(nil), agentIDs...)
	sort.Strings(ordered)
	last := ""
	for _, agentID := range ordered {
		if agentID == "" || agentID == last {
			continue
		}
		last = agentID
		if err := r.profileOrderAfterLock(operation, agentID); err != nil {
			return err
		}
	}
	return nil
}

func lockAgentProfileIdentity(ctx context.Context, tx *sqlx.Tx, driver, profileID string) error {
	if !dialect.IsPostgres(driver) {
		return nil
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`), "agent-profile-membership:"+profileID)
	return err
}

func (r *sqliteRepository) updateAgentProfileWithMembershipLocks(
	ctx context.Context,
	profile *models.AgentProfile,
	enabled *bool,
	updateExtra func(*sqlx.Tx) error,
) error {
	for range 3 {
		observed, err := r.GetAgentProfile(ctx, profile.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("agent profile not found: %s", profile.ID)
		}
		if err != nil {
			return err
		}
		if r.profileOrderAfterOwnershipRead != nil {
			if err := r.profileOrderAfterOwnershipRead(profile.ID, observed.AgentID, observed.WorkspaceID); err != nil {
				return err
			}
		}
		tx, err := r.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		if err := r.lockProfileUpdate(ctx, tx, profile, observed); err != nil {
			_ = tx.Rollback()
			return err
		}
		actualAgentID, actualWorkspaceID, err := readProfileOwnership(ctx, tx, profile.ID)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if actualAgentID != observed.AgentID || actualWorkspaceID != observed.WorkspaceID {
			_ = tx.Rollback()
			continue
		}
		if err := resetProfileOrderOnMembershipMove(ctx, tx, profile, actualAgentID, actualWorkspaceID); err != nil {
			_ = tx.Rollback()
			return err
		}
		committedEnabled, err := r.updateAgentProfileWithExtra(ctx, tx, profile, enabled, updateExtra)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		profile.Enabled = committedEnabled
		return nil
	}
	return ErrProfileChanged
}

func (r *sqliteRepository) updateAgentProfileWithExtra(
	ctx context.Context,
	tx *sqlx.Tx,
	profile *models.AgentProfile,
	enabled *bool,
	updateExtra func(*sqlx.Tx) error,
) (bool, error) {
	committedEnabled, err := r.updateAgentProfile(ctx, tx, profile, enabled)
	if err == nil && updateExtra != nil {
		err = updateExtra(tx)
	}
	return committedEnabled, err
}

func resetProfileOrderOnMembershipMove(
	ctx context.Context,
	tx *sqlx.Tx,
	profile *models.AgentProfile,
	agentID, workspaceID string,
) error {
	if agentID == profile.AgentID && workspaceID == profile.WorkspaceID {
		return nil
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_profiles SET sort_order = 0 WHERE id = ?`), profile.ID)
	return err
}

func (r *sqliteRepository) lockProfileUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	profile, observed *models.AgentProfile,
) error {
	agentIDs := make([]string, 0, 2)
	if profile.WorkspaceID == "" {
		agentIDs = append(agentIDs, profile.AgentID)
	}
	if observed.WorkspaceID == "" {
		agentIDs = append(agentIDs, observed.AgentID)
	}
	if err := r.lockMembership(ctx, tx, "update-profile", agentIDs...); err != nil {
		return err
	}
	return lockAgentProfileIdentity(ctx, tx, r.db.DriverName(), profile.ID)
}

func readProfileOwnership(ctx context.Context, tx *sqlx.Tx, profileID string) (string, string, error) {
	var agentID, workspaceID string
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT agent_id, workspace_id FROM agent_profiles WHERE id = ? AND deleted_at IS NULL`), profileID).Scan(&agentID, &workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("agent profile not found: %s", profileID)
	}
	return agentID, workspaceID, err
}

func ensureAgentProfileOrderAgentExists(ctx context.Context, tx *sqlx.Tx, agentID string) error {
	var exists int
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT 1 FROM agents WHERE id = ?`), agentID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProfileOrderAgentNotFound
	}
	return err
}

func (r *sqliteRepository) ReorderAgentProfiles(ctx context.Context, agentID string, orderedIDs []string) (int64, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.lockMembership(ctx, tx, "reorder", agentID); err != nil {
		return 0, false, fmt.Errorf("lock agent profile order: %w", err)
	}
	if err := ensureAgentProfileOrderAgentExists(ctx, tx, agentID); err != nil {
		return 0, false, err
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT id FROM agent_profiles WHERE agent_id = ? AND workspace_id = '' AND deleted_at IS NULL ORDER BY sort_order ASC, created_at DESC, id ASC`), agentID)
	if err != nil {
		return 0, false, err
	}
	current := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, false, err
		}
		current = append(current, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, false, err
	}
	_ = rows.Close()
	if !sameStringSet(current, orderedIDs) {
		return 0, false, ErrProfileOrderSetMismatch
	}
	changed := !sameStringOrder(current, orderedIDs)
	if !changed {
		revision, err := readProfileOrderRevision(ctx, tx, agentID)
		return revision, false, err
	}
	for index, id := range orderedIDs {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_profiles SET sort_order = ? WHERE id = ? AND agent_id = ? AND workspace_id = '' AND deleted_at IS NULL`), index+1, id, agentID); err != nil {
			return 0, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_profile_orders (agent_id, revision) VALUES (?, 1) ON CONFLICT(agent_id) DO UPDATE SET revision = agent_profile_orders.revision + 1`), agentID); err != nil {
		return 0, false, err
	}
	revision, err := readProfileOrderRevision(ctx, tx, agentID)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return revision, true, nil
}

// GetAgentProfileOrderSnapshots reads each requested list and revision consistently.
func (r *sqliteRepository) GetAgentProfileOrderSnapshots(ctx context.Context, agentIDs []string) (map[string]AgentProfileOrderSnapshot, error) {
	return r.getAgentProfileOrderSnapshots(ctx, agentIDs, nil)
}

func (r *sqliteRepository) getAgentProfileOrderSnapshots(ctx context.Context, agentIDs []string, afterProfiles func() error) (map[string]AgentProfileOrderSnapshot, error) {
	options := &sql.TxOptions{ReadOnly: true}
	if dialect.IsPostgres(r.ro.DriverName()) {
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, err := r.ro.BeginTxx(ctx, options)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result := make(map[string]AgentProfileOrderSnapshot, len(agentIDs))
	for _, agentID := range agentIDs {
		profiles, err := listProfilesInTx(ctx, tx, agentID)
		if err != nil {
			return nil, err
		}
		if afterProfiles != nil {
			if err := afterProfiles(); err != nil {
				return nil, err
			}
		}
		revision, err := readProfileOrderRevision(ctx, tx, agentID)
		if err != nil {
			return nil, err
		}
		result[agentID] = AgentProfileOrderSnapshot{Profiles: profiles, Revision: revision}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func listProfilesInTx(ctx context.Context, tx *sqlx.Tx, agentID string) ([]*models.AgentProfile, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(agentProfileSelectColumns+` WHERE agent_id = ? AND deleted_at IS NULL ORDER BY sort_order ASC, created_at DESC, id ASC`), agentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	profiles := make([]*models.AgentProfile, 0)
	for rows.Next() {
		profile, err := scanAgentProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func readProfileOrderRevision(ctx context.Context, execer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
	Rebind(string) string
}, agentID string) (int64, error) {
	var revision int64
	err := execer.QueryRowxContext(ctx, execer.Rebind(`SELECT revision FROM agent_profile_orders WHERE agent_id = ?`), agentID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		if _, duplicate := leftSet[value]; duplicate {
			return false
		}
		leftSet[value] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		if _, duplicate := rightSet[value]; duplicate {
			return false
		}
		rightSet[value] = struct{}{}
		if _, ok := leftSet[value]; !ok {
			return false
		}
	}
	return true
}
func sameStringOrder(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
