package models

import (
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

type TurnChangeAvailability = turnchanges.Availability
type TurnChangeReason = turnchanges.ReasonCode

const (
	TurnChangeAvailabilityPending     = turnchanges.Pending
	TurnChangeAvailabilityReady       = turnchanges.Ready
	TurnChangeAvailabilityUnavailable = turnchanges.Unavailable
	TurnChangeAvailabilityFailed      = turnchanges.Failed
	TurnChangeAvailabilityExpired     = turnchanges.Expired
)

const (
	TurnChangeReasonCaptureDisabled          = turnchanges.ReasonCaptureDisabled
	TurnChangeReasonPolicyReadFailed         = turnchanges.ReasonPolicyReadFailed
	TurnChangeReasonUnsupportedExecutor      = turnchanges.ReasonUnsupportedExecutor
	TurnChangeReasonNoGitRepository          = turnchanges.ReasonNoGitRepository
	TurnChangeReasonCheckoutUnavailable      = turnchanges.ReasonCheckoutUnavailable
	TurnChangeReasonUnsafeGitState           = turnchanges.ReasonUnsafeGitState
	TurnChangeReasonCaptureFailed            = turnchanges.ReasonCaptureFailed
	TurnChangeReasonComparisonFailed         = turnchanges.ReasonComparisonFailed
	TurnChangeReasonContentUnavailable       = turnchanges.ReasonContentUnavailable
	TurnChangeReasonContentTruncated         = turnchanges.ReasonContentTruncated
	TurnChangeReasonSizeLimit                = turnchanges.ReasonSizeLimit
	TurnChangeReasonEntryLimit               = turnchanges.ReasonEntryLimit
	TurnChangeReasonInvalidPathEncoding      = turnchanges.ReasonInvalidPathEncoding
	TurnChangeReasonSparseCaptureUnsupported = turnchanges.ReasonSparseCaptureUnsupported
	TurnChangeReasonExpiredAge               = turnchanges.ReasonExpiredAge
	TurnChangeReasonExpiredTaskLimit         = turnchanges.ReasonExpiredTaskLimit
	TurnChangeReasonExpiredInstallLimit      = turnchanges.ReasonExpiredInstallLimit
)

type TurnChangeSet struct {
	ID                              string                           `db:"id" json:"id"`
	TaskID                          string                           `db:"task_id" json:"task_id"`
	TaskSessionID                   string                           `db:"session_id" json:"session_id"`
	TurnID                          string                           `db:"turn_id" json:"turn_id"`
	TaskEnvironmentID               string                           `db:"task_environment_id" json:"task_environment_id"`
	Revision                        int64                            `db:"revision" json:"revision"`
	RuntimeExecutionID              string                           `db:"runtime_execution_id" json:"runtime_execution_id"`
	StartupAttemptID                string                           `db:"startup_attempt_id" json:"startup_attempt_id"`
	PromptGeneration                int64                            `db:"prompt_generation" json:"prompt_generation"`
	ExecutionProfileID              string                           `db:"execution_profile_id" json:"execution_profile_id"`
	RouteGeneration                 int64                            `db:"route_generation" json:"route_generation"`
	CaptureEnabled                  bool                             `db:"capture_enabled" json:"capture_enabled"`
	SettingsUserID                  string                           `db:"settings_user_id" json:"settings_user_id"`
	ActorID                         string                           `db:"actor_id" json:"actor_id"`
	SettingsRevision                int64                            `db:"settings_revision" json:"settings_revision"`
	ResolutionKind                  turnchanges.PolicyResolutionKind `db:"resolution_kind" json:"resolution_kind"`
	Availability                    TurnChangeAvailability           `db:"availability" json:"availability"`
	Reason                          TurnChangeReason                 `db:"reason" json:"reason,omitempty"`
	StartAccepted                   bool                             `db:"start_accepted" json:"-"`
	Complete                        bool                             `db:"complete" json:"complete"`
	SummaryComplete                 bool                             `db:"summary_complete" json:"summary_complete"`
	ContentComplete                 bool                             `db:"content_complete" json:"content_complete"`
	TurnOrdinal                     int64                            `db:"turn_ordinal" json:"turn_ordinal"`
	TerminalAt                      *time.Time                       `db:"terminal_at" json:"terminal_at,omitempty"`
	TerminalOutcome                 string                           `db:"terminal_outcome" json:"terminal_outcome"`
	FinalAssistantMessageID         string                           `db:"final_assistant_message_id" json:"final_assistant_message_id"`
	TerminalCaptureStartedAt        *time.Time                       `db:"terminal_capture_started_at" json:"-"`
	TerminalCaptureExecutionID      string                           `db:"terminal_capture_execution_id" json:"-"`
	TerminalCaptureStartupAttemptID string                           `db:"terminal_capture_startup_attempt_id" json:"-"`
	TerminalCapturePromptGeneration int64                            `db:"terminal_capture_prompt_generation" json:"-"`
	TerminalCaptureEnvironmentID    string                           `db:"terminal_capture_environment_id" json:"-"`
	TerminalCaptureOutcome          string                           `db:"terminal_capture_outcome" json:"-"`
	TerminalCaptureFinalMessageID   string                           `db:"terminal_capture_final_message_id" json:"-"`
	FallbackAnchor                  string                           `db:"fallback_anchor" json:"fallback_anchor"`
	FileCount                       int64                            `db:"file_count" json:"file_count"`
	AddedLines                      *int64                           `db:"added_lines" json:"added_lines,omitempty"`
	DeletedLines                    *int64                           `db:"deleted_lines" json:"deleted_lines,omitempty"`
	BinaryFileCount                 int64                            `db:"binary_file_count" json:"binary_file_count"`
	UnknownCountFileCount           int64                            `db:"unknown_count_file_count" json:"unknown_count_file_count"`
	RepositoryCount                 int64                            `db:"repository_count" json:"repository_count"`
	RetainUntil                     *time.Time                       `db:"retain_until" json:"retain_until,omitempty"`
	ExpiryReason                    string                           `db:"expiry_reason" json:"expiry_reason,omitempty"`
	ContentBytes                    int64                            `db:"content_bytes" json:"content_bytes"`
	OverlapIntervals                []TurnChangeOverlap              `db:"-" json:"overlap_intervals,omitempty"`
	OverlapIntervalsJSON            string                           `db:"overlap_intervals_json" json:"-"`
	CreatedAt                       time.Time                        `db:"created_at" json:"created_at"`
	UpdatedAt                       time.Time                        `db:"updated_at" json:"updated_at"`
}

func (c TurnChangeSet) Partial() bool {
	return !c.Complete || !c.SummaryComplete || !c.ContentComplete
}

type TurnChangeOverlap struct {
	ChangeSetID string     `json:"change_set_id"`
	CheckoutID  string     `json:"checkout_id"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

type TurnRepositoryChangeSet struct {
	ID                    string                 `db:"id" json:"id"`
	TurnChangeSetID       string                 `db:"change_set_id" json:"change_set_id"`
	CheckoutID            string                 `db:"checkout_id" json:"checkout_id"`
	TaskEnvironmentRepoID string                 `db:"environment_repo_id" json:"environment_repo_id"`
	TaskRepositoryID      string                 `db:"task_repository_id" json:"task_repository_id"`
	RepositoryID          string                 `db:"repository_id" json:"repository_id"`
	WorktreeID            string                 `db:"worktree_id" json:"worktree_id"`
	DisplayName           string                 `db:"display_name" json:"display_name"`
	RepositorySubpath     string                 `db:"repository_subpath" json:"repository_subpath"`
	StartCommitOID        string                 `db:"start_commit_oid" json:"start_commit_oid"`
	StartTreeOID          string                 `db:"start_tree_oid" json:"start_tree_oid"`
	EndCommitOID          string                 `db:"end_commit_oid" json:"end_commit_oid"`
	EndTreeOID            string                 `db:"end_tree_oid" json:"end_tree_oid"`
	HashAlgorithm         string                 `db:"hash_algorithm" json:"hash_algorithm"`
	StartCapturedAt       *time.Time             `db:"start_captured_at" json:"start_captured_at,omitempty"`
	EndCapturedAt         *time.Time             `db:"end_captured_at" json:"end_captured_at,omitempty"`
	StartReachabilityRef  string                 `db:"start_ref" json:"-"`
	EndReachabilityRef    string                 `db:"end_ref" json:"-"`
	Availability          TurnChangeAvailability `db:"availability" json:"availability"`
	Reason                TurnChangeReason       `db:"reason" json:"reason,omitempty"`
	CleanupPending        bool                   `db:"cleanup_pending" json:"-"`
	EnumerationComplete   bool                   `db:"enumeration_complete" json:"enumeration_complete"`
	ComparisonComplete    bool                   `db:"comparison_complete" json:"comparison_complete"`
	ContentComplete       bool                   `db:"content_complete" json:"content_complete"`
	OverlapIntervals      []TurnChangeOverlap    `db:"-" json:"overlap_intervals,omitempty"`
	OverlapIntervalsJSON  string                 `db:"overlap_intervals_json" json:"-"`
	CreatedAt             time.Time              `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time              `db:"updated_at" json:"updated_at"`
}

type TurnFileChange struct {
	ID                    string                 `db:"id" json:"id"`
	RepositoryChangeID    string                 `db:"repository_change_id" json:"repository_change_id"`
	CheckoutID            string                 `db:"checkout_id" json:"checkout_id"`
	Path                  string                 `db:"path" json:"path"`
	PathBytes             []byte                 `db:"path_bytes" json:"-"`
	OldPath               *string                `db:"old_path" json:"old_path,omitempty"`
	OldPathBytes          []byte                 `db:"old_path_bytes" json:"-"`
	Kind                  string                 `db:"kind" json:"kind"`
	OldBlobOID            string                 `db:"old_blob_oid" json:"old_blob_oid,omitempty"`
	NewBlobOID            string                 `db:"new_blob_oid" json:"new_blob_oid,omitempty"`
	OldMode               string                 `db:"old_mode" json:"old_mode,omitempty"`
	NewMode               string                 `db:"new_mode" json:"new_mode,omitempty"`
	Submodule             bool                   `db:"submodule" json:"submodule"`
	Binary                bool                   `db:"is_binary" json:"binary"`
	AddedLines            *int64                 `db:"added_lines" json:"added_lines,omitempty"`
	DeletedLines          *int64                 `db:"deleted_lines" json:"deleted_lines,omitempty"`
	CanonicalContentID    string                 `db:"canonical_content_id" json:"-"`
	FilteredContentID     string                 `db:"filtered_content_id" json:"-"`
	OldContentID          string                 `db:"old_content_id" json:"-"`
	NewContentID          string                 `db:"new_content_id" json:"-"`
	ContentAvailability   TurnChangeAvailability `db:"content_availability" json:"content_availability"`
	ContentReason         TurnChangeReason       `db:"content_reason" json:"content_reason,omitempty"`
	ContentTruncated      bool                   `db:"content_truncated" json:"content_truncated"`
	CanonicalContentBytes int64                  `db:"canonical_content_bytes" json:"canonical_content_bytes"`
	CreatedAt             time.Time              `db:"created_at" json:"created_at"`
}

type TurnChangeContent struct {
	ID                string    `db:"id" json:"id"`
	Digest            string    `db:"digest" json:"digest"`
	Codec             string    `db:"codec" json:"codec"`
	UncompressedBytes int64     `db:"uncompressed_bytes" json:"uncompressed_bytes"`
	PayloadBytes      []byte    `db:"payload_bytes" json:"-"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
}

type TurnChangeContentLink struct {
	FileChangeID string    `db:"file_change_id" json:"file_change_id"`
	Variant      string    `db:"variant" json:"variant"`
	ContentID    string    `db:"content_id" json:"content_id"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type TurnChangeContentVariant string

const (
	TurnChangeContentCanonicalPatch TurnChangeContentVariant = "canonical_patch"
	TurnChangeContentFilteredPatch  TurnChangeContentVariant = "filtered_patch"
	TurnChangeContentOldRendering   TurnChangeContentVariant = "old_rendering"
	TurnChangeContentNewRendering   TurnChangeContentVariant = "new_rendering"
)

// TurnChangeFileContent is the bounded executor export for one immutable file
// entry. Nil payloads are unavailable; non-nil empty rendering blobs are valid.
type TurnChangeFileContent struct {
	File           TurnFileChange
	CanonicalPatch []byte
	FilteredPatch  []byte
	OldRendering   []byte
	NewRendering   []byte
}

type TurnChangeContentStoreReceipt struct {
	StoredBytes int64
	Complete    bool
	Reason      TurnChangeReason
}

type TurnChangeContentPayload struct {
	FileChangeID string
	Variant      TurnChangeContentVariant
	Content      []byte
	Digest       string
	Truncated    bool
}

type TurnChangeContentLease struct {
	ID          string
	ChangeSetID string
	ExpiresAt   time.Time
}

type TurnChangeRetentionPolicy struct {
	RetainFor         time.Duration
	TaskBytes         int64
	InstallationBytes int64
}

type TurnChangeRetentionResult struct {
	ExpiredChangeSets int64
	DeletedContents   int64
	FreedPayloadBytes int64
}

type TurnChangeSetFinalization struct {
	Availability            TurnChangeAvailability
	Reason                  TurnChangeReason
	Complete                bool
	SummaryComplete         bool
	ContentComplete         bool
	TerminalAt              time.Time
	TerminalOutcome         string
	FinalAssistantMessageID string
	FileCount               int64
	AddedLines              *int64
	DeletedLines            *int64
	BinaryFileCount         int64
	UnknownCountFileCount   int64
	RepositoryCount         int64
	RetainUntil             *time.Time
	ExpiryReason            string
	ContentBytes            int64
	Repositories            []TurnRepositoryChangeSet
	OverlapIntervals        []TurnChangeOverlap
}

type TurnChangeSetTerminalClaim struct {
	At                      time.Time
	ExecutionID             string
	StartupAttemptID        string
	PromptGeneration        int64
	TaskEnvironmentID       string
	Outcome                 string
	FinalAssistantMessageID string
}
