package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestTypedPolicyBindingChecksEnvelopeAgainstCeiling(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	service.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) {
		return TargetScope{OrgID: "org"}, nil
	})
	ceilingPolicy := typedPolicyFixture()
	ceilingPolicy.ID = "ceiling"
	ceiling, err := service.Publish(operatorContext(t), ceilingPolicy, 0)
	require.NoError(t, err)
	askPolicy := typedPolicyFixture()
	askPolicy.ID = "ask"
	askPolicy.PolicyV2.StaticGrants.TCP = nil
	askPolicy.PolicyV2.ApprovalEnvelope.TCP = []contract.TCPDestination{{Host: "api.example.test", Port: 443, Transport: "tcp"}}
	ask, err := service.Publish(operatorContext(t), askPolicy, 0)
	require.NoError(t, err)
	binding := Binding{Scope: ScopeExecutor, TargetID: "executor", Executor: &ExecutorPolicy{
		Mode: ModeRequired, InstallationID: ceiling.Policy.InstallationID, ProviderKey: ceiling.Policy.ProviderKey,
		CeilingRevisionID: ceiling.ID, EligibleRevisionIDs: []string{ask.ID}, DefaultRevisionID: ask.ID,
	}}
	saved, err := service.PutBinding(operatorContext(t), binding, 0)
	require.NoError(t, err)
	resolved, err := service.Resolve(operatorContext(t), SelectionContext{OrgID: "org", ExecutorID: "executor"})
	require.NoError(t, err)
	require.Equal(t, ask.Policy.PolicyV2, resolved.Revision.Policy.PolicyV2)
	askPolicy.ID = "forbidden"
	askPolicy.PolicyV2.ApprovalEnvelope.TCP[0].Port = 8443
	forbidden, err := service.Publish(operatorContext(t), askPolicy, 0)
	require.NoError(t, err)
	binding.Executor.EligibleRevisionIDs = append(binding.Executor.EligibleRevisionIDs, forbidden.ID)
	_, err = service.PutBinding(operatorContext(t), binding, saved.Version)
	require.ErrorIs(t, err, contract.ErrOutsideCeiling, "eligibility must check the possible expansion, not only static access")
}

func TestTypedPolicyResolveRefusesUnknownRevisionSchema(t *testing.T) {
	service, ceiling, _ := policySelectionFixture(t)
	ceiling.SchemaVersion = 99
	content, err := json.Marshal(ceiling)
	require.NoError(t, err)
	_, err = service.db.Exec(`UPDATE sandbox_policy_revisions SET content_json=? WHERE id=?`, string(content), ceiling.ID)
	require.NoError(t, err)
	_, err = service.Resolve(operatorContext(t), SelectionContext{OrgID: "org", ExecutorID: "executor"})
	require.ErrorIs(t, err, ErrIneligible, "unknown schema must not pass through legacy grants")
}

func TestTypedPolicyMixedCeilingPreservesLegacyNetworkMeaning(t *testing.T) {
	service := &Service{}
	for _, protocol := range []string{contract.ProtocolTCP, contract.ProtocolHTTPS} {
		legacy := &Revision{SchemaVersion: 1, Policy: Policy{Grants: contract.Grants{
			Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}},
			Network:    []contract.NetworkGrant{{Host: "api.example.test", Port: 443, Protocol: protocol}},
		}}}
		typed := &Revision{SchemaVersion: contract.PolicySchemaV2, Policy: typedPolicyFixture()}
		for _, pair := range [][2]*Revision{{legacy, typed}, {typed, legacy}} {
			err := service.validateResolvedGrants(pair[0], pair[1])
			if protocol == contract.ProtocolTCP {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, contract.ErrOutsideCeiling, "HTTPS permission must not become an unrestricted TCP permission")
			}
		}
	}
}

func TestTypedPolicyCeilingCannotSupplyImplicitApprovalEligibility(t *testing.T) {
	service := &Service{enabled: true}
	selected := &Revision{SchemaVersion: 2, Policy: typedPolicyFixture()}
	ceiling := &Revision{SchemaVersion: 2, Policy: typedPolicyFixture()}
	ceiling.Policy.PolicyV2.ApprovalEnvelope.TCP = ceiling.Policy.PolicyV2.StaticGrants.TCP
	require.ErrorIs(t, service.validateResolvedGrants(selected, ceiling), ErrIneligible)

	prior := &Revision{SchemaVersion: 1, Policy: Policy{InstallationID: selected.Policy.InstallationID, ProviderKey: selected.Policy.ProviderKey,
		Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}},
			Network: []contract.NetworkGrant{{Host: "api.example.test", Port: 443, Protocol: contract.ProtocolTCP}}}}}
	selected.Policy.PolicyV2.ApprovalEnvelope.TCP = selected.Policy.PolicyV2.StaticGrants.TCP
	selection := SelectionContext{Prior: &Resolved{Revision: prior}}
	require.ErrorIs(t, service.checkPrior(t.Context(), selection, selected), ErrIneligible)
	require.NoError(t, service.checkPrior(operatorContext(t), selection, selected))
}
