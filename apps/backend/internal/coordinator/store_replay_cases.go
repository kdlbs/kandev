package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

// ReplayCases reads the recorded past for the replay harness over the
// coordinator's reader pool.
type ReplayCases struct{ store *Store }

// NewReplayCases returns the replay harness's case reader.
func NewReplayCases(s *Store) *ReplayCases { return &ReplayCases{store: s} }

var _ replay.Cases = (*ReplayCases)(nil)

const replayDecidedList = "('approved','edited','rejected','returned','undone')"

type replayTurnRow struct {
	ID           string `db:"id"`
	Trigger      string `db:"trigger"`
	WakeKinds    string `db:"wake_kinds"`
	SnapshotHash string `db:"snapshot_hash"`
}

type replayOutcomeRow struct {
	TurnID       string `db:"turn_id"`
	ProposalID   string `db:"proposal_id"`
	Decision     string `db:"decision"`
	Automatic    bool   `db:"automatic"`
	EditedFields string `db:"edited_fields"`
}

// SelectTurns returns group one (the newest turns with a decided outcome) then
// group two (the newest turns in the 90 days before now holding a feedback row
// and a decided outcome, not in group one), each at most FirstGroupSize and
// SecondGroupSize long, with their outcome rows.
func (c *ReplayCases) SelectTurns(ctx context.Context, coordinatorID string, now time.Time) ([]replay.Turn, error) {
	s := c.store
	var one, two []replayTurnRow
	if err := s.ro.SelectContext(ctx, &one, s.ro.Rebind(`
		SELECT t.id, t."trigger", t.wake_kinds, t.snapshot_hash FROM coordinator_turns t
		WHERE t.coordinator_id = ? AND EXISTS (SELECT 1 FROM coordinator_outcomes o WHERE o.turn_id = t.id AND o.decision IN `+replayDecidedList+`)
		ORDER BY t.started_at DESC, t.id DESC LIMIT ?`), coordinatorID, replay.FirstGroupSize); err != nil {
		return nil, fmt.Errorf("select replay turns: %w", err)
	}
	if err := s.ro.SelectContext(ctx, &two, s.ro.Rebind(`
		SELECT t.id, t."trigger", t.wake_kinds, t.snapshot_hash FROM coordinator_turns t
		WHERE t.coordinator_id = ? AND t.started_at >= ? AND t.started_at <= ?
		  AND EXISTS (SELECT 1 FROM coordinator_feedback f WHERE f.turn_id = t.id)
		  AND EXISTS (SELECT 1 FROM coordinator_outcomes o WHERE o.turn_id = t.id AND o.decision IN `+replayDecidedList+`)
		ORDER BY t.started_at DESC, t.id DESC LIMIT ?`),
		coordinatorID, now.UTC().Add(-replay.OverrideHorizon), now.UTC(), replay.FirstGroupSize+replay.SecondGroupSize); err != nil {
		return nil, fmt.Errorf("select replay override turns: %w", err)
	}
	seen := map[string]bool{}
	var rows []replayTurnRow
	for _, r := range one {
		seen[r.ID] = true
		rows = append(rows, r)
	}
	added := 0
	for _, r := range two {
		if seen[r.ID] || added == replay.SecondGroupSize {
			continue
		}
		added++
		rows = append(rows, r)
	}
	return c.withOutcomes(ctx, rows)
}

func (c *ReplayCases) withOutcomes(ctx context.Context, rows []replayTurnRow) ([]replay.Turn, error) {
	out := make([]replay.Turn, 0, len(rows))
	for _, r := range rows {
		var kinds []string
		if err := json.Unmarshal([]byte(r.WakeKinds), &kinds); err != nil {
			return nil, fmt.Errorf("decode wake kinds of %s: %w", r.ID, err)
		}
		var outs []replayOutcomeRow
		if err := c.store.ro.SelectContext(ctx, &outs, c.store.ro.Rebind(`
			SELECT turn_id, proposal_id, decision, automatic, edited_fields FROM coordinator_outcomes
			WHERE turn_id = ? ORDER BY proposal_id`), r.ID); err != nil {
			return nil, fmt.Errorf("read outcomes of %s: %w", r.ID, err)
		}
		turn := replay.Turn{ID: r.ID, Trigger: r.Trigger, WakeKinds: kinds, SnapshotHash: r.SnapshotHash}
		for _, o := range outs {
			var edited []string
			if err := json.Unmarshal([]byte(o.EditedFields), &edited); err != nil {
				return nil, fmt.Errorf("decode edited fields of %s: %w", o.ProposalID, err)
			}
			turn.Outcomes = append(turn.Outcomes, replay.Outcome{ProposalID: o.ProposalID, Decision: o.Decision, Automatic: o.Automatic, EditedFields: edited})
		}
		out = append(out, turn)
	}
	return out, nil
}

// Proposal returns the proposal as the coordinator made it, from spec_json.
func (c *ReplayCases) Proposal(ctx context.Context, proposalID string) (replay.Proposal, error) {
	var row struct {
		Kind         string         `db:"kind"`
		TargetTaskID sql.NullString `db:"target_task_id"`
		SpecJSON     string         `db:"spec_json"`
	}
	err := c.store.ro.GetContext(ctx, &row, c.store.ro.Rebind(`SELECT kind, target_task_id, spec_json FROM coordinator_proposals WHERE id = ?`), proposalID)
	if err != nil {
		return replay.Proposal{}, notFound(err, "read proposal")
	}
	var spec ProposalSpec
	if row.SpecJSON != "" {
		if err := json.Unmarshal([]byte(row.SpecJSON), &spec); err != nil {
			return replay.Proposal{}, fmt.Errorf("decode proposal spec %s: %w", proposalID, err)
		}
	}
	return replay.Proposal{Kind: row.Kind, TargetTaskID: row.TargetTaskID.String, WorkflowID: spec.WorkflowID, Title: spec.Title}, nil
}

// Snapshot returns the stored snapshot body for the hash.
func (c *ReplayCases) Snapshot(ctx context.Context, hash string) (string, error) {
	var body string
	err := c.store.ro.GetContext(ctx, &body, c.store.ro.Rebind(`SELECT body FROM coordinator_turn_snapshots WHERE hash = ?`), hash)
	if err != nil {
		return "", notFound(err, "read snapshot")
	}
	return body, nil
}

// TriggerText is the earliest user-authored message of the task session turn
// the ledger turn names, or the empty text of a wake turn.
func (c *ReplayCases) TriggerText(ctx context.Context, turn replay.Turn) (string, error) {
	var body string
	err := c.store.ro.GetContext(ctx, &body, c.store.ro.Rebind(`
		SELECT m.content FROM task_session_messages m
		JOIN coordinator_turns t ON t.session_turn_id = m.turn_id AND t.session_id = m.task_session_id
		WHERE t.id = ? AND m.author_type = 'user'
		ORDER BY m.created_at, m.id LIMIT 1`), turn.ID)
	if err != nil {
		return "", notFound(err, "read trigger message")
	}
	return body, nil
}

// TaskTitle returns the task's current title.
func (c *ReplayCases) TaskTitle(ctx context.Context, taskID string) (string, error) {
	var title string
	err := c.store.ro.GetContext(ctx, &title, c.store.ro.Rebind(`SELECT title FROM tasks WHERE id = ?`), taskID)
	if err != nil {
		return "", notFound(err, "read task title")
	}
	return title, nil
}

// notFound maps a missing row to replay.ErrNotFound and wraps any other
// read error.
func notFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return replay.ErrNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}
