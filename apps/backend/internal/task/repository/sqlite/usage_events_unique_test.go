package sqlite

// Detector-level coverage for isUsageEventUniqueViolation (AC-32): it must
// fire only for uniq_task_usage_events_usage_event_id, not for the table's
// primary key or its foreign keys, so a non-duplicate insert failure is
// never silently swallowed as "expected" duplicate behaviour.

import (
	"testing"
	"time"
)

func TestIsUsageEventUniqueViolation_Nil(t *testing.T) {
	if isUsageEventUniqueViolation(nil) {
		t.Error("isUsageEventUniqueViolation(nil) = true, want false")
	}
}

func TestIsUsageEventUniqueViolation_SQLiteDuplicateUsageEventID(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-1")
	insertUsageEventRow(t, repo, "evt-dup", "task-1", "")

	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), "evt-dup", "task-1", nil, nil, now, now)
	if err == nil {
		t.Fatal("expected a unique-constraint error inserting a duplicate usage_event_id, got nil")
	}
	if !isUsageEventUniqueViolation(err) {
		t.Errorf("isUsageEventUniqueViolation(%v) = false, want true", err)
	}
}

// TestIsUsageEventUniqueViolation_SQLitePrimaryKeyConflictNotMisclassified
// proves the detector is constraint-specific: a duplicate on the table's own
// primary key must NOT be reported as a usage_event_id violation, even
// though both are "UNIQUE constraint failed" errors.
func TestIsUsageEventUniqueViolation_SQLitePrimaryKeyConflictNotMisclassified(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-1")
	now := time.Now().UTC()

	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			id, usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), int64(9001), "evt-pk-1", "task-1", nil, nil, now, now)
	if err != nil {
		t.Fatalf("seed row with explicit id: %v", err)
	}

	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			id, usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), int64(9001), "evt-pk-2", "task-1", nil, nil, now, now)
	if err == nil {
		t.Fatal("expected a primary-key conflict inserting a duplicate id, got nil")
	}
	if isUsageEventUniqueViolation(err) {
		t.Errorf("isUsageEventUniqueViolation(%v) = true, want false (this is a primary-key conflict, not a usage_event_id conflict)", err)
	}
}

// TestIsUsageEventUniqueViolation_SQLiteForeignKeyNotMisclassified proves a
// foreign-key rejection (AC-32's other classified failure kind) is never
// mistaken for a unique violation.
func TestIsUsageEventUniqueViolation_SQLiteForeignKeyNotMisclassified(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	now := time.Now().UTC()

	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), "evt-no-task", "task-does-not-exist", nil, nil, now, now)
	if err == nil {
		t.Fatal("expected a foreign-key error inserting a row against an unknown task_id, got nil")
	}
	if isUsageEventUniqueViolation(err) {
		t.Errorf("isUsageEventUniqueViolation(%v) = true, want false (this is a foreign-key violation)", err)
	}
}
