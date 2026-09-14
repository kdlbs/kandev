package configsync

import (
	"expvar"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// expvar counters published at package init, exposed via stdlib's
// /debug/vars handler in dev mode. Prefixed office_config_sync_ so it is
// separable from workflow sync's own metrics without a shared label. These
// are aggregate run outcomes, not per-workspace maps: the design's
// "Observability" section lists exactly this set (attempts, successes,
// failures, unchanged runs, entities created/updated/deleted, warnings
// emitted, cap truncations).
var (
	syncAttemptsTotal    = expvar.NewInt("office_config_sync_attempts_total")
	syncSuccessesTotal   = expvar.NewInt("office_config_sync_successes_total")
	syncFailuresTotal    = expvar.NewInt("office_config_sync_failures_total")
	syncUnchangedTotal   = expvar.NewInt("office_config_sync_unchanged_total")
	entitiesCreatedTotal = expvar.NewInt("office_config_sync_entities_created_total")
	entitiesUpdatedTotal = expvar.NewInt("office_config_sync_entities_updated_total")
	entitiesDeletedTotal = expvar.NewInt("office_config_sync_entities_deleted_total")
	warningsEmittedTotal = expvar.NewInt("office_config_sync_warnings_emitted_total")
	capTruncationsTotal  = expvar.NewInt("office_config_sync_cap_truncations_total")
)

// capTruncationMarker is a substring unique to capWarnings' truncation
// entry, used only to recognize it for the cap-truncation counter.
const capTruncationMarker = "further warning(s) dropped"

// recordSyncMetrics updates the expvar counters and emits a structured
// office.configsync.* log line for one completed (or failed) run. Called
// from the single place both trigger paths converge, Service.SyncWorkspace.
func recordSyncMetrics(log *logger.Logger, workspaceID, provider string, result *SyncResult, err error) {
	syncAttemptsTotal.Add(1)
	if err != nil {
		syncFailuresTotal.Add(1)
		log.Warn("office.configsync.sync_failed",
			zap.String("workspace_id", workspaceID), zap.String("provider", provider), zap.Error(err))
		return
	}
	syncSuccessesTotal.Add(1)
	if result.Unchanged {
		syncUnchangedTotal.Add(1)
	}
	entitiesCreatedTotal.Add(int64(len(result.Created)))
	entitiesUpdatedTotal.Add(int64(len(result.Updated)))
	entitiesDeletedTotal.Add(int64(len(result.Deleted)))
	warningsEmittedTotal.Add(int64(len(result.Warnings)))
	if len(result.Warnings) > 0 && strings.Contains(result.Warnings[len(result.Warnings)-1], capTruncationMarker) {
		capTruncationsTotal.Add(1)
	}
	log.Info("office.configsync.sync_succeeded",
		zap.String("workspace_id", workspaceID), zap.String("provider", provider),
		zap.Int("created", len(result.Created)), zap.Int("updated", len(result.Updated)),
		zap.Int("deleted", len(result.Deleted)), zap.Int("warnings", len(result.Warnings)),
		zap.Bool("unchanged", result.Unchanged))
}
