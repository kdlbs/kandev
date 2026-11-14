package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// ensureRetainedUsageMigration marks the adoption cursor complete so retained
// target discovery is ready without consuming legacy source rows that these
// preservation fixtures still need to migrate themselves.
func ensureRetainedUsageMigration(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec(db.Rebind(`INSERT INTO usage_history_migration_state (source_key, high_water, last_key, complete, updated_at)
		VALUES ('source-adoption', '', '', 1, ?)
		ON CONFLICT(source_key) DO UPDATE SET complete = 1, last_key = ''`), time.Now().UTC())
	require.NoError(t, err)
}

func retainedUsageBinding(ctx context.Context, repo *analyticssqlite.Repository, workspace, session, collector string) (string, error) {
	selector := &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: session}
	page, err := repo.ListUsageCollectionTargets(ctx, collector, pluginsdk.UsageCollectionTargetQueryV1{
		WorkspaceID: workspace, CurrentTarget: selector,
	})
	if err != nil {
		return "", err
	}
	if len(page.Targets) != 1 || page.Targets[0].SubjectBinding == "" {
		return "", fmt.Errorf("retained usage target for session %q is unavailable", session)
	}
	return page.Targets[0].SubjectBinding, nil
}

func externalUsageObservation(binding, record, model string, tokens int64, _ time.Time) pluginsdk.UsageObservationV1 {
	item := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: record, SourceRecordID: record, SourceVersion: "fixture-v1", Revision: 1,
		ModelID: &model, Tokens: &pluginsdk.UsageTokenAmountsV1{InputTokens: &tokens, TotalTokens: &tokens},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true},
	}
	digest, err := item.ComputePayloadDigestV1()
	if err != nil {
		panic(err)
	}
	item.PayloadDigest = digest
	return item
}

func reportExternalUsageWithBinding(ctx context.Context, repo *analyticssqlite.Repository, collector, binding, record, model string, tokens int64, at time.Time) error {
	item := externalUsageObservation(binding, record, model, tokens, at)
	result, err := repo.ReportUsageObservations(ctx, collector, pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	if err != nil {
		return err
	}
	if len(result.Results) != 1 || (result.Results[0].Status != pluginsdk.UsageObservationStatusAccepted && result.Results[0].Status != pluginsdk.UsageObservationStatusUnchanged) {
		return fmt.Errorf("retained usage report was not accepted: %s", result.Results[0].Status)
	}
	return nil
}

func reportExternalUsage(ctx context.Context, repo *analyticssqlite.Repository, workspace, session, collector, record, model string, tokens int64, at time.Time) error {
	binding, err := retainedUsageBinding(ctx, repo, workspace, session, collector)
	if err != nil {
		return err
	}
	return reportExternalUsageWithBinding(ctx, repo, collector, binding, record, model, tokens, at)
}
