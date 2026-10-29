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
	ReasonHistoryConflict          ReasonCode = "history_conflict"
	ReasonHistoryContextChanged    ReasonCode = "history_context_changed"
	ReasonHistoryProvenanceMissing ReasonCode = "history_provenance_unavailable"
	ReasonHistoryUnsupported       ReasonCode = "history_unsupported"
	ReasonExportTimeout            ReasonCode = "export_timeout"
	ReasonExportMissingObject      ReasonCode = "export_missing_object"
	ReasonExportInterrupted        ReasonCode = "export_interrupted"
)

func (r ReasonCode) Valid() bool {
	switch r {
	case "", ReasonCaptureDisabled, ReasonPolicyReadFailed, ReasonUnsupportedExecutor, ReasonNoGitRepository,
		ReasonCheckoutUnavailable, ReasonUnsafeGitState, ReasonCaptureFailed,
		ReasonComparisonFailed, ReasonContentUnavailable, ReasonContentTruncated,
		ReasonSizeLimit, ReasonEntryLimit, ReasonInvalidPathEncoding, ReasonSparseCaptureUnsupported,
		ReasonExpiredAge, ReasonExpiredTaskLimit, ReasonExpiredInstallLimit,
		ReasonHistoryConflict, ReasonHistoryContextChanged, ReasonHistoryProvenanceMissing,
		ReasonHistoryUnsupported, ReasonExportTimeout, ReasonExportMissingObject, ReasonExportInterrupted:
		return true
	default:
		return false
	}
}

// Comparison bases separate captured endpoint evidence from the accepted
// turn comparison. Legacy rows keep their raw endpoint totals explicitly.
type ComparisonBasis string

const (
	ComparisonDirect             ComparisonBasis = "direct"
	ComparisonUpstreamAdjusted   ComparisonBasis = "upstream_adjusted"
	ComparisonHistoryUnavailable ComparisonBasis = "history_unavailable"
	ComparisonLegacyRaw          ComparisonBasis = "legacy_raw"
)

func (b ComparisonBasis) Valid() bool {
	switch b {
	case ComparisonDirect, ComparisonUpstreamAdjusted, ComparisonHistoryUnavailable, ComparisonLegacyRaw:
		return true
	default:
		return false
	}
}

const (
	ProvenanceVersionLegacy = 0
	ProvenanceVersion2      = 2
	ComparisonVersionLegacy = 0
	ComparisonVersion2      = 2
	ExportBatchVersion1     = 1
	MaxProvenanceBytes      = 16 << 10
	MaxExportBatchEntries   = 25
)

// Provenance is the bounded, immutable history evidence captured with one
// endpoint. It is parsed from the checkpoint commit body on idempotent reads.
type Provenance struct {
	Version             int                `json:"version"`
	ChangeSetID         string             `json:"change_set_id"`
	CheckoutID          string             `json:"checkout_id"`
	Boundary            CheckpointBoundary `json:"boundary"`
	HashAlgorithm       string             `json:"hash_algorithm"`
	Unborn              bool               `json:"unborn,omitempty"`
	HeadCommitOID       string             `json:"head_commit_oid,omitempty"`
	HeadRef             string             `json:"head_ref,omitempty"`
	TargetConfigured    bool               `json:"target_configured"`
	TargetIdentity      string             `json:"target_identity,omitempty"`
	TargetRef           string             `json:"target_ref,omitempty"`
	TargetCommitOID     string             `json:"target_commit_oid,omitempty"`
	TargetStatus        string             `json:"target_status,omitempty"`
	TargetErrorCode     string             `json:"target_error_code,omitempty"`
	MergeBaseOID        string             `json:"merge_base_oid,omitempty"`
	OperationState      string             `json:"operation_state,omitempty"`
	TreeMergeCapability bool               `json:"tree_merge_capability"`
	CapturedAt          time.Time          `json:"captured_at,omitempty"`
}

func (p *Provenance) Valid() bool {
	return p != nil && p.Version == ProvenanceVersion2 && p.ChangeSetID != "" && p.CheckoutID != "" &&
		(p.Boundary == CheckpointStart || p.Boundary == CheckpointEnd)
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
	CommonGitDir    string             `json:"common_git_dir,omitempty"`
	CapturedAt      time.Time          `json:"captured_at"`
	Reused          bool               `json:"reused,omitempty"`
	Provenance      *Provenance        `json:"provenance,omitempty"`
}

