package sandbox

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

func (s *Service) initLearning() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sandbox_learning_proposals (
 org_id TEXT NOT NULL, id TEXT NOT NULL, session_id TEXT NOT NULL, attempt_id TEXT NOT NULL,
 content_json TEXT NOT NULL, published_revision_id TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,attempt_id) REFERENCES sandbox_attempts(org_id,attempt_id) ON DELETE CASCADE
 )`)
	return err
}

func learningDigest(proposal LearningProposal) (string, error) {
	proposal.Digest, proposal.PublishedRevisionID = "", ""
	raw, err := json.Marshal(proposal)
	if err != nil {
		return "", err
	}
	if len(raw) > 256<<10 {
		return "", ErrIneligible
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) storeLearning(ctx context.Context, orgID string, proposal *LearningProposal) error {
	raw, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	if len(raw) > 256<<10 {
		return ErrIneligible
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_learning_proposals (org_id,id,session_id,attempt_id,content_json) VALUES (?,?,?,?,?)`), orgID, proposal.ID, proposal.SessionID, proposal.AttemptID, string(raw))
	return err
}

func (s *Service) ReadLearning(ctx context.Context, id string) (*LearningProposal, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	var raw, published, sessionID, attemptID string
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT content_json,published_revision_id,session_id,attempt_id FROM sandbox_learning_proposals WHERE org_id=? AND id=?`), identity.OrgID, id).Scan(&raw, &published, &sessionID, &attemptID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err = s.authorizeReport(ctx, sessionID, attemptID); err != nil {
		return nil, err
	}
	var proposal LearningProposal
	if len(raw) > 256<<10 || json.Unmarshal([]byte(raw), &proposal) != nil {
		return nil, ErrConflict
	}
	digest, err := learningDigest(proposal)
	if err != nil || digest != proposal.Digest || proposal.ID != id || proposal.SessionID != sessionID || proposal.AttemptID != attemptID || proposal.SchemaVersion != 1 {
		return nil, ErrConflict
	}
	proposal.PublishedRevisionID = published
	return &proposal, nil
}
