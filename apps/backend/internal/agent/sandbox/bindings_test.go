package sandbox

import (
	"context"
	"errors"
	"testing"
)

func policySelectionFixture(t *testing.T) (*Service, *Revision, *Revision) {
	t.Helper()
	s, err := NewService(sqlitePolicyDB(t), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) { return TargetScope{OrgID: "org"}, nil })
	policy := policyFixture(t)
	ceiling, err := s.Publish(operatorContext(t), policy, 0)
	if err != nil {
		t.Fatal(err)
	}
	policy.ID = "read"
	policy.Grants.Filesystem[0].Writable = false
	read, err := s.Publish(operatorContext(t), policy, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutBinding(operatorContext(t), Binding{Scope: ScopeExecutor, TargetID: "executor", Executor: &ExecutorPolicy{Mode: ModeRequired, InstallationID: policy.InstallationID, ProviderKey: policy.ProviderKey, CeilingRevisionID: ceiling.ID, EligibleRevisionIDs: []string{ceiling.ID, read.ID}, DefaultRevisionID: ceiling.ID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, ceiling, read
}

func TestSandboxPolicySelectionPrecedence(t *testing.T) {
	s, ceiling, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	selection := SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "agent", WorkflowStepID: "step", TaskID: "task"}
	for _, scope := range []string{ScopeExecutor, ScopeAgent, ScopeWorkflowStep, ScopeTask} {
		if scope != ScopeExecutor {
			id := map[string]string{ScopeAgent: "agent", ScopeWorkflowStep: "step", ScopeTask: "task"}[scope]
			if _, err := s.PutBinding(ctx, Binding{Scope: scope, TargetID: id, RevisionID: read.ID}, 0); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.Resolve(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.SelectionSource != scope {
			t.Fatalf("scope=%s got=%+v", scope, got)
		}
		if scope == ScopeExecutor && got.Revision.ID != ceiling.ID {
			t.Fatal("executor default lost")
		}
	}
	// A utility has no workflow/task context.
	got, err := s.Resolve(ctx, SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "agent"})
	if err != nil || got == nil || got.SelectionSource != ScopeAgent {
		t.Fatalf("utility selection=%+v err=%v", got, err)
	}
}

func TestSandboxPolicyRejectsWeakening(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	if _, err := s.PutBinding(ctx, Binding{Scope: ScopeTask, TargetID: "task", RevisionID: read.ID}, 0); err != nil {
		t.Fatal(err)
	}
	prior, err := s.Resolve(ctx, SelectionContext{OrgID: "org", ExecutorID: "executor", TaskID: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if prior == nil {
		t.Fatal("missing frozen selection")
	}
	if _, err = s.Resolve(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor", Prior: prior}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("agent switch expanded grants: %v", err)
	}
	if _, err = s.Resolve(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "unprotected", Prior: prior}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("executor switch removed confinement: %v", err)
	}
	if _, err = s.PutBinding(t.Context(), Binding{Scope: ScopeTask, TargetID: "task"}, 1); !errors.Is(err, ErrAuthority) {
		t.Fatalf("agent changed policy: %v", err)
	}
	if _, err = s.PutBinding(ctx, Binding{Scope: ScopeTask, TargetID: "task", RevisionID: read.ID}, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale binding overwrite: %v", err)
	}
}

func TestSandboxPolicyBindingValidation(t *testing.T) {
	s, ceiling, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	for _, binding := range []Binding{
		{Scope: "unknown", TargetID: "x", RevisionID: read.ID},
		{Scope: ScopeTask, TargetID: "task", RevisionID: "missing"},
		{Scope: ScopeTask, TargetID: "task", Executor: &ExecutorPolicy{Mode: ModeDisabled}},
		{Scope: ScopeExecutor, TargetID: "other", Executor: &ExecutorPolicy{Mode: ModeRequired}},
		{Scope: ScopeExecutor, TargetID: "other", Executor: &ExecutorPolicy{Mode: ModeRequired, InstallationID: read.Policy.InstallationID, ProviderKey: read.Policy.ProviderKey, CeilingRevisionID: read.ID, EligibleRevisionIDs: []string{ceiling.ID}, DefaultRevisionID: ceiling.ID}},
	} {
		if _, err := s.PutBinding(ctx, binding, 0); err == nil {
			t.Fatalf("accepted invalid binding: %+v", binding)
		}
	}
	if _, err := s.Resolve(ctx, SelectionContext{OrgID: "org", TaskID: "task"}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("missing executor context: %v", err)
	}
}
