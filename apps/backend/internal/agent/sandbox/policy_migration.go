package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type PolicyMigrationRequest struct {
	SourceDigest     string `json:"source_digest"`
	ExpectedRevision int64  `json:"expected_revision"`
	Policy           Policy `json:"policy"`
}

type PolicyMigrationPreview struct {
	SchemaVersion    int    `json:"schema_version"`
	SourceRevisionID string `json:"source_revision_id"`
	SourceDigest     string `json:"source_digest"`
	ExpectedRevision int64  `json:"expected_revision"`
	Before           Policy `json:"before"`
	After            Policy `json:"after"`
	Digest           string `json:"digest"`
}

type PolicyMigrationProvenance struct {
	SourceRevisionID string `json:"source_revision_id"`
	SourceDigest     string `json:"source_digest"`
	ReviewDigest     string `json:"review_digest"`
}

// PreviewPolicyMigration reviews a new meaning without selecting or granting it.
func (s *Service) PreviewPolicyMigration(ctx context.Context, sourceID string, request PolicyMigrationRequest) (*PolicyMigrationPreview, error) {
	if _, err := s.operator(ctx); err != nil {
		return nil, err
	}
	source, err := s.GetRevision(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if !source.ValidSchema() || source.Revoked || source.SchemaVersion != 1 {
		return nil, ErrIneligible
	}
	if source.Digest != request.SourceDigest || source.Number != request.ExpectedRevision {
		return nil, ErrConflict
	}
	latest, err := s.GetLatest(ctx, source.Policy.ID)
	if err != nil {
		return nil, err
	}
	if latest.ID != source.ID {
		return nil, ErrConflict
	}
	target, err := s.migrationTarget(ctx, source.Policy, request.Policy)
	if err != nil {
		return nil, err
	}
	preview := &PolicyMigrationPreview{SchemaVersion: 1, SourceRevisionID: source.ID, SourceDigest: source.Digest,
		ExpectedRevision: source.Number, Before: source.Policy, After: target}
	preview.Digest, err = policyMigrationDigest(*preview)
	return preview, err
}

func (s *Service) migrationTarget(ctx context.Context, source, target Policy) (Policy, error) {
	if target.ID != source.ID || target.WorkspaceID != source.WorkspaceID || target.InstallationID != source.InstallationID || target.ProviderKey != source.ProviderKey || target.PolicyV2 == nil || !validPolicyIdentity(target) {
		return Policy{}, contract.ErrInvalidPolicy
	}
	canonical, _, err := canonicalPolicy(target, s.protectedRoots)
	if err != nil {
		return Policy{}, err
	}
	if err = s.validateSources(ctx, canonical); err != nil {
		return Policy{}, err
	}
	return canonical, nil
}

func policyMigrationDigest(preview PolicyMigrationPreview) (string, error) {
	preview.Digest = ""
	raw, err := json.Marshal(preview)
	if err != nil || len(raw) > 512<<10 {
		return "", contract.ErrInvalidPolicy
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// PublishPolicyMigration rechecks the reviewed contents and source head.
func (s *Service) PublishPolicyMigration(ctx context.Context, sourceID string, request PolicyMigrationRequest, reviewDigest string) (*Revision, error) {
	preview, err := s.PreviewPolicyMigration(ctx, sourceID, request)
	if err != nil {
		return nil, err
	}
	if !validApprovalDigest(reviewDigest) || preview.Digest != reviewDigest {
		return nil, ErrConflict
	}
	provenance := &PolicyMigrationProvenance{SourceRevisionID: sourceID, SourceDigest: preview.SourceDigest, ReviewDigest: reviewDigest}
	return s.publishPolicy(ctx, preview.After, preview.ExpectedRevision, provenance)
}
