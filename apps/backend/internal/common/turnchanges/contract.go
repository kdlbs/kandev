// Package turnchanges defines transport-neutral contracts for immutable
// repository intervals captured around an admitted agent turn.
package turnchanges

import "time"

type Availability string

const (
	Pending     Availability = "pending"
	Ready       Availability = "ready"
	Unavailable Availability = "unavailable"
	Failed      Availability = "failed"
	Expired     Availability = "expired"
)

func (a Availability) Valid() bool {
	switch a {
	case Pending, Ready, Unavailable, Failed, Expired:
		return true
	default:
		return false
	}
}

type ReasonCode string

const (
	ReasonCaptureDisabled          ReasonCode = "capture_disabled"
	ReasonPolicyReadFailed         ReasonCode = "policy_read_failed"
	ReasonUnsupportedExecutor      ReasonCode = "unsupported_executor"
	ReasonNoGitRepository          ReasonCode = "no_git_repository"
	ReasonCheckoutUnavailable      ReasonCode = "checkout_unavailable"
	ReasonUnsafeGitState           ReasonCode = "unsafe_git_state"
	ReasonCaptureFailed            ReasonCode = "capture_failed"
	ReasonComparisonFailed         ReasonCode = "comparison_failed"
	ReasonContentUnavailable       ReasonCode = "content_unavailable"
	ReasonContentTruncated         ReasonCode = "content_truncated"
	ReasonSizeLimit                ReasonCode = "size_limit"
	ReasonEntryLimit               ReasonCode = "entry_limit"
	ReasonInvalidPathEncoding      ReasonCode = "invalid_path_encoding"
	ReasonSparseCaptureUnsupported ReasonCode = "sparse_capture_unsupported"
	ReasonExpiredAge               ReasonCode = "expired_age"
	ReasonExpiredTaskLimit         ReasonCode = "expired_task_limit"
	ReasonExpiredInstallLimit      ReasonCode = "expired_install_limit"
)

func (r ReasonCode) Valid() bool {
	switch r {
	case "", ReasonCaptureDisabled, ReasonPolicyReadFailed, ReasonUnsupportedExecutor, ReasonNoGitRepository,
		ReasonCheckoutUnavailable, ReasonUnsafeGitState, ReasonCaptureFailed,
		ReasonComparisonFailed, ReasonContentUnavailable, ReasonContentTruncated,
		ReasonSizeLimit, ReasonEntryLimit, ReasonInvalidPathEncoding, ReasonSparseCaptureUnsupported,
		ReasonExpiredAge, ReasonExpiredTaskLimit, ReasonExpiredInstallLimit:
		return true
	default:
		return false
	}
}

type PolicyResolutionKind string

const (
	PolicyAuthenticatedUser PolicyResolutionKind = "authenticated_user"
	PolicyDefaultUser       PolicyResolutionKind = "default_user"
	PolicySyntheticDefault  PolicyResolutionKind = "synthetic_default"
	PolicyReadFailed        PolicyResolutionKind = "read_failed"
)

func (k PolicyResolutionKind) Valid() bool {
	switch k {
	case PolicyAuthenticatedUser, PolicyDefaultUser, PolicySyntheticDefault, PolicyReadFailed:
		return true
	default:
		return false
	}
}

type CapturePolicy struct {
	SettingsUserID string               `json:"settings_user_id"`
	Enabled        bool                 `json:"enabled"`
	Revision       int64                `json:"revision"`
	ResolutionKind PolicyResolutionKind `json:"resolution_kind"`
}

type CheckpointBoundary string

const (
	CheckpointStart CheckpointBoundary = "start"
	CheckpointEnd   CheckpointBoundary = "end"
)

type CheckpointRequest struct {
	ChangeSetID string             `json:"change_set_id"`
	CheckoutID  string             `json:"checkout_id"`
	Repo        string             `json:"repo,omitempty"`
	Boundary    CheckpointBoundary `json:"boundary"`
}

type CheckpointDeleteRequest struct {
	ChangeSetID string             `json:"change_set_id"`
	CheckoutID  string             `json:"checkout_id"`
	Repo        string             `json:"repo,omitempty"`
	Boundary    CheckpointBoundary `json:"boundary"`
	CommitOID   string             `json:"commit_oid"`
}

