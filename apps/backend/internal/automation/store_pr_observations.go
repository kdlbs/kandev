package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// prObservationTablesSQL stores, per github_pr trigger and open pull request,
// what the evaluator last processed: the head SHA, the requested reviewers,
// the review-request generation, a review-request generation reserved before
// dispatch, and a pending quiet-period head.
const prObservationTablesSQL = `
	CREATE TABLE IF NOT EXISTS automation_pr_observations (
		trigger_id TEXT NOT NULL REFERENCES automation_triggers(id) ON DELETE CASCADE,
		repo_owner TEXT NOT NULL,
		repo_name TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		head_sha TEXT NOT NULL,
		requested TEXT NOT NULL DEFAULT '[]',
		review_generation BIGINT NOT NULL DEFAULT 0,
		pending_review_generation BIGINT NOT NULL DEFAULT 0,
		pending_review_reviewers TEXT NOT NULL DEFAULT '[]',
		pending_head_sha TEXT NOT NULL DEFAULT '',
		pending_since {{timestamp}},
		tracked_events TEXT NOT NULL DEFAULT '',
		updated_at {{timestamp}} NOT NULL,
		PRIMARY KEY (trigger_id, repo_owner, repo_name, pr_number)
	);
`

// PRObservation is the evaluator's processed state for one trigger and one
// open pull request.
type PRObservation struct {
	TriggerID        string
	RepoOwner        string
	RepoName         string
	PRNumber         int
	HeadSHA          string
	Requested        []string
	ReviewGeneration int64
	// PendingReviewGeneration is the generation reserved for a review request
	// before its run was dispatched. A retry of the same request reuses it, so
	// a run committed without its observation advancing is never dispatched
	// again under a new key.
	PendingReviewGeneration int64
	// PendingReviewReviewers are the reviewer keys the reservation covers. A
	// retry reuses the reserved generation only for these reviewers.
	PendingReviewReviewers []string
	PendingHeadSHA         string
	PendingSince           *time.Time
	// TrackedEvents lists the follow-up events this observation was recorded
	// for, so an event added later starts from its own baseline.
	TrackedEvents string
}

type prObservationRow struct {
	TriggerID        string       `db:"trigger_id"`
	RepoOwner        string       `db:"repo_owner"`
	RepoName         string       `db:"repo_name"`
	PRNumber         int          `db:"pr_number"`
	HeadSHA          string       `db:"head_sha"`
	Requested        string       `db:"requested"`
	ReviewGeneration int64        `db:"review_generation"`
	PendingReviewGen int64        `db:"pending_review_generation"`
	PendingReviewers string       `db:"pending_review_reviewers"`
	PendingHeadSHA   string       `db:"pending_head_sha"`
	PendingSince     sql.NullTime `db:"pending_since"`
	TrackedEvents    string       `db:"tracked_events"`
}

func (r prObservationRow) toObservation() (*PRObservation, error) {
	obs := &PRObservation{
		TriggerID: r.TriggerID, RepoOwner: r.RepoOwner, RepoName: r.RepoName, PRNumber: r.PRNumber,
		HeadSHA: r.HeadSHA, ReviewGeneration: r.ReviewGeneration, PendingReviewGeneration: r.PendingReviewGen,
		PendingHeadSHA: r.PendingHeadSHA,
		TrackedEvents:  r.TrackedEvents,
	}
	if r.PendingSince.Valid {
		since := r.PendingSince.Time.UTC()
		obs.PendingSince = &since
	}
	if err := json.Unmarshal([]byte(r.Requested), &obs.Requested); err != nil {
		return nil, fmt.Errorf("decode requested reviewers: %w", err)
	}
	if err := json.Unmarshal([]byte(r.PendingReviewers), &obs.PendingReviewReviewers); err != nil {
		return nil, fmt.Errorf("decode reserved reviewers: %w", err)
	}
	return obs, nil
}

