package sandbox

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) ProposeLearning(ctx context.Context, sessionID, attemptID string, req LearningRequest) (*LearningProposal, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	report, attempt, err := s.learningInputs(ctx, sessionID, attemptID, req.ReportDigest)
	if err != nil {
		return nil, err
	}
	records, err := selectDenials(report, req.RecordIDs)
	if err != nil {
		return nil, err
	}
	candidate, added, err := s.compileLearningPolicy(ctx, attempt, records)
	if err != nil {
		return nil, err
	}
	candidate.ID, candidate.Name = uuid.NewString(), req.Name
	switch req.Scope {
	case learningScopeWorkspace:
		candidate.WorkspaceID = attempt.Snapshot.WorkspaceID
		if candidate.WorkspaceID == "" {
			return nil, ErrIneligible
		}
	case "installation":
		candidate.WorkspaceID = ""
	default:
		return nil, contract.ErrInvalidPolicy
	}
	if !validPolicyIdentity(candidate) {
		return nil, contract.ErrInvalidPolicy
	}
	base, ceiling := attempt.Snapshot.Resolved.Revision, attempt.Snapshot.Resolved.Ceiling
	proposal := &LearningProposal{ID: uuid.NewString(), SchemaVersion: 1, AttemptID: attemptID, SessionID: sessionID,
		ReportDigest: report.Digest, ReportVersion: report.Version, ReportSources: report.Data.Sources, ReportIntegrity: report.Data.Integrity, ReportComplete: report.Data.Complete, BaseRevisionID: base.ID, BaseDigest: base.Digest,
		CeilingRevisionID: ceiling.ID, CeilingDigest: ceiling.Digest, Provider: attempt.Snapshot.Provider,
		ExecutorID: attempt.Snapshot.Resolved.ExecutorID, BindingVersion: attempt.Snapshot.Resolved.BindingVersion,
		SelectedRecords: records, AddedGrants: added, Policy: candidate, OperatorID: identity.UserID, CreatedAt: time.Now().UTC()}
	// Provider reads can yield. Admit the same report and source again before persistence.
	if _, _, err = s.learningInputs(ctx, sessionID, attemptID, req.ReportDigest); err != nil {
		return nil, err
	}
	proposal.Digest, err = learningDigest(*proposal)
	if err != nil {
		return nil, err
	}
	if err = s.storeLearning(ctx, identity.OrgID, proposal); err != nil {
		return nil, err
	}
	return proposal, nil
}

func (s *Service) learningInputs(ctx context.Context, sessionID, attemptID, digest string) (*Report, *Attempt, error) {
	report, err := s.ReadReport(ctx, sessionID, attemptID)
	if err != nil {
		return nil, nil, err
	}
	if report.Digest != digest || report.Data.Status != "available" || report.Data.Integrity == reportIntegrityUnverified {
		return nil, nil, ErrConflict
	}
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, nil, err
	}
	attempt, err := s.ReadAttempt(ctx, identity.OrgID, attemptID)
	if err != nil {
		return nil, nil, err
	}
	if err = s.ValidateAttemptAuthority(ctx, attempt); err != nil {
		return nil, nil, err
	}
	for _, captured := range []*Revision{attempt.Snapshot.Resolved.Revision, attempt.Snapshot.Resolved.Ceiling} {
		latest, readErr := s.GetLatest(ctx, captured.Policy.ID)
		if readErr != nil {
			return nil, nil, readErr
		}
		if latest.ID != captured.ID || latest.Digest != captured.Digest {
			return nil, nil, ErrConflict
		}
	}
	binding, err := s.GetBinding(ctx, ScopeExecutor, attempt.Snapshot.Resolved.ExecutorID)
	if err != nil {
		return nil, nil, err
	}
	if binding.Version != attempt.Snapshot.Resolved.BindingVersion {
		return nil, nil, ErrConflict
	}
	return report, attempt, nil
}

func selectDenials(report *Report, ids []string) ([]DenialRecord, error) {
	if len(ids) == 0 || len(ids) > 256 {
		return nil, contract.ErrInvalidPolicy
	}
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		if selected[id] {
			return nil, contract.ErrInvalidPolicy
		}
		selected[id] = true
	}
	records := make([]DenialRecord, 0, len(ids))
	for _, record := range report.Data.Records {
		if selected[record.ID] {
			records = append(records, record)
			delete(selected, record.ID)
		}
	}
	if len(selected) != 0 {
		return nil, ErrConflict
	}
	return records, nil
}

func (s *Service) compileLearningPolicy(ctx context.Context, attempt *Attempt, records []DenialRecord) (Policy, contract.Grants, error) {
	if s.compileLearning == nil {
		return Policy{}, contract.Grants{}, ErrIneligible
	}
	added, err := s.compileLearning(ctx, attempt, records)
	if err != nil {
		return Policy{}, contract.Grants{}, err
	}
	added, err = contract.Canonicalize(added, s.protectedRoots)
	if err != nil {
		return Policy{}, contract.Grants{}, err
	}
	base := attempt.Snapshot.Resolved.Revision
	if contract.WithinCeiling(added, base.Policy.Grants) == nil {
		return Policy{}, contract.Grants{}, ErrIneligible
	}
	added = learningDifference(base.Policy.Grants, added)
	grants := mergeLearningGrants(base.Policy.Grants, added)
	grants, err = contract.Canonicalize(grants, s.protectedRoots)
	if err != nil {
		return Policy{}, contract.Grants{}, err
	}
	if err = contract.WithinCeiling(grants, attempt.Snapshot.Resolved.Ceiling.Policy.Grants); err != nil {
		return Policy{}, contract.Grants{}, err
	}
	candidate := base.Policy
	candidate.Grants = grants
	candidate.Sources = []Source{{Kind: "policy_revision", ID: base.ID, Digest: base.Digest}}
	return candidate, added, nil
}

func mergeLearningGrants(base, added contract.Grants) contract.Grants {
	return contract.Grants{
		Filesystem:      append(slices.Clone(base.Filesystem), added.Filesystem...),
		Network:         append(slices.Clone(base.Network), added.Network...),
		EnvironmentKeys: append(slices.Clone(base.EnvironmentKeys), added.EnvironmentKeys...),
		KandevMCP:       base.KandevMCP || added.KandevMCP,
	}
}
