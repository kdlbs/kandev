package sqlite

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func hasIncrementalShellHistory(raw []byte) bool {
	var metadata struct {
		Storage   int `json:"shell_output_storage"`
		Retention struct {
			Fields []string `json:"removed_fields"`
		} `json:"payload_retention"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return false
	}
	if metadata.Storage == 1 {
		return true
	}
	for _, field := range metadata.Retention.Fields {
		if field == models.ShellOutputStorageKey {
			return true
		}
	}
	return false
}

// Ordinary replacements retire chunks in the same transaction as their new body.
func reconcileShellOutputStorageTx(ctx context.Context, tx *sqlx.Tx, messageID, payloadDigest string, stored, incoming []byte) ([]byte, map[string]any, error) {
	var current struct {
		Storage int `json:"shell_output_storage"`
	}
	if err := json.Unmarshal(stored, &current); err != nil {
		return nil, nil, err
	}
	if current.Storage != 1 {
		return incoming, nil, nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(incoming, &metadata); err != nil {
		return nil, nil, err
	}
	if payloadDigest == "" && isProjectedShellOutputMetadata(metadata) && !models.ToolPayloadRemoved(metadata) {
		if models.HasIncrementalShellOutput(metadata) {
			return incoming, nil, nil
		}
		metadata[shellOutputStorageKey] = 1
		encoded, err := json.Marshal(metadata)
		return encoded, metadata, err
	}
	delete(metadata, shellOutputStorageKey)
	if err := RemoveMessageShellOutputTx(ctx, tx, messageID); err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(metadata)
	return encoded, metadata, err
}
