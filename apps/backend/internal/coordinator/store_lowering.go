package coordinator

import (
	"context"
	"fmt"
	"time"
)

// classProposalFilter is the proposal-side predicate of a class's evidence
// rows: a resume proposal for resume, an agent-starting one for start_agent.
func classProposalFilter(class Action) string {
	if class == ActionStartAgent {
		return `p.starts_agent = TRUE`
	}
	return `p.kind = '` + string(class) + `'`
}

// AutomaticFailureAfter reports whether an automatic attempt for class settled
// failed, dated by its proposal's automatic_at strictly after after.
func (s *Store) AutomaticFailureAfter(ctx context.Context, coordinatorID string, class Action, after time.Time) (bool, error) {
	return s.existsEvidence(ctx, "automatic failure", `
		SELECT COUNT(*) FROM coordinator_activity a JOIN coordinator_proposals p ON p.id = a.proposal_id
		WHERE a.coordinator_id = ? AND a.outcome = 'failed' AND a."authorization" = 'automatic'
		AND p.automatic_at > ? AND `+classProposalFilter(class), coordinatorID, after.UTC())
}

// ManagerOverrideAfter reports whether a manager overrode a proposal that
// needed class: a reject, a return, or an approval or failure with an edited
// spec, dated by the decided row's created_at strictly after after.
func (s *Store) ManagerOverrideAfter(ctx context.Context, coordinatorID string, class Action, after time.Time) (bool, error) {
	return s.existsEvidence(ctx, "manager override", `
		SELECT COUNT(*) FROM coordinator_activity a JOIN coordinator_proposals p ON p.id = a.proposal_id
		WHERE a.coordinator_id = ? AND a.created_at > ? AND a."authorization" <> 'automatic'
		AND (a.outcome IN ('rejected', 'returned') OR (a.edited = TRUE AND a.outcome IN ('approved', 'failed')))
		AND `+classProposalFilter(class), coordinatorID, after.UTC())
}

// AutomaticStartAgentTaskIDs returns the task ids of the coordinator's
// agent-starting proposals whose current claim is the automatic path's.
func (s *Store) AutomaticStartAgentTaskIDs(ctx context.Context, coordinatorID string) (map[string]struct{}, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT task_id FROM coordinator_proposals
		WHERE coordinator_id = ? AND claimed_automatically = 1 AND starts_agent = TRUE AND task_id IS NOT NULL`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("read automatic agent-starting tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan automatic agent-starting task: %w", err)
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) existsEvidence(ctx context.Context, what, query string, args ...any) (bool, error) {
	var n int
	if err := s.ro.QueryRowContext(ctx, s.ro.Rebind(query), args...).Scan(&n); err != nil {
		return false, fmt.Errorf("read %s: %w", what, err)
	}
	return n > 0, nil
}

// CoordinatorIDsWithRaisedClasses returns the ids of coordinators with any
// class change whose new value is automatic.
func (s *Store) CoordinatorIDsWithRaisedClasses(ctx context.Context) ([]string, error) {
	return s.queryIDs(ctx, "coordinators with raised classes", `
		SELECT DISTINCT coordinator_id FROM coordinator_class_changes WHERE to_value = 'automatic' ORDER BY coordinator_id`)
}

// ProposalDecidedAt returns the created_at of the newest activity row of the
// proposal.
func (s *Store) ProposalDecidedAt(ctx context.Context, proposalID string) (time.Time, error) {
	var at time.Time
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT created_at FROM coordinator_activity
		WHERE proposal_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`), proposalID).Scan(&at)
	if err != nil {
		return time.Time{}, fmt.Errorf("read decision time: %w", err)
	}
	return at.UTC(), nil
}
