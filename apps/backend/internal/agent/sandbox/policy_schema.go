package sandbox

import (
	"context"
	"errors"
	"slices"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func canonicalPolicy(policy Policy, protectedRoots []string) (Policy, int, error) {
	if policy.PolicyV2 == nil {
		grants, err := contract.Canonicalize(policy.Grants, protectedRoots)
		policy.Grants = grants
		return policy, 1, err
	}
	if hasLegacyGrants(policy.Grants) {
		return Policy{}, 0, contract.ErrInvalidPolicy
	}
	canonical, err := contract.CanonicalizePolicyV2(*policy.PolicyV2, protectedRoots)
	if err != nil {
		return Policy{}, 0, err
	}
	policy.Grants = contract.Grants{}
	policy.PolicyV2 = &canonical
	return policy, contract.PolicySchemaV2, nil
}

func hasLegacyGrants(grants contract.Grants) bool {
	return len(grants.Filesystem) > 0 || len(grants.Network) > 0 || len(grants.EnvironmentKeys) > 0 || grants.KandevMCP
}

// ValidSchema requires a known revision version and one matching grant model.
func (revision *Revision) ValidSchema() bool {
	if revision == nil {
		return false
	}
	if revision.SchemaVersion == 1 {
		return revision.Policy.PolicyV2 == nil
	}
	return revision.SchemaVersion == contract.PolicySchemaV2 && revision.Policy.PolicyV2 != nil &&
		revision.Policy.PolicyV2.SchemaVersion == contract.PolicySchemaV2 && !hasLegacyGrants(revision.Policy.Grants)
}

// SupportedBy requires explicit schema support from a major 2 provider.
func (revision *Revision) SupportedBy(protocolMajor uint32, schemas []uint32) bool {
	if !revision.ValidSchema() {
		return false
	}
	switch protocolMajor {
	case 1:
		return revision.SchemaVersion == 1
	case 2:
		return slices.Contains(schemas, uint32(revision.SchemaVersion))
	default:
		return false
	}
}

func hasApprovalPermissions(permissions contract.PermissionsV2) bool {
	return len(permissions.Filesystem) > 0 || len(permissions.TCP) > 0 || len(permissions.EnvironmentKeys) > 0 || permissions.KandevMCP
}

func withinPriorPolicy(candidate, prior Policy) error {
	if candidate.PolicyV2 != nil && prior.PolicyV2 != nil {
		return contract.WithinPolicyV2(*candidate.PolicyV2, *prior.PolicyV2)
	}
	if prior.PolicyV2 != nil {
		return contract.LegacyWithinCeilingV2(candidate.Grants, prior.PolicyV2.StaticGrants)
	}
	if candidate.PolicyV2 != nil {
		if hasApprovalPermissions(candidate.PolicyV2.ApprovalEnvelope) {
			return contract.ErrOutsideCeiling
		}
		return contract.WithinLegacyCeilingV2(*candidate.PolicyV2, prior.Grants)
	}
	return contract.WithinCeiling(candidate.Grants, prior.Grants)
}

func (s *Service) validatePublishedSchema(ctx context.Context, policy Policy, schema int, expected int64, migration *PolicyMigrationProvenance) error {
	if expected == 0 {
		if migration != nil {
			return ErrIneligible
		}
		return nil
	}
	previous, err := s.GetLatest(ctx, policy.ID)
	if errors.Is(err, ErrNotFound) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if previous.Number != expected {
		return ErrConflict
	}
	if !previous.ValidSchema() {
		return ErrIneligible
	}
	if migration == nil {
		if previous.SchemaVersion != schema {
			return ErrIneligible
		}
		return nil
	}
	if previous.SchemaVersion != 1 || schema != contract.PolicySchemaV2 || previous.Revoked || migration.SourceRevisionID != previous.ID || migration.SourceDigest != previous.Digest || !validApprovalDigest(migration.ReviewDigest) {
		return ErrIneligible
	}
	return nil
}
