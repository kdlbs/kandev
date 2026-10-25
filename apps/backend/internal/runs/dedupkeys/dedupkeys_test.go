package dedupkeys

import "testing"

func TestAssignmentKey(t *testing.T) {
	got := AssignmentKey("task-1", "agent-1", 3)
	want := "task_assigned:task-1:agent-1:3"
	if got != want {
		t.Fatalf("AssignmentKey() = %q, want %q", got, want)
	}
}

func TestAssignmentKey_VariesByGeneration(t *testing.T) {
	a := AssignmentKey("task-1", "agent-1", 1)
	b := AssignmentKey("task-1", "agent-1", 2)
	if a == b {
		t.Fatalf("expected distinct keys for distinct generations, got %q for both", a)
	}
}

func TestBlockerDigest_OrderIndependent(t *testing.T) {
	a := BlockerDigest([]string{"b1", "b2", "b3"})
	b := BlockerDigest([]string{"b3", "b1", "b2"})
	if a != b {
		t.Fatalf("BlockerDigest should be order-independent: %q != %q", a, b)
	}
}

func TestBlockerDigest_VariesBySet(t *testing.T) {
	a := BlockerDigest([]string{"b1", "b2"})
	b := BlockerDigest([]string{"b1", "b2", "b3"})
	if a == b {
		t.Fatalf("expected distinct digests for distinct sets, got %q for both", a)
	}
}

func TestBlockerDigest_DoesNotMutateInput(t *testing.T) {
	input := []string{"b3", "b1", "b2"}
	_ = BlockerDigest(input)
	if input[0] != "b3" || input[1] != "b1" || input[2] != "b2" {
		t.Fatalf("BlockerDigest mutated its input slice: %v", input)
	}
}
