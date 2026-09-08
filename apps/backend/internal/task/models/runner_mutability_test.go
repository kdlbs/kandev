package models

import (
	"testing"
	"time"
)

func eligibleSignals() RunnerMutabilitySignals {
	return RunnerMutabilitySignals{RepositoryCount: 1}
}

func TestEvaluateRunnerMutability_Eligible(t *testing.T) {
	verdict := EvaluateRunnerMutability(eligibleSignals())
	if !verdict.Editable || verdict.Reason != RunnerReasonEligible {
		t.Fatalf("verdict = %+v, want editable=true reason=%s", verdict, RunnerReasonEligible)
	}
}

func TestEvaluateRunnerMutability_ConditionOrder(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RunnerMutabilitySignals)
		wantErr string
	}{
		{"archived", func(s *RunnerMutabilitySignals) { s.Archived = true }, RunnerReasonTaskArchived},
		{"no repository", func(s *RunnerMutabilitySignals) { s.RepositoryCount = 0 }, RunnerReasonNoRepository},
		{"multiple repositories", func(s *RunnerMutabilitySignals) { s.RepositoryCount = 2 }, RunnerReasonMultipleRepositories},
		{"session exists", func(s *RunnerMutabilitySignals) { s.HasSession = true }, RunnerReasonSessionExists},
		{"environment exists", func(s *RunnerMutabilitySignals) { s.HasEnvironment = true }, RunnerReasonEnvironmentExists},
		{"executor running", func(s *RunnerMutabilitySignals) { s.HasExecutorRunning = true }, RunnerReasonExecutorRunning},
		{"workspace folder attached", func(s *RunnerMutabilitySignals) { s.HasWorkspaceFolder = true }, RunnerReasonWorkspaceFolderAttached},
		{"workspace path set", func(s *RunnerMutabilitySignals) { s.WorkspacePath = "/host/path" }, RunnerReasonWorkspacePathSet},
		{"workspace group member", func(s *RunnerMutabilitySignals) { s.HasActiveGroupMembership = true }, RunnerReasonWorkspaceGroupMember},
		{"binding not independent (shared group, no parent)", func(s *RunnerMutabilitySignals) { s.WorkspaceMode = "shared_group" }, RunnerReasonWorkspaceBindingNotIndependent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := eligibleSignals()
			tt.mutate(&signals)
			verdict := EvaluateRunnerMutability(signals)
			if verdict.Editable {
				t.Fatalf("Editable = true, want false for %s", tt.name)
			}
			if verdict.Reason != tt.wantErr {
				t.Fatalf("Reason = %s, want %s", verdict.Reason, tt.wantErr)
			}
		})
	}
}

// TestEvaluateRunnerMutability_FirstFailingConditionWins proves the ten
// conditions are evaluated in the fixed AC-TASKS-RUNNER-SWITCH-001.3 order:
// a task tripping several conditions always reports the earliest one.
func TestEvaluateRunnerMutability_FirstFailingConditionWins(t *testing.T) {
	signals := RunnerMutabilitySignals{
		Archived:                 true,
		RepositoryCount:          0,
		HasSession:               true,
		HasEnvironment:           true,
		HasExecutorRunning:       true,
		HasWorkspaceFolder:       true,
		WorkspacePath:            "/host/path",
		HasActiveGroupMembership: true,
		WorkspaceMode:            "shared_group",
	}
	verdict := EvaluateRunnerMutability(signals)
	if verdict.Reason != RunnerReasonTaskArchived {
		t.Fatalf("Reason = %s, want %s (archived must win over every later condition)", verdict.Reason, RunnerReasonTaskArchived)
	}

	signals.Archived = false
	verdict = EvaluateRunnerMutability(signals)
	if verdict.Reason != RunnerReasonNoRepository {
		t.Fatalf("Reason = %s, want %s", verdict.Reason, RunnerReasonNoRepository)
	}
}

