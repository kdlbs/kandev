package sandbox

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSandboxPolicyTaskAdmissionRetainsPriorAcrossReload(t *testing.T) {
	s, ceiling, read := policySelectionFixture(t)
	operator := operatorContext(t)
	_, err := s.PutBinding(operator, Binding{Scope: ScopeAgent, TargetID: "reader", RevisionID: read.ID}, 0)
	require.NoError(t, err)
	initial := SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "reader", TaskID: "task"}
	got, err := s.AdmitTask(t.Context(), initial, "")
	require.NoError(t, err)
	require.Equal(t, read.ID, got.Revision.ID)
	reloaded, err := NewService(s.db, true, nil)
	require.NoError(t, err)
	changed := initial
	changed.AgentProfileID = "writer"
	_, err = reloaded.AdmitTask(t.Context(), changed, "")
	require.ErrorIs(t, err, ErrIneligible)
	changed.ExecutorID = "unrestricted"
	_, err = reloaded.AdmitTask(t.Context(), changed, "")
	require.ErrorIs(t, err, ErrIneligible)
	changed.ExecutorID = "executor"
	got, err = reloaded.AdmitTask(operator, changed, "")
	require.NoError(t, err)
	require.Equal(t, ceiling.ID, got.Revision.ID)
}

func TestSandboxPolicyChildCannotEscapeParent(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	_, err := s.PutBinding(operatorContext(t), Binding{Scope: ScopeAgent, TargetID: "reader", RevisionID: read.ID}, 0)
	require.NoError(t, err)
	_, err = s.AdmitTask(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "reader", TaskID: "parent"}, "")
	require.NoError(t, err)
	for _, executor := range []string{"executor", "unrestricted"} {
		_, err = s.AdmitTask(t.Context(), SelectionContext{OrgID: "org", ExecutorID: executor, TaskID: "child"}, "parent")
		require.ErrorIs(t, err, ErrIneligible)
	}
	_, err = s.AdmitTask(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "reader", TaskID: "child"}, "parent")
	require.NoError(t, err)
	// Removing the parent relation must not erase the child's captured limit.
	_, err = s.AdmitTask(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor", TaskID: "child"}, "")
	require.ErrorIs(t, err, ErrIneligible)
}

func TestSandboxPolicyCopyRetainsSourceWithoutParentRelation(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	_, err := s.PutBinding(operatorContext(t), Binding{Scope: ScopeAgent, TargetID: "reader", RevisionID: read.ID}, 0)
	require.NoError(t, err)
	_, err = s.AdmitTask(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "reader", TaskID: "source"}, "")
	require.NoError(t, err)
	copied := SelectionContext{OrgID: "org", ExecutorID: "executor", TaskID: "copy"}
	require.NoError(t, s.InheritTaskGuard(t.Context(), copied, "source"))
	_, err = s.AdmitTask(t.Context(), copied, "")
	require.ErrorIs(t, err, ErrIneligible)
	copied.AgentProfileID = "reader"
	_, err = s.AdmitTask(t.Context(), copied, "")
	require.NoError(t, err)
	// A second copy cannot overwrite a restriction already captured for the target.
	require.ErrorIs(t, s.InheritTaskGuard(t.Context(), copied, "source"), ErrConflict)
}
