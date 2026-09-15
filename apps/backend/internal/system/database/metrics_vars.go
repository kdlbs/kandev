package database

import "expvar"

var (
	databaseSizeBytes             = expvar.NewInt("database_size_bytes")
	databaseWALSizeBytes          = expvar.NewInt("database_wal_size_bytes")
	messageContentBytes           = expvar.NewInt("task_message_content_bytes")
	messageMetadataBytes          = expvar.NewInt("task_message_metadata_bytes")
	messagePayloadCompressedBytes = expvar.NewInt("task_message_payload_compressed_bytes")
	gitSnapshotBytes              = expvar.NewInt("task_git_snapshot_bytes")
)

func recordStorageMetrics(stats Stats) {
	databaseSizeBytes.Set(stats.SizeBytes)
	databaseWALSizeBytes.Set(stats.WALSizeBytes)
	messageContentBytes.Set(stats.MessageContentBytes)
	messageMetadataBytes.Set(stats.MessageMetadataBytes)
	messagePayloadCompressedBytes.Set(stats.MessagePayloadBytes)
	gitSnapshotBytes.Set(stats.GitSnapshotBytes)
}