type CheckpointResult struct {
	ChangeSetID     string             `json:"change_set_id"`
	CheckoutID      string             `json:"checkout_id"`
	Boundary        CheckpointBoundary `json:"boundary"`
	CommitOID       string             `json:"commit_oid"`
	TreeOID         string             `json:"tree_oid"`
	HashAlgorithm   string             `json:"hash_algorithm"`
	ReachabilityRef string             `json:"reachability_ref"`
	CapturedAt      time.Time          `json:"captured_at"`
	Reused          bool               `json:"reused,omitempty"`
}

type CompareRequest struct {
	ChangeSetID    string `json:"change_set_id"`
	CheckoutID     string `json:"checkout_id"`
	Repo           string `json:"repo,omitempty"`
	HashAlgorithm  string `json:"hash_algorithm"`
	StartCommitOID string `json:"start_commit_oid"`
	StartTreeOID   string `json:"start_tree_oid"`
	EndCommitOID   string `json:"end_commit_oid"`
	EndTreeOID     string `json:"end_tree_oid"`
}

type CheckpointFile struct {
	PathBytes    []byte     `json:"path_bytes"`
	OldPathBytes []byte     `json:"old_path_bytes,omitempty"`
	Kind         string     `json:"kind"`
	OldOID       string     `json:"old_oid,omitempty"`
	NewOID       string     `json:"new_oid,omitempty"`
	OldMode      string     `json:"old_mode,omitempty"`
	NewMode      string     `json:"new_mode,omitempty"`
	Submodule    bool       `json:"submodule"`
	Reason       ReasonCode `json:"reason,omitempty"`
	Added        *int64     `json:"added,omitempty"`
	Deleted      *int64     `json:"deleted,omitempty"`
	Binary       bool       `json:"binary"`
}

type CheckpointComparison struct {
	ChangeSetID    string           `json:"change_set_id"`
	CheckoutID     string           `json:"checkout_id"`
	HashAlgorithm  string           `json:"hash_algorithm"`
	StartCommitOID string           `json:"start_commit_oid"`
	StartTreeOID   string           `json:"start_tree_oid"`
	EndCommitOID   string           `json:"end_commit_oid"`
	EndTreeOID     string           `json:"end_tree_oid"`
	Files          []CheckpointFile `json:"files"`
	FileCount      int64            `json:"file_count"`
	AddedLines     *int64           `json:"added_lines,omitempty"`
	DeletedLines   *int64           `json:"deleted_lines,omitempty"`
	BinaryCount    int64            `json:"binary_count"`
	UnknownCount   int64            `json:"unknown_count"`
	Complete       bool             `json:"complete"`
	Reason         ReasonCode       `json:"reason,omitempty"`
}

type ExportRequest struct {
	ChangeSetID    string `json:"change_set_id"`
	CheckoutID     string `json:"checkout_id"`
	Repo           string `json:"repo,omitempty"`
	HashAlgorithm  string `json:"hash_algorithm"`
	StartCommitOID string `json:"start_commit_oid"`
	StartTreeOID   string `json:"start_tree_oid"`
	EndCommitOID   string `json:"end_commit_oid"`
	EndTreeOID     string `json:"end_tree_oid"`
}

type CheckpointExportFile struct {
	File                CheckpointFile `json:"file"`
	CanonicalPatch      []byte         `json:"canonical_patch"`
	FilteredPatch       []byte         `json:"filtered_patch"`
	OldRendering        []byte         `json:"old_rendering"`
	NewRendering        []byte         `json:"new_rendering"`
	ContentAvailability Availability   `json:"content_availability"`
	Reason              ReasonCode     `json:"reason,omitempty"`
	Truncated           bool           `json:"truncated"`
	CanonicalBytes      int64          `json:"canonical_bytes"`
}

type CheckpointExport struct {
	ChangeSetID    string                 `json:"change_set_id"`
	CheckoutID     string                 `json:"checkout_id"`
	HashAlgorithm  string                 `json:"hash_algorithm"`
	StartCommitOID string                 `json:"start_commit_oid"`
	StartTreeOID   string                 `json:"start_tree_oid"`
	EndCommitOID   string                 `json:"end_commit_oid"`
	EndTreeOID     string                 `json:"end_tree_oid"`
	Files          []CheckpointExportFile `json:"files"`
	Complete       bool                   `json:"complete"`
	Reason         ReasonCode             `json:"reason,omitempty"`
	ExportBytes    int64                  `json:"export_bytes"`
}
