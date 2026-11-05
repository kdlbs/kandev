package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTypedPolicyTaskGuardSQLite(t *testing.T) {
	testTypedPolicyTaskGuard(t, sqlitePolicyDB(t))
}

func TestTypedPolicyTaskGuardPostgres(t *testing.T) {
	testTypedPolicyTaskGuard(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testTypedPolicyTaskGuard(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, ask, static, wider := typedPolicyGuardFixture(t, database)
	selection := SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "ask", TaskID: "source"}
	_, err := service.AdmitTask(t.Context(), selection, "")
	require.NoError(t, err)
	service, err = NewService(database, true, nil)
	require.NoError(t, err)
	got, err := service.AdmitTask(t.Context(), selection, "")
	require.NoError(t, err)
	require.Equal(t, ask.ID, got.Revision.ID)

	for _, agent := range []string{"static", "wider"} {
		changed := selection
		changed.AgentProfileID = agent
		_, err = service.AdmitTask(t.Context(), changed, "")
		require.ErrorIs(t, err, ErrIneligible, "task must retain static and conditional limits independently")
		changed.TaskID = "child-" + agent
		_, err = service.AdmitTask(t.Context(), changed, "source")
		require.ErrorIs(t, err, ErrIneligible, "parent guard must retain conditional limits")
		changed.TaskID = "copy-" + agent
		require.NoError(t, service.InheritTaskGuard(t.Context(), changed, "source"))
		_, err = service.AdmitTask(t.Context(), changed, "")
		require.ErrorIs(t, err, ErrIneligible, "copied guard must retain conditional limits")
	}

	other := selection
	other.TaskID, other.AgentProfileID = "other", "static"
	_, err = service.AdmitTask(t.Context(), other, "")
	require.NoError(t, err)
	merged := selection
	merged.TaskID = "merged"
	require.ErrorIs(t, service.InheritTaskGuard(t.Context(), merged, "source", "other"), ErrIneligible,
		"static and conditional grants cannot be combined by choosing an apparently empty legacy grant set")

	selection.AgentProfileID = "static"
	got, err = service.AdmitTask(operatorContext(t), selection, "")
	require.NoError(t, err)
	require.Equal(t, static.ID, got.Revision.ID)
	unchanged, err := service.GetRevision(operatorContext(t), wider.ID)
	require.NoError(t, err)
	require.Equal(t, wider, unchanged, "task authority must not mutate saved policies")
}

func typedPolicyGuardFixture(t *testing.T, database *sqlx.DB) (*Service, *Revision, *Revision, *Revision) {
	t.Helper()
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	service.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) {
		return TargetScope{OrgID: "org"}, nil
	})
	maximum := typedPolicyFixture()
	maximum.ID = "maximum"
	maximum.PolicyV2.StaticGrants.TCP = append(maximum.PolicyV2.StaticGrants.TCP,
		contract.TCPDestination{Host: "api.example.test", Port: 8443, Transport: contract.ProtocolTCP})
	ceiling, err := service.Publish(operatorContext(t), maximum, 0)
	require.NoError(t, err)
	staticPolicy := typedPolicyFixture()
	staticPolicy.ID = "static"
	static, err := service.Publish(operatorContext(t), staticPolicy, 0)
	require.NoError(t, err)
	askPolicy := typedPolicyFixture()
	askPolicy.ID = "ask"
	askPolicy.PolicyV2.ApprovalEnvelope.TCP = askPolicy.PolicyV2.StaticGrants.TCP
	askPolicy.PolicyV2.StaticGrants.TCP = nil
	ask, err := service.Publish(operatorContext(t), askPolicy, 0)
	require.NoError(t, err)
	askPolicy.ID = "wider"
	askPolicy.PolicyV2.ApprovalEnvelope.TCP = maximum.PolicyV2.StaticGrants.TCP
	wider, err := service.Publish(operatorContext(t), askPolicy, 0)
	require.NoError(t, err)
	_, err = service.PutBinding(operatorContext(t), Binding{Scope: ScopeExecutor, TargetID: "executor", Executor: &ExecutorPolicy{
		Mode: ModeRequired, InstallationID: ceiling.Policy.InstallationID, ProviderKey: ceiling.Policy.ProviderKey,
		CeilingRevisionID: ceiling.ID, EligibleRevisionIDs: []string{ask.ID, static.ID, wider.ID}, DefaultRevisionID: static.ID,
	}}, 0)
	require.NoError(t, err)
	for agent, revision := range map[string]*Revision{"ask": ask, "static": static, "wider": wider} {
		_, err = service.PutBinding(operatorContext(t), Binding{Scope: ScopeAgent, TargetID: agent, RevisionID: revision.ID}, 0)
		require.NoError(t, err)
	}
	return service, ask, static, wider
}

func TestTypedPolicySourceCannotPromoteConditionalAccess(t *testing.T) {
	service, ask, _, _ := typedPolicyGuardFixture(t, sqlitePolicyDB(t))
	derived := typedPolicyFixture()
	derived.ID = "derived"
	derived.Sources = []Source{{Kind: policySourceRevision, ID: ask.ID, Digest: ask.Digest}}
	_, err := service.Publish(operatorContext(t), derived, 0)
	require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	derived.PolicyV2.ApprovalEnvelope = ask.Policy.PolicyV2.ApprovalEnvelope
	_, err = service.Publish(operatorContext(t), derived, 0)
	require.NoError(t, err, "explicit provenance must preserve the source's conditional access")
}

func TestTypedPolicyGuardRefusesUnknownCapturedCeiling(t *testing.T) {
	service, _, _, _ := typedPolicyGuardFixture(t, sqlitePolicyDB(t))
	selection := SelectionContext{OrgID: "org", ExecutorID: "executor", AgentProfileID: "ask", TaskID: "source"}
	resolved, err := service.AdmitTask(t.Context(), selection, "")
	require.NoError(t, err)
	resolved.Ceiling.SchemaVersion = 99
	raw, err := json.Marshal(taskGuard{Resolved: resolved})
	require.NoError(t, err)
	_, err = service.db.Exec(`UPDATE sandbox_task_guards SET content_json=? WHERE task_id=?`, string(raw), selection.TaskID)
	require.NoError(t, err)
	_, err = service.AdmitTask(operatorContext(t), selection, "")
	require.ErrorIs(t, err, ErrIneligible, "operator authority does not repair an unknown captured format")
	selection.TaskID = "copy"
	require.ErrorIs(t, service.InheritTaskGuard(t.Context(), selection, "source"), ErrIneligible)
}