type CompareRequest struct {
	ChangeSetID       string `json:"change_set_id"`
	CheckoutID        string `json:"checkout_id"`
	Repo              string `json:"repo,omitempty"`
	HashAlgorithm     string `json:"hash_algorithm"`
	StartCommitOID    string `json:"start_commit_oid"`
	StartTreeOID      string `json:"start_tree_oid"`
	EndCommitOID      string `json:"end_commit_oid"`
	EndTreeOID        string `json:"end_tree_oid"`
	ComparisonVersion int    `json:"comparison_version,omitempty"`
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

	ComparisonVersion       int             `json:"comparison_version"`
	ComparisonBasis         ComparisonBasis `json:"comparison_basis,omitempty"`
	ComparisonReason        ReasonCode      `json:"comparison_reason,omitempty"`
	EffectiveStartCommitOID string          `json:"effective_start_commit_oid,omitempty"`
	EffectiveStartTreeOID   string          `json:"effective_start_tree_oid,omitempty"`
	EffectiveEndCommitOID   string          `json:"effective_end_commit_oid,omitempty"`
	EffectiveEndTreeOID     string          `json:"effective_end_tree_oid,omitempty"`
	ComparisonRef           string          `json:"comparison_ref,omitempty"`
}

type ExportRequest struct {
	ChangeSetID             string          `json:"change_set_id"`
	CheckoutID              string          `json:"checkout_id"`
	Repo                    string          `json:"repo,omitempty"`
	HashAlgorithm           string          `json:"hash_algorithm"`
	StartCommitOID          string          `json:"start_commit_oid"`
	StartTreeOID            string          `json:"start_tree_oid"`
	EndCommitOID            string          `json:"end_commit_oid"`
	EndTreeOID              string          `json:"end_tree_oid"`
	ComparisonVersion       int             `json:"comparison_version,omitempty"`
	ComparisonBasis         ComparisonBasis `json:"comparison_basis,omitempty"`
	EffectiveStartCommitOID string          `json:"effective_start_commit_oid,omitempty"`
	EffectiveStartTreeOID   string          `json:"effective_start_tree_oid,omitempty"`
	EffectiveEndCommitOID   string          `json:"effective_end_commit_oid,omitempty"`
	EffectiveEndTreeOID     string          `json:"effective_end_tree_oid,omitempty"`
	ComparisonRef           string          `json:"comparison_ref,omitempty"`
	ExportBatchVersion      int             `json:"export_batch_version,omitempty"`
	Cursor                  int64           `json:"cursor,omitempty"`
	MaxEntries              int             `json:"max_entries,omitempty"`
	InventoryFingerprint    string          `json:"inventory_fingerprint,omitempty"`
	BudgetBytes             int64           `json:"budget_bytes,omitempty"`
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

	ComparisonVersion       int             `json:"comparison_version"`
	ComparisonBasis         ComparisonBasis `json:"comparison_basis,omitempty"`
	EffectiveStartCommitOID string          `json:"effective_start_commit_oid,omitempty"`
	EffectiveStartTreeOID   string          `json:"effective_start_tree_oid,omitempty"`
	EffectiveEndCommitOID   string          `json:"effective_end_commit_oid,omitempty"`
	EffectiveEndTreeOID     string          `json:"effective_end_tree_oid,omitempty"`
	ComparisonRef           string          `json:"comparison_ref,omitempty"`
	ExportBatchVersion      int             `json:"export_batch_version,omitempty"`
	NextCursor              int64           `json:"next_cursor,omitempty"`
	InventoryFingerprint    string          `json:"inventory_fingerprint,omitempty"`
	TotalFiles              int64           `json:"total_files,omitempty"`
}

// ArchiveRequest asks the executor to export the exact accepted object
// closure for one immutable interval. Object identity is always server-owned;
// the archive never accepts arbitrary revisions.
type ArchiveRequest struct {
	ChangeSetID    string   `json:"change_set_id"`
	KnownObjects   []string `json:"known_objects,omitempty"`
	CheckoutID     string   `json:"checkout_id"`
	Repo           string   `json:"repo,omitempty"`
	HashAlgorithm  string   `json:"hash_algorithm"`
	StartCommitOID string   `json:"start_commit_oid"`
	StartTreeOID   string   `json:"start_tree_oid"`
	EndCommitOID   string   `json:"end_commit_oid"`
	EndTreeOID     string   `json:"end_tree_oid"`
}

// CheckpointArchive is a self-contained pack containing the accepted commits,
// both interval trees, and every tree/blob needed to render their difference.
// Objects lists the verified object identities in Pack for the importer.
type CheckpointArchive struct {
	ChangeSetID       string     `json:"change_set_id"`
	CheckoutID        string     `json:"checkout_id"`
	HashAlgorithm     string     `json:"hash_algorithm"`
	StartCommitOID    string     `json:"start_commit_oid"`
	StartTreeOID      string     `json:"start_tree_oid"`
	EndCommitOID      string     `json:"end_commit_oid"`
	EndTreeOID        string     `json:"end_tree_oid"`
	Pack              []byte     `json:"pack"`
	Objects           []string   `json:"objects"`
	ObjectCount       int64      `json:"object_count"`
	UncompressedBytes int64      `json:"uncompressed_bytes"`
	Complete          bool       `json:"complete"`
	Reason            ReasonCode `json:"reason,omitempty"`
}
