package database

import (
	"expvar"
	"time"
)

var (
	databaseSizeBytes             = expvar.NewInt("database_size_bytes")
	databaseWALSizeBytes          = expvar.NewInt("database_wal_size_bytes")
	messageContentBytes           = expvar.NewInt("task_message_content_bytes")
	messageMetadataBytes          = expvar.NewInt("task_message_metadata_bytes")
	messagePayloadCompressedBytes = expvar.NewInt("task_message_payload_compressed_bytes")
	gitSnapshotBytes              = expvar.NewInt("task_git_snapshot_bytes")
	logicalStatsScanOutcomes      = expvar.NewMap("database_logical_stats_scan_total")
	logicalStatsMeasuredAtUnix    = expvar.NewInt("database_logical_stats_measured_at_unix")
)

func recordLogicalStorageMetrics(stats logicalStorageStats, measuredAt time.Time) {
	messageContentBytes.Set(stats.messageContent)
	messageMetadataBytes.Set(stats.messageMetadata)
	messagePayloadCompressedBytes.Set(stats.messagePayload)
	gitSnapshotBytes.Set(stats.gitSnapshot)
	logicalStatsMeasuredAtUnix.Set(measuredAt.Unix())
}

func recordLogicalStatsScanOutcome(outcome string) {
	logicalStatsScanOutcomes.Add(outcome, 1)
}
