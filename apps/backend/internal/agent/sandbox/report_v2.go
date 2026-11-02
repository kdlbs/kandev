package sandbox

import "time"

// TypedReport keeps collection coverage, byte integrity and finalization separate.
// All records inherit the enclosing report's attempt, generation and policy binding.
type TypedReport struct {
	SchemaVersion     int                 `json:"schema_version"`
	ArtifactSetDigest string              `json:"artifact_set_digest"`
	Integrity         string              `json:"integrity"`
	Finalization      string              `json:"finalization"`
	Coverage          []ReportCoverage    `json:"coverage"`
	Sources           []TypedReportSource `json:"sources"`
	Records           []ReportEvent       `json:"records"`
}

type ReportCoverage struct {
	Kind     string `json:"kind"`
	Coverage string `json:"coverage"`
}

type TypedReportSource struct {
	ID       string `json:"id"`
	Schema   string `json:"schema"`
	Digest   string `json:"digest"`
	Identity string `json:"identity"`
	Bytes    int64  `json:"bytes"`
}

type ReportEvent struct {
	ID               string          `json:"id"`
	SourceID         string          `json:"source_id"`
	ProviderRecordID string          `json:"provider_record_id"`
	Sequence         uint64          `json:"sequence"`
	Kind             string          `json:"kind"`
	Decision         string          `json:"decision"`
	ReasonCode       string          `json:"reason_code"`
	OccurredAt       time.Time       `json:"occurred_at"`
	ExactTarget      bool            `json:"exact_target"`
	LearningBlocker  string          `json:"learning_blocker"`
	LegacyRecordID   string          `json:"legacy_record_id,omitempty"`
	Operation        ReportOperation `json:"operation"`
}

// ReportOperation is a public inspection projection, never a permission grant.
// Raw argv, callback URLs, private filenames and query values are excluded.
type ReportOperation struct {
	Filesystem *ReportFilesystem `json:"filesystem,omitempty"`
	TCP        *ReportTCP        `json:"tcp,omitempty"`
	Command    *ReportCommand    `json:"command,omitempty"`
	Endpoint   *ReportEndpoint   `json:"http_endpoint,omitempty"`
}

type ReportFilesystem struct {
	Path   string `json:"path"`
	Access string `json:"access"`
}
type ReportTCP struct {
	Host      string `json:"host"`
	Port      uint32 `json:"port"`
	Transport string `json:"transport"`
}
type ReportCommand struct {
	ID     string `json:"id"`
	Caller string `json:"caller"`
}
type ReportEndpoint struct {
	RouteID string `json:"route_id"`
	Origin  string `json:"origin"`
	Method  string `json:"method"`
	Path    string `json:"path"`
}
