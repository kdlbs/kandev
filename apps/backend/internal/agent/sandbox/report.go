package sandbox

import (
	"time"
)

const reportIntegrityComplete = "complete"
const reportIntegrityUnverified = "unverified"

// ReportSource identifies bounded input bytes without exposing private paths.
type ReportSource struct {
	ID         string `json:"id"`
	Schema     string `json:"schema"`
	Digest     string `json:"digest"`
	Cursor     string `json:"cursor"`
	NextCursor string `json:"next_cursor"`
	Integrity  string `json:"integrity"`
	Complete   bool   `json:"complete"`
}

type DenialRecord struct {
	ID               string    `json:"id"`
	SourceID         string    `json:"source_id"`
	ProviderRecordID string    `json:"provider_record_id"`
	Kind             string    `json:"kind"`
	Operation        string    `json:"operation"`
	Target           string    `json:"target"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// ReportData is a provider-normalized snapshot. Missing evidence is unavailable,
// and partial reports never imply that every denied operation was observed.
type ReportData struct {
	Status           string         `json:"status"`
	Integrity        string         `json:"integrity"`
	Complete         bool           `json:"complete"`
	Finalized        bool           `json:"finalized"`
	SupportedClasses []string       `json:"supported_classes"`
	Sources          []ReportSource `json:"sources"`
	Records          []DenialRecord `json:"records"`
}

type Report struct {
	SchemaVersion int             `json:"schema_version"`
	AttemptID     string          `json:"attempt_id"`
	SessionID     string          `json:"session_id"`
	RevisionID    string          `json:"revision_id"`
	PolicyDigest  string          `json:"policy_digest"`
	CeilingDigest string          `json:"ceiling_digest"`
	Provider      AttemptProvider `json:"provider"`
	Version       int64           `json:"version"`
	Digest        string          `json:"digest"`
	CapturedAt    time.Time       `json:"captured_at"`
	Data          ReportData      `json:"data"`
}
