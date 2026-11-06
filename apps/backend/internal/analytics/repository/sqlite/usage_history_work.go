package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func transcriptUsageWork(scope, subject, transcript string) string {
	if transcript == "" {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-work:"+subject)).String()
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-transcript:"+scope+":"+transcript)).String()
}

func (r *Repository) subjectTranscriptTx(ctx context.Context, tx *sqlx.Tx, scope, subject string) (string, error) {
	var transcript string
	err := tx.GetContext(ctx, &transcript, r.db.Rebind(`SELECT external_session_id FROM usage_collection_bindings
		WHERE scope_id = ? AND subject_id = ? ORDER BY sequence DESC LIMIT 1`), scope, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return transcript, err
}

func (r *Repository) freezeNativeTranscriptTx(ctx context.Context, tx *sqlx.Tx, m *models.SessionUsageMeasurement) error {
	if m.TranscriptID != "" {
		return nil
	}
	err := tx.GetContext(ctx, &m.TranscriptID, r.db.Rebind(`SELECT transcript_id FROM session_usage_buckets WHERE source='native' AND source_record_id=?`), m.SourceRecordID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	m.TranscriptID, err = r.subjectTranscriptTx(ctx, tx, m.ScopeID, m.SubjectID)
	return err
}

func (r *Repository) validateUsageEventCoverageTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, event string, m models.SessionUsageMeasurement) error {
	if event == "" {
		return nil
	}
	if canonicalCoverageKind(m) != usageCoveragePoint || m.CoverageStart == nil {
		return errors.New("provider event requires its exact occurrence coverage")
	}
	var at sql.NullTime
	if err := tx.GetContext(ctx, &at, r.db.Rebind(`SELECT coverage_start FROM session_usage_buckets WHERE source='native' AND source_record_id=? AND scope_id=? AND subject_id=? AND retired=0`), event, target.ScopeID, target.SubjectID); err != nil {
		return err
	}
	if !at.Valid || !at.Time.Equal(*m.CoverageStart) {
		return errors.New("provider event occurrence does not match retained evidence")
	}
	return nil
}

func (r *Repository) deriveExternalUsageAttributionTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, event string, m *models.SessionUsageMeasurement) error {
	if event != "" {
		return tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT actor_id, actor_status, actor_kind FROM session_usage_buckets
			WHERE scope_id = ? AND subject_id = ? AND source = 'native' AND source_record_id = ? AND retired = 0`),
			target.ScopeID, target.SubjectID, event).Scan(&m.ActorID, &m.ActorStatus, &m.ActorKind)
	}
	subjects := `SELECT subject_id FROM usage_collection_bindings WHERE scope_id = ? AND (subject_id = ? OR (external_session_id <> '' AND external_session_id = ?))`
	args := []any{target.ScopeID, target.SubjectID, target.ExternalSessionID}
	through := m.CollectedAt
	if m.CoverageEnd != nil && (through.IsZero() || m.CoverageEnd.Before(through)) {
		through = *m.CoverageEnd
	}
	var known, unknown, members int
	var actor, kind sql.NullString
	actorArgs := append(append([]any{}, args...), through)
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(DISTINCT CASE WHEN b.actor_status = 'attributed' THEN b.actor_kind || ':' || b.actor_id END),
		COALESCE(SUM(CASE WHEN b.binding_id IS NULL OR b.actor_status <> 'attributed' THEN 1 ELSE 0 END),0), MAX(b.actor_id), MAX(b.actor_kind)
		FROM (SELECT DISTINCT subject_id FROM (`+subjects+`) subjects) members LEFT JOIN usage_execution_bindings b
		ON b.subject_id=members.subject_id AND b.created_at < ?`), actorArgs...).Scan(&known, &unknown, &actor, &kind)
	if err != nil {
		return err
	}
	m.ActorID, m.ActorStatus, m.ActorKind = "", usageUnknown, usageUnknown
	if known == 1 && unknown == 0 {
		m.ActorID, m.ActorStatus, m.ActorKind = actor.String, "attributed", kind.String
	} else if known > 1 || (known > 0 && unknown > 0) {
		m.ActorStatus = usageMixedValue
	}
	if err := tx.GetContext(ctx, &members, r.db.Rebind(`SELECT COUNT(DISTINCT subject_id) FROM (`+subjects+`) members`), args...); err != nil {
		return err
	}
	if members > 1 {
		m.AttributionStatus = models.UsageAttributionAmbiguous
	}
	return nil
}

func applySharedUsageAttribution(fact *models.UsageCanonicalFact, group []indexedObservation, tokens indexedObservation) {
	if tokens.Measurement.Source == usageSourceNative || tokens.EventID != "" {
		return
	}
	ambiguous := tokens.Measurement.AttributionStatus == models.UsageAttributionAmbiguous
	for _, row := range group {
		ambiguous = ambiguous || row.Measurement.SubjectID != tokens.Measurement.SubjectID
	}
	if ambiguous {
		m := &fact.Measurement
		m.SubjectID, m.TaskID, m.SessionID, m.UsageBindingID = "", "", "", ""
		m.AttributionStatus = models.UsageAttributionAmbiguous
	}
}
