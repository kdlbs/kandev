package service

import "testing"

func TestOrphanReapPathWithinRoot(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		candidate string
		want      bool
	}{
		{"exact match", "/tasks/task-a", "/tasks/task-a", true},
		{"nested match", "/tasks/task-a", "/tasks/task-a/repo/src", true},
		{"prefix collision without boundary", "/tasks/task-a", "/tasks/task-abc", false},
		{"sibling", "/tasks/task-a", "/tasks/task-b", false},
		{"parent is not within child root", "/tasks/task-a/repo", "/tasks/task-a", false},
		{"empty root", "", "/tasks/task-a", false},
		{"empty candidate", "/tasks/task-a", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orphanReapPathWithinRoot(tt.root, tt.candidate); got != tt.want {
				t.Errorf("orphanReapPathWithinRoot(%q, %q) = %v, want %v", tt.root, tt.candidate, got, tt.want)
			}
		})
	}
}

func TestAttributeOrphanReapCandidatesLongestMatch(t *testing.T) {
	// AC-TASKS-ORPHAN-REAP-002.7: a worktree directory inside a task directory
	// attributes to the longer (deeper) matching root, once each.
	roots := []string{"/tasks/task-a", "/tasks/task-a/repo"}
	snapshot := []hostProcess{
		{PID: 100, PPID: 1, Cwd: "/tasks/task-a/repo/src", Command: "sh"},
		{PID: 200, PPID: 1, Cwd: "/tasks/task-a/other-file", Command: "sh"},
		{PID: 300, PPID: 1, Cwd: "/unrelated/dir", Command: "sh"},
	}

	byRoot := attributeOrphanReapCandidates(snapshot, roots)

	deep := byRoot["/tasks/task-a/repo"]
	if len(deep) != 1 || deep[0].PID != 100 {
		t.Fatalf("expected pid 100 attributed to the deeper root, got %+v", deep)
	}
	shallow := byRoot["/tasks/task-a"]
	if len(shallow) != 1 || shallow[0].PID != 200 {
		t.Fatalf("expected pid 200 attributed to the shallower root, got %+v", shallow)
	}
	if _, ok := byRoot["/unrelated/dir"]; ok {
		t.Fatalf("unrelated cwd must not be attributed to any root")
	}
	for _, candidates := range byRoot {
		for _, cand := range candidates {
			if cand.PID == 300 {
				t.Fatalf("pid 300's cwd is outside every root and must not be a candidate (AC-TASKS-ORPHAN-REAP-003.1)")
			}
		}
	}
}

func TestAttributeOrphanReapCandidatesIgnoresEmptyCwd(t *testing.T) {
	snapshot := []hostProcess{{PID: 1, PPID: 0, Cwd: "", Command: "unparseable"}}
	byRoot := attributeOrphanReapCandidates(snapshot, []string{"/tasks/task-a"})
	if len(byRoot) != 0 {
		t.Fatalf("expected no attribution for an empty cwd, got %+v", byRoot)
	}
}
