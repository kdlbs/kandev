package repository

import "testing"

// TestParticipantRoleSeatLockKey_ByteIdenticalForSameInputs proves the one
// exported derivation both automatic casting and manual registration call
// produces byte-identical keys for the same (task, role) pair, regardless
// of call site. A shared exclusion is only real if both writers derive the
// same key bytes — no embedded-dialect integration test can distinguish a
// shared exclusion from two private ones that happen to both work under
// SQLite's single-writer pool.
func TestParticipantRoleSeatLockKey_ByteIdenticalForSameInputs(t *testing.T) {
	a := ParticipantRoleSeatLockKey("task-1", "reviewer")
	b := ParticipantRoleSeatLockKey("task-1", "reviewer")
	if a != b {
		t.Fatalf("keys differ for identical inputs: %q vs %q", a, b)
	}
}

// TestParticipantRoleSeatLockKey_DistinctInputsDiverge guards against a
// degenerate derivation (e.g. concatenation without a separator) that
// could collide two different (task, role) pairs onto the same key —
// {"task-1","2reviewer"} joined without a separator collides with
// {"task-12","reviewer"}.
func TestParticipantRoleSeatLockKey_DistinctInputsDiverge(t *testing.T) {
	keyA := ParticipantRoleSeatLockKey("task-1", "2reviewer")
	keyB := ParticipantRoleSeatLockKey("task-12", "reviewer")
	if keyA == keyB {
		t.Fatalf("distinct (task, role) pairs collided onto one key: %q", keyA)
	}

	keyC := ParticipantRoleSeatLockKey("task-1", "reviewer")
	keyD := ParticipantRoleSeatLockKey("task-2", "reviewer")
	keyE := ParticipantRoleSeatLockKey("task-1", "approver")
	if keyC == keyD || keyC == keyE || keyD == keyE {
		t.Fatalf("distinct inputs produced colliding keys: %q %q %q", keyC, keyD, keyE)
	}
}
