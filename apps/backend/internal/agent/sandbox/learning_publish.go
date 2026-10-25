package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func (s *Service) PublishLearning(ctx context.Context, id, digest string) (*Revision, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	proposal, err := s.ReadLearning(ctx, id)
	if err != nil {
		return nil, err
	}
	if proposal.Digest != digest || proposal.PublishedRevisionID != "" {
		return nil, ErrConflict
	}
	_, attempt, err := s.learningInputs(ctx, proposal.SessionID, proposal.AttemptID, proposal.ReportDigest)
	if err != nil {
		return nil, err
	}
	candidate, added, err := s.compileLearningPolicy(ctx, attempt, proposal.SelectedRecords)
	if err != nil {
		return nil, err
	}
	candidate.ID, candidate.Name, candidate.WorkspaceID = proposal.Policy.ID, proposal.Policy.Name, proposal.Policy.WorkspaceID
	if !reflect.DeepEqual(candidate, proposal.Policy) || !reflect.DeepEqual(added, proposal.AddedGrants) || attempt.Snapshot.Provider != proposal.Provider {
		return nil, ErrConflict
	}
	if _, _, err = s.learningInputs(ctx, proposal.SessionID, proposal.AttemptID, proposal.ReportDigest); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	revision := &Revision{ID: uuid.NewString(), Number: 1, SchemaVersion: 1, Digest: hex.EncodeToString(hash[:]), Policy: candidate,
		OperatorID: identity.UserID, OrgID: identity.OrgID, CreatedAt: time.Now().UTC(), LearningProposalID: proposal.ID, Learning: &LearningProvenance{
			ProposalDigest: proposal.Digest, ReportDigest: proposal.ReportDigest, Sources: proposal.ReportSources, Integrity: proposal.ReportIntegrity, Complete: proposal.ReportComplete,
			Base:     Source{Kind: "policy_revision", ID: proposal.BaseRevisionID, Digest: proposal.BaseDigest},
			Ceiling:  Source{Kind: "policy_revision", ID: proposal.CeilingRevisionID, Digest: proposal.CeilingDigest},
			Provider: proposal.Provider, Records: proposal.SelectedRecords}}
	raw, err = json.Marshal(revision)
	if err != nil {
		return nil, err
	}
	if err = s.publishLearningTx(ctx, proposal, attempt, revision, raw); err != nil {
		return nil, err
	}
	return revision, nil
}

func (s *Service) publishLearningTx(ctx context.Context, proposal *LearningProposal, attempt *Attempt, revision *Revision, raw []byte) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockLearningInputs(ctx, tx, proposal, attempt); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE sandbox_learning_proposals SET published_revision_id=? WHERE org_id=? AND id=? AND published_revision_id=''`), revision.ID, revision.OrgID, proposal.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	if err = insertRevisionTx(ctx, tx, revision, raw, 0); err != nil {
		return err
	}
	return tx.Commit()
}

func lockLearningInputs(ctx context.Context, tx *sqlx.Tx, proposal *LearningProposal, attempt *Attempt) error {
	orgID := attempt.Snapshot.OrgID
	if err := learningLock(ctx, tx, `UPDATE sandbox_reports SET version=version WHERE org_id=? AND attempt_id=? AND version=?`, orgID, attempt.ID, proposal.ReportVersion); err != nil {
		return err
	}
	if err := learningLock(ctx, tx, `UPDATE sandbox_policy_bindings SET version=version WHERE org_id=? AND scope=? AND target_id=? AND version=?`, orgID, ScopeExecutor, proposal.ExecutorID, proposal.BindingVersion); err != nil {
		return err
	}
	sources := []*Revision{attempt.Snapshot.Resolved.Revision, attempt.Snapshot.Resolved.Ceiling}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Policy.ID < sources[j].Policy.ID })
	for _, source := range sources {
		if err := learningLock(ctx, tx, `UPDATE sandbox_policy_heads SET revision=revision WHERE org_id=? AND policy_id=? AND revision=?`, orgID, source.Policy.ID, source.Number); err != nil {
			return err
		}
		if err := learningLock(ctx, tx, `UPDATE sandbox_policy_revisions SET revoked=revoked WHERE org_id=? AND id=? AND revoked=0`, orgID, source.ID); err != nil {
			return err
		}
	}
	return nil
}

func learningLock(ctx context.Context, tx *sqlx.Tx, query string, args ...any) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}
