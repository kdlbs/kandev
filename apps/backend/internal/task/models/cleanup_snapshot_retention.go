package models

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

const snapshotReducedKey = "snapshot_reduced"

// The retained snapshot fields are exactly what Service.GetTaskSourceManifest
// still reads from a succeeded job. It decodes the snapshot, skips generations
// with no manifest, authorizes snapshot.WorkspaceID, and matches every manifest
// against snapshot.Worktrees. Dropping either key does not merely lose data: a
// reduced generation carrying a manifest would fail the workspace-identity or
// worktree-match check and turn a readable manifest into an error, including
// after the task row has been deleted.
const (
	// CleanupSnapshotManifestKey carries the archive/delete evidence itself.
	CleanupSnapshotManifestKey = "archive_source_manifest"
	// cleanupSnapshotWorkspaceKey authorizes who may read that evidence.
	cleanupSnapshotWorkspaceKey = "workspace_id"
	// cleanupSnapshotWorktreesKey is the inventory the manifest is matched
	// against, so a retained manifest stays self-consistent.
	cleanupSnapshotWorktreesKey = "worktrees"
)

func cleanupSnapshotRetained(key string) bool {
	return key == CleanupSnapshotManifestKey || key == cleanupSnapshotWorkspaceKey || key == cleanupSnapshotWorktreesKey
}

type SnapshotReduction struct {
	Snapshot     string
	RemovedBytes int64
	Reason       string
}

// ReduceCleanupSnapshot keeps only the archive source evidence contract and
// records what was removed. Everything else in the snapshot is operational
// recovery material for a cleanup that already reached a terminal state. An
// unknown key is dropped deliberately, because the retained set is the evidence
// contract rather than an inventory of known field names.
func ReduceCleanupSnapshot(snapshot string, reducedAt time.Time) (SnapshotReduction, error) {
	result := SnapshotReduction{Snapshot: snapshot}
	if snapshot == "" {
		result.Reason = payloadMalformed
		return result, nil
	}
	var fields rawPayloadObject
	if err := json.Unmarshal([]byte(snapshot), &fields); err != nil || fields == nil {
		result.Reason = payloadMalformed
		return result, nil
	}
	if _, reduced := fields[snapshotReducedKey]; reduced {
		result.Reason = "already_removed"
		return result, nil
	}
	removed := make([]string, 0, len(fields))
	for key := range fields {
		if cleanupSnapshotRetained(key) {
			continue
		}
		removed = append(removed, key)
		delete(fields, key)
	}
	if len(removed) == 0 {
		result.Reason = "no_payload"
		return result, nil
	}
	// The receipt must be deterministic so the same input always encodes to the
	// same bytes.
	sort.Strings(removed)
	return encodeSnapshotReduction(fields, removed, snapshot, reducedAt)
}

func encodeSnapshotReduction(fields rawPayloadObject, removed []string, snapshot string, reducedAt time.Time) (SnapshotReduction, error) {
	marker := map[string]any{
		"version":        1,
		"reduced_at":     reducedAt.UTC().Format(time.RFC3339Nano),
		"removed_fields": removed,
		// Fixed-width notation keeps the receipt independent of the decimal
		// width of the positive byte count.
		"removed_bytes": json.Number(strconv.FormatFloat(0, 'e', 8, 64)),
	}
	encoded, removedBytes, ok, err := encodeWithMarker(fields, marker, snapshot)
	if err != nil || !ok {
		return SnapshotReduction{Snapshot: snapshot, Reason: "no_payload"}, err
	}
	return SnapshotReduction{Snapshot: encoded, RemovedBytes: removedBytes}, nil
}

func encodeWithMarker(fields rawPayloadObject, marker map[string]any, snapshot string) (string, int64, bool, error) {
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		return "", 0, false, err
	}
	fields[snapshotReducedKey] = markerJSON
	encoded, err := json.Marshal(fields)
	if err != nil {
		return "", 0, false, err
	}
	removedBytes := int64(len(snapshot) - len(encoded))
	if removedBytes <= 0 {
		return "", 0, false, nil
	}
	marker["removed_bytes"] = json.Number(strconv.FormatFloat(float64(removedBytes), 'e', 8, 64))
	markerJSON, err = json.Marshal(marker)
	if err != nil {
		return "", 0, false, err
	}
	fields[snapshotReducedKey] = markerJSON
	encoded, err = json.Marshal(fields)
	if err != nil {
		return "", 0, false, err
	}
	removedBytes = int64(len(snapshot) - len(encoded))
	if removedBytes <= 0 {
		return "", 0, false, nil
	}
	return string(encoded), removedBytes, true, nil
}
