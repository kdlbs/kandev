package models

import "time"

// ClarificationResolution is the durable claim on a clarification bundle. One
// row per pending_id, written at most once per terminal outcome; the primary
// key IS the claim (docs/specs/external-question-answering/spec.md,
// "clarification_resolutions").
type ClarificationResolution struct {
	PendingID  string
	SessionID  string
	TaskID     string
	Status     string
	Response   string
	Resume     string
	ResolvedBy string
	Source     string
	ResolvedAt time.Time
}

// Clarification resolution status values (spec "clarification_resolutions", M6).
const (
	ClarificationResolutionStatusAnswered  = "answered"
	ClarificationResolutionStatusRejected  = "rejected"
	ClarificationResolutionStatusCancelled = "cancelled"
)

// Clarification resolution resume outcomes (spec M7/R8a).
const (
	ClarificationResolutionResumePending       = "pending"
	ClarificationResolutionResumePublished     = "published"
	ClarificationResolutionResumeFailed        = "failed"
	ClarificationResolutionResumeNotApplicable = "not_applicable"
)

// Clarification resolution source surfaces (spec M10).
const (
	ClarificationResolutionSourceWeb      = "web"
	ClarificationResolutionSourceMCP      = "mcp"
	ClarificationResolutionSourceInternal = "internal"
)