// TestEvaluateRunnerMutability_WorkspaceBindingIndependence covers every
// combination of HasParent x WorkspaceMode against AC-TASKS-RUNNER-SWITCH-001.3
// condition 10: a task with no parent is independent unless it declares
// shared-group mode; a task with a parent is independent only when it
// declares new_workspace explicitly.
func TestEvaluateRunnerMutability_WorkspaceBindingIndependence(t *testing.T) {
	tests := []struct {
		name            string
		hasParent       bool
		mode            string
		wantIndependent bool
	}{
		{"no parent, no mode", false, "", true},
		{"no parent, inherit_parent", false, "inherit_parent", true},
		{"no parent, new_workspace", false, "new_workspace", true},
		{"no parent, shared_group", false, "shared_group", false},
		{"parent, no mode", true, "", false},
		{"parent, inherit_parent", true, "inherit_parent", false},
		{"parent, shared_group", true, "shared_group", false},
		{"parent, new_workspace", true, "new_workspace", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := eligibleSignals()
			signals.HasParent = tt.hasParent
			signals.WorkspaceMode = tt.mode
			verdict := EvaluateRunnerMutability(signals)
			gotIndependent := verdict.Editable
			if gotIndependent != tt.wantIndependent {
				t.Fatalf("Editable = %v, want %v (independent=%v)", verdict.Editable, tt.wantIndependent, tt.wantIndependent)
			}
			if !tt.wantIndependent && verdict.Reason != RunnerReasonWorkspaceBindingNotIndependent {
				t.Fatalf("Reason = %s, want %s", verdict.Reason, RunnerReasonWorkspaceBindingNotIndependent)
			}
		})
	}
}

// TestEvaluateRunnerMutability_WorkspacePathBoundary pins the Build-time
// decision for F23 (spec left this boundary open): a stored workspace path
// that is empty or whitespace-only is treated as NOT set, the same "blank"
// test AC-TASKS-RUNNER-SWITCH-002.8 applies to payload identifiers. A
// permanently-unrecoverable false-immutable (whitespace trips the gate) is a
// worse product failure than a false-editable (whitespace does not trip it),
// since nothing has materialized yet and an editable task risks nothing but
// an extra no-op switch attempt.
func TestEvaluateRunnerMutability_WorkspacePathBoundary(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool // editable
	}{
		{"empty", "", true},
		{"whitespace only", "   \t\n", true},
		{"non-empty", "/host/path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := eligibleSignals()
			signals.WorkspacePath = tt.path
			verdict := EvaluateRunnerMutability(signals)
			if verdict.Editable != tt.want {
				t.Fatalf("Editable = %v, want %v", verdict.Editable, tt.want)
			}
		})
	}
}

func TestRunnerSignalsFromTask(t *testing.T) {
	now := time.Now().UTC()

	t.Run("archived", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{ArchivedAt: &now})
		if !s.Archived {
			t.Fatalf("Archived = false, want true")
		}
	})

	t.Run("not archived", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{})
		if s.Archived {
			t.Fatalf("Archived = true, want false")
		}
	})

	t.Run("workspace path from metadata", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{Metadata: map[string]interface{}{
			MetaKeyWorkspacePath: "/host/path",
		}})
		if s.WorkspacePath != "/host/path" {
			t.Fatalf("WorkspacePath = %q, want /host/path", s.WorkspacePath)
		}
	})

	t.Run("has parent", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{ParentID: "parent-1"})
		if !s.HasParent {
			t.Fatalf("HasParent = false, want true")
		}
	})

	t.Run("workspace mode from nested metadata block", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "new_workspace"},
		}})
		if s.WorkspaceMode != "new_workspace" {
			t.Fatalf("WorkspaceMode = %q, want new_workspace", s.WorkspaceMode)
		}
	})

	t.Run("no metadata block", func(t *testing.T) {
		s := RunnerSignalsFromTask(&Task{})
		if s.WorkspaceMode != "" || s.WorkspacePath != "" {
			t.Fatalf("expected empty mode/path, got %+v", s)
		}
	})
}
