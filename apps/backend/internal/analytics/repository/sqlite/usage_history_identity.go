package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (r *Repository) registerNativeWorkIdentitiesTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage, m models.SessionUsageMeasurement) error {
	aliases := []struct{ authority, id string }{{"", native.UsageEventID}}
	if native.ProviderResponseID != "" {
		aliases = append(aliases, struct{ authority, id string }{native.Provider, native.ProviderResponseID})
	}
	for _, alias := range aliases {
		_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_work_identities (scope_id, subject_id, scheme, authority, external_id, event_id)
			VALUES (?, ?, 'provider_event', ?, ?, ?) ON CONFLICT(scope_id, subject_id, scheme, authority, external_id)
			DO UPDATE SET event_id = CASE WHEN usage_work_identities.event_id = excluded.event_id THEN excluded.event_id ELSE '' END`), m.ScopeID, m.SubjectID, alias.authority, alias.id, native.UsageEventID)
		if err != nil {
			return err
		}
	}
	return r.enqueueUsageSubjectRepairTx(ctx, tx, m.ScopeID, m.SubjectID, transcriptUsageWork(m.ScopeID, m.SubjectID, m.TranscriptID))
}

func (r *Repository) resolveContractWorkIdentityTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, source string, identity *pluginsdk.UsageWorkIdentityV1) (string, string, string, error) {
	work, status := contractWorkIdentity(target, source, identity)
	if identity == nil || identity.Scheme != "provider_event" {
		return work, status, "", nil
	}
	var event string
	err := tx.GetContext(ctx, &event, r.db.Rebind(`SELECT event_id FROM usage_work_identities WHERE scope_id = ? AND subject_id = ? AND scheme = ? AND authority = ? AND external_id = ?`), target.ScopeID, target.SubjectID, identity.Scheme, stringValue(identity.Authority), identity.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return work, usageIdentityUnproven, "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	if event == "" {
		return work, "ambiguous", "", nil
	}
	var proven string
	err = tx.GetContext(ctx, &proven, r.db.Rebind(`SELECT selection_partition FROM session_usage_buckets WHERE scope_id = ? AND subject_id = ? AND source = 'native' AND source_record_id = ? AND retired = 0`), target.ScopeID, target.SubjectID, event)
	if errors.Is(err, sql.ErrNoRows) {
		return work, usageIdentityUnproven, "", nil
	}
	return proven, usageIdentityProven, event, err
}