// ListPRObservations returns every stored observation for one trigger and
// repository, keyed by pull request number.
func (s *Store) ListPRObservations(ctx context.Context, triggerID, owner, repo string) (map[int]*PRObservation, error) {
	var rows []prObservationRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT trigger_id, repo_owner, repo_name, pr_number, head_sha, requested,
			review_generation, pending_review_generation, pending_review_reviewers,
			pending_head_sha, pending_since, tracked_events
		FROM automation_pr_observations
		WHERE trigger_id = ? AND repo_owner = ? AND repo_name = ?`), triggerID, owner, repo)
	if err != nil {
		return nil, err
	}
	result := make(map[int]*PRObservation, len(rows))
	for _, row := range rows {
		obs, err := row.toObservation()
		if err != nil {
			return nil, err
		}
		result[obs.PRNumber] = obs
	}
	return result, nil
}

// UpsertPRObservation stores the observation, replacing any previous row for
// the same trigger and pull request.
func (s *Store) UpsertPRObservation(ctx context.Context, obs *PRObservation) error {
	if obs == nil {
		return errors.New("pr observation is required")
	}
	encoded, err := encodeReviewerKeys(obs.Requested)
	if err != nil {
		return err
	}
	reserved, err := encodeReviewerKeys(obs.PendingReviewReviewers)
	if err != nil {
		return err
	}
	var pendingSince interface{}
	if obs.PendingSince != nil {
		pendingSince = obs.PendingSince.UTC()
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO automation_pr_observations (
			trigger_id, repo_owner, repo_name, pr_number, head_sha, requested,
			review_generation, pending_review_generation, pending_review_reviewers,
			pending_head_sha, pending_since, tracked_events, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id, repo_owner, repo_name, pr_number) DO UPDATE SET
			head_sha = excluded.head_sha,
			requested = excluded.requested,
			review_generation = excluded.review_generation,
			pending_review_generation = excluded.pending_review_generation,
			pending_review_reviewers = excluded.pending_review_reviewers,
			pending_head_sha = excluded.pending_head_sha,
			pending_since = excluded.pending_since,
			tracked_events = excluded.tracked_events,
			updated_at = excluded.updated_at`),
		obs.TriggerID, obs.RepoOwner, obs.RepoName, obs.PRNumber, obs.HeadSHA, encoded,
		obs.ReviewGeneration, obs.PendingReviewGeneration, reserved, obs.PendingHeadSHA, pendingSince, obs.TrackedEvents, time.Now().UTC())
	return err
}

// encodeReviewerKeys stores reviewer keys as a sorted JSON list.
func encodeReviewerKeys(keys []string) (string, error) {
	sorted := append([]string{}, keys...)
	sort.Strings(sorted)
	encoded, err := json.Marshal(sorted)
	return string(encoded), err
}

// DeletePRObservation removes one observation.
func (s *Store) DeletePRObservation(ctx context.Context, triggerID, owner, repo string, number int) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM automation_pr_observations
		WHERE trigger_id = ? AND repo_owner = ? AND repo_name = ? AND pr_number = ?`),
		triggerID, owner, repo, number)
	return err
}

// deletePRObservationsForTrigger removes every observation of a trigger. The
// foreign key cascades on PostgreSQL; SQLite connections may run without
// foreign-key enforcement, so trigger deletion clears them explicitly.
func (s *Store) deletePRObservationsForTrigger(ctx context.Context, triggerID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM automation_pr_observations WHERE trigger_id = ?`), triggerID)
	return err
}

// MaxReviewRequestGeneration returns the highest generation already used in
// an automation's review-request dedup keys for one PR, so a new baseline
// (after a reopen or a recreated trigger) never reuses a held key.
func (s *Store) MaxReviewRequestGeneration(ctx context.Context, automationID, prefix string) (int64, error) {
	var keys []string
	err := s.ro.SelectContext(ctx, &keys, s.ro.Rebind(`
		SELECT dedup_key FROM automation_runs
		WHERE automation_id = ? AND SUBSTR(dedup_key, 1, ?) = ?`),
		automationID, len(prefix), prefix)
	if err != nil {
		return 0, err
	}
	var highest int64
	for _, key := range keys {
		generation, err := strconv.ParseInt(strings.TrimPrefix(key, prefix), 10, 64)
		if err == nil && generation > highest {
			highest = generation
		}
	}
	return highest, nil
}
