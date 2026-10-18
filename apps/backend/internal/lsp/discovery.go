package lsp

import "time"

type DiscoveryResult struct {
	Languages      []string       `json:"languages"`
	State          DetectionState `json:"state"`
	Truncated      bool           `json:"truncated"`
	EntriesScanned int            `json:"entries_scanned"`
	ScannedAt      time.Time      `json:"scanned_at"`
	Reasons        []string       `json:"reasons,omitempty"`
}
