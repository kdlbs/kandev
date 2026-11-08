package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

const (
	// BaseBranchSelectionMetadataKey stores the branch-bound provenance of a
	// task-repository base selection. It lives in the existing attachment
	// metadata JSON so no schema migration is required.
	BaseBranchSelectionMetadataKey = "base_branch_selection"
	BaseBranchSelectionVersion     = 1
)

// BaseBranchSelectionSource identifies who selected an attachment's base
// branch. Only server-authored sources reach persisted metadata; public JSON
// request surfaces cannot grant permissive recovery.
type BaseBranchSelectionSource string

const (
	// BaseBranchSelectionSourceExplicit is a caller-supplied base, a selected
	// branch-policy snapshot, an explicit edit, or a provider-qualified target.
	// A missing base must fail preparation.
	BaseBranchSelectionSourceExplicit BaseBranchSelectionSource = "explicit"
	// BaseBranchSelectionSourceInherited is a server-selected parent or
	// current-task repository inheritance. Known missing-base recovery keeps
	// its existing repository-default fallback.
	BaseBranchSelectionSourceInherited BaseBranchSelectionSource = "inherited"
	// BaseBranchSelectionSourceRepositoryDefault is a base the caller omitted
	// while the server selected the repository default. Known missing-base
	// recovery keeps its existing repository-default fallback.
	BaseBranchSelectionSourceRepositoryDefault BaseBranchSelectionSource = "repository_default"
)

// BaseBranchSelection is the versioned persisted selection intent. Branch is
// the effective attachment base. It is empty only for an omitted base that
// still awaits clone-time default discovery.
type BaseBranchSelection struct {
	Version int                       `json:"version"`
	Branch  string                    `json:"branch"`
	Source  BaseBranchSelectionSource `json:"source"`
}

// Validate rejects malformed or credential-free metadata rehydrated from a
// task-repository row. Any source may defer its branch while a later
// resolution supplies it; the source still controls substitution permission.
func (s BaseBranchSelection) Validate() error {
	if s.Version != BaseBranchSelectionVersion {
		return fmt.Errorf("base branch selection version %d is not supported", s.Version)
	}
	if s.Branch != strings.TrimSpace(s.Branch) {
		return errors.New("base branch selection branch must be trimmed")
	}
	if s.Branch != "" && !securityutil.IsValidBaseBranchRef(s.Branch) {
		return fmt.Errorf("base branch selection branch %q is invalid", s.Branch)
	}
	switch s.Source {
	case BaseBranchSelectionSourceExplicit,
		BaseBranchSelectionSourceInherited,
		BaseBranchSelectionSourceRepositoryDefault:
	default:
		return fmt.Errorf("base branch selection source %q is not supported", s.Source)
	}
	return nil
}

// PutBaseBranchSelection stores a validated selection while preserving
// unrelated task-repository metadata.
func PutBaseBranchSelection(metadata map[string]interface{}, selection *BaseBranchSelection) error {
	if selection == nil {
		return errors.New("base branch selection is required")
	}
	if err := selection.Validate(); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("metadata map is required")
	}
	metadata[BaseBranchSelectionMetadataKey] = selection
	return nil
}

// LoadBaseBranchSelection decodes and validates a selection from typed or
// JSON-rehydrated metadata. A missing key is not an error.
func LoadBaseBranchSelection(metadata map[string]interface{}) (BaseBranchSelection, bool, error) {
	return loadValidatedMetadata(
		metadata,
		BaseBranchSelectionMetadataKey,
		"base branch selection",
		"base branch selection is nil",
		BaseBranchSelection.Validate,
	)
}

// BaseBranchFallbackPolicy controls whether preparation may replace an
// unresolved base with another branch. Its zero value is strict.
type BaseBranchFallbackPolicy string

const (
	// BaseBranchFallbackPolicyStrict forbids every different-base
	// substitution, including live remote-default discovery.
	BaseBranchFallbackPolicyStrict BaseBranchFallbackPolicy = ""
	// BaseBranchFallbackPolicyRepositoryDefault permits the existing
	// repository-default recovery for a trusted inherited or automatic
	// selection.
	BaseBranchFallbackPolicyRepositoryDefault BaseBranchFallbackPolicy = "repository_default"
)

// PermitsRepositoryDefaultSubstitution reports whether the policy allows
// automatic repository-default recovery.
func (p BaseBranchFallbackPolicy) PermitsRepositoryDefaultSubstitution() bool {
	return p == BaseBranchFallbackPolicyRepositoryDefault
}

// ResolveBaseBranchFallbackPolicy derives substitution permission for the
// effective attachment base. The manual override marker always wins, and any
// missing, malformed, explicit, or branch-mismatched record is strict.
func ResolveBaseBranchFallbackPolicy(
	metadata map[string]interface{}, effectiveBaseBranch string,
) BaseBranchFallbackPolicy {
	if HasManualBaseBranchOverride(metadata) {
		return BaseBranchFallbackPolicyStrict
	}
	selection, ok, err := LoadBaseBranchSelection(metadata)
	if err != nil || !ok {
		return BaseBranchFallbackPolicyStrict
	}
	switch selection.Source {
	case BaseBranchSelectionSourceInherited, BaseBranchSelectionSourceRepositoryDefault:
	default:
		return BaseBranchFallbackPolicyStrict
	}
	if selection.Branch != "" && selection.Branch != effectiveBaseBranch {
		return BaseBranchFallbackPolicyStrict
	}
	return BaseBranchFallbackPolicyRepositoryDefault
}

// ResolveAttachmentBaseBranchFallbackPolicy derives policy for a persisted
// attachment. An attachment with no base carries no selection, so its later
// repository-default resolution retains automatic-default intent. A
// non-empty base resolves through the recorded selection only.
func ResolveAttachmentBaseBranchFallbackPolicy(
	metadata map[string]interface{}, attachmentBaseBranch string,
) BaseBranchFallbackPolicy {
	if strings.TrimSpace(attachmentBaseBranch) != "" {
		return ResolveBaseBranchFallbackPolicy(metadata, attachmentBaseBranch)
	}
	if HasManualBaseBranchOverride(metadata) {
		return BaseBranchFallbackPolicyStrict
	}
	if selection, ok, err := LoadBaseBranchSelection(metadata); err == nil && ok &&
		selection.Source == BaseBranchSelectionSourceExplicit {
		return BaseBranchFallbackPolicyStrict
	}
	return BaseBranchFallbackPolicyRepositoryDefault
}
