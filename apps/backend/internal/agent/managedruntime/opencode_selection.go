package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const openCodeSelectionKey = "managed_runtime.opencode.selection"

const (
	OpenCodeV1Package = "opencode-ai"
	OpenCodeV2Package = "@opencode/cli"
)

var (
	ErrInvalidOpenCodeSelection          = errors.New("invalid OpenCode runtime selection")
	ErrOpenCodeSelectionRevisionConflict = errors.New("OpenCode runtime selection revision conflict")
)

type OpenCodeFamily string

const (
	OpenCodeFamilyV1 OpenCodeFamily = "v1"
	OpenCodeFamilyV2 OpenCodeFamily = "v2"
)

type OpenCodeSource string

const (
	OpenCodeSourceManaged OpenCodeSource = "managed"
	OpenCodeSourceNative  OpenCodeSource = "native"
)

// OpenCodeSelection is the install-wide OpenCode distribution choice. The
// package and family are validated together so a stored record cannot redirect
// commands to an untrusted npm package.
type OpenCodeSelection struct {
	SchemaVersion         int            `json:"schema_version"`
	Family                OpenCodeFamily `json:"family"`
	Source                OpenCodeSource `json:"source"`
	Package               string         `json:"package"`
	SelectedVersion       string         `json:"selected_version,omitempty"`
	AppliedDefaultVersion string         `json:"applied_default_version"`
	Revision              uint64         `json:"revision"`
}

// OpenCodeRuntimeDefaults carries the two reviewed family defaults used for
// bootstrap and same-family reconciliation.
type OpenCodeRuntimeDefaults struct {
	V1Package string
	V1Version string
	V2Package string
	V2Version string
}

// OpenCodeBootstrapEvidence contains local facts that distinguish an existing
// installation from a fresh one. NativeFamily is empty when no supported
// native executable was found.
type OpenCodeBootstrapEvidence struct {
	NativeFamily OpenCodeFamily
	PriorUse     bool
}

// OpenCodeSelectionReader reads the authoritative OpenCode runtime choice.
type OpenCodeSelectionReader interface {
	GetOpenCodeSelection(context.Context) (OpenCodeSelection, bool, error)
}

// OpenCodeSelectionWriter saves the authoritative choice after candidate
// validation. expectedRevision is zero only when the record does not exist.
type OpenCodeSelectionWriter interface {
	SaveOpenCodeSelection(context.Context, uint64, OpenCodeSelection) error
}

// BootstrapOpenCode imports legacy state once, chooses v2 only for a fresh
// installation, reconciles the adopted family's reviewed default, and then
// removes legacy rows after the authoritative record is durable.
func (s *Store) BootstrapOpenCode(
	ctx context.Context,
	defaults OpenCodeRuntimeDefaults,
	evidence OpenCodeBootstrapEvidence,
) (OpenCodeSelection, error) {
	if s == nil || s.settings == nil {
		return OpenCodeSelection{}, errSettingsMissing
	}
	if err := validateOpenCodeDefaults(defaults); err != nil {
		return OpenCodeSelection{}, err
	}

	selection, found, err := s.GetOpenCodeSelection(ctx)
	if err != nil {
		return OpenCodeSelection{}, err
	}
	if found {
		selection, err = s.reconcileOpenCodeDefault(ctx, selection, defaults)
		if err != nil {
			return OpenCodeSelection{}, err
		}
		if err := s.cleanupLegacyOpenCodeSelection(ctx); err != nil {
			return OpenCodeSelection{}, err
		}
		return selection, nil
	}

	legacySelection, selectionFound, err := s.settings.Get(ctx, selectionKey("opencode-acp"))
	if err != nil {
		return OpenCodeSelection{}, fmt.Errorf("read legacy OpenCode selection: %w", err)
	}
	legacyMarker, markerFound, err := s.settings.Get(ctx, defaultGenerationKey("opencode-acp"))
	if err != nil {
		return OpenCodeSelection{}, fmt.Errorf("read legacy OpenCode default marker: %w", err)
	}

	selection, err = selectInitialOpenCodeRuntime(defaults, evidence, legacySelection, selectionFound, legacyMarker, markerFound)
	if err != nil {
		return OpenCodeSelection{}, err
	}
	if err := s.SaveOpenCodeSelection(ctx, 0, selection); err != nil {
		return OpenCodeSelection{}, fmt.Errorf("save initial OpenCode runtime selection: %w", err)
	}
	selection, err = s.reconcileOpenCodeDefault(ctx, selection, defaults)
	if err != nil {
		return OpenCodeSelection{}, err
	}
	if err := s.cleanupLegacyOpenCodeSelection(ctx); err != nil {
		return OpenCodeSelection{}, err
	}
	return selection, nil
}

func (s *Store) GetOpenCodeSelection(ctx context.Context) (OpenCodeSelection, bool, error) {
	if s == nil || s.settings == nil {
		return OpenCodeSelection{}, false, errSettingsMissing
	}
	raw, found, err := s.settings.Get(ctx, openCodeSelectionKey)
	if err != nil || !found {
		return OpenCodeSelection{}, found, err
	}
	var selection OpenCodeSelection
	if err := json.Unmarshal(raw, &selection); err != nil {
		return OpenCodeSelection{}, false, fmt.Errorf("%w: decode: %v", ErrInvalidOpenCodeSelection, err)
	}
	if err := validateOpenCodeSelection(selection); err != nil {
		return OpenCodeSelection{}, false, err
	}
	return selection, true, nil
}

func (s *Store) SaveOpenCodeSelection(ctx context.Context, expectedRevision uint64, selection OpenCodeSelection) error {
	if s == nil || s.settings == nil {
		return errSettingsMissing
	}
	current, found, err := s.GetOpenCodeSelection(ctx)
	if err != nil {
		return err
	}
	if (found && current.Revision != expectedRevision) || (!found && expectedRevision != 0) {
		return ErrOpenCodeSelectionRevisionConflict
	}
	if selection.Revision != expectedRevision+1 {
		return fmt.Errorf("%w: next revision must be %d", ErrOpenCodeSelectionRevisionConflict, expectedRevision+1)
	}
	if err := validateOpenCodeSelection(selection); err != nil {
		return err
	}
	raw, err := json.Marshal(selection)
	if err != nil {
		return fmt.Errorf("marshal OpenCode runtime selection: %w", err)
	}
	return s.settings.Save(ctx, openCodeSelectionKey, raw)
}

func (s *Store) reconcileOpenCodeDefault(
	ctx context.Context,
	selection OpenCodeSelection,
	defaults OpenCodeRuntimeDefaults,
) (OpenCodeSelection, error) {
	currentDefault := openCodeDefaultVersion(selection.Family, defaults)
	if selection.AppliedDefaultVersion == currentDefault {
		return selection, nil
	}
	updated := selection
	// A new reviewed default replaces an older imported default and clears its
	// version override so the selected family follows the shipped runtime.
	updated.SelectedVersion = ""
	updated.AppliedDefaultVersion = currentDefault
	updated.Revision++
	if err := s.SaveOpenCodeSelection(ctx, selection.Revision, updated); err != nil {
		return OpenCodeSelection{}, fmt.Errorf("reconcile OpenCode family default: %w", err)
	}
	return updated, nil
}

func (s *Store) cleanupLegacyOpenCodeSelection(ctx context.Context) error {
	for _, key := range []string{selectionKey("opencode-acp"), defaultGenerationKey("opencode-acp")} {
		_, found, err := s.settings.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("read legacy OpenCode setting %q for cleanup: %w", key, err)
		}
		if !found {
			continue
		}
		if err := s.settings.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete legacy OpenCode setting %q: %w", key, err)
		}
	}
	return nil
}

func selectInitialOpenCodeRuntime(
	defaults OpenCodeRuntimeDefaults,
	evidence OpenCodeBootstrapEvidence,
	legacySelection []byte,
	selectionFound bool,
	legacyMarker []byte,
	markerFound bool,
) (OpenCodeSelection, error) {
	if evidence.NativeFamily != "" {
		if _, ok := openCodePackageForFamily(evidence.NativeFamily); !ok {
			return OpenCodeSelection{}, fmt.Errorf("%w: unsupported native family %q", ErrInvalidOpenCodeSelection, evidence.NativeFamily)
		}
		return newOpenCodeSelection(
			evidence.NativeFamily,
			OpenCodeSourceNative,
			openCodeDefaultVersion(evidence.NativeFamily, defaults),
			defaults,
		), nil
	}
	if selectionFound {
		var selection Selection
		if err := json.Unmarshal(legacySelection, &selection); err != nil {
			return OpenCodeSelection{}, fmt.Errorf("%w: decode legacy selection: %v", ErrInvalidOpenCodeSelection, err)
		}
		if selection.Package != OpenCodeV1Package {
			return OpenCodeSelection{}, fmt.Errorf("%w: unsupported legacy package %q", ErrInvalidOpenCodeSelection, selection.Package)
		}
		if err := validateFamilyVersion(OpenCodeFamilyV1, selection.Version); err != nil {
			return OpenCodeSelection{}, fmt.Errorf("%w: legacy selection: %v", ErrInvalidOpenCodeSelection, err)
		}
		result := newOpenCodeSelection(OpenCodeFamilyV1, OpenCodeSourceManaged, defaults.V1Version, defaults)
		result.SelectedVersion = selection.Version
		result.AppliedDefaultVersion = legacyOpenCodeAppliedDefault(legacyMarker, markerFound, defaults)
		return result, nil
	}
	if markerFound {
		selection := newOpenCodeSelection(OpenCodeFamilyV1, OpenCodeSourceManaged, defaults.V1Version, defaults)
		selection.AppliedDefaultVersion = legacyOpenCodeAppliedDefault(legacyMarker, markerFound, defaults)
		return selection, nil
	}
	if evidence.PriorUse {
		return newOpenCodeSelection(OpenCodeFamilyV1, OpenCodeSourceManaged, defaults.V1Version, defaults), nil
	}
	return newOpenCodeSelection(OpenCodeFamilyV2, OpenCodeSourceManaged, defaults.V2Version, defaults), nil
}

func legacyOpenCodeAppliedDefault(
	legacyMarker []byte,
	markerFound bool,
	defaults OpenCodeRuntimeDefaults,
) string {
	var marker appliedDefaultGeneration
	if markerFound && json.Unmarshal(legacyMarker, &marker) == nil &&
		marker.Package == defaults.V1Package && validateFamilyVersion(OpenCodeFamilyV1, marker.Version) == nil {
		return marker.Version
	}
	return defaults.V1Version
}

func newOpenCodeSelection(
	family OpenCodeFamily,
	source OpenCodeSource,
	defaultVersion string,
	defaults OpenCodeRuntimeDefaults,
) OpenCodeSelection {
	packageName := defaults.V1Package
	if family == OpenCodeFamilyV2 {
		packageName = defaults.V2Package
	}
	return OpenCodeSelection{
		SchemaVersion:         1,
		Family:                family,
		Source:                source,
		Package:               packageName,
		AppliedDefaultVersion: defaultVersion,
		Revision:              1,
	}
}

func validateOpenCodeDefaults(defaults OpenCodeRuntimeDefaults) error {
	if defaults.V1Package != OpenCodeV1Package || defaults.V2Package != OpenCodeV2Package {
		return fmt.Errorf("%w: OpenCode packages must match the trusted family allowlist", ErrInvalidOpenCodeSelection)
	}
	if err := validateFamilyVersion(OpenCodeFamilyV1, defaults.V1Version); err != nil {
		return fmt.Errorf("%w: invalid v1 default: %v", ErrInvalidOpenCodeSelection, err)
	}
	if err := validateFamilyVersion(OpenCodeFamilyV2, defaults.V2Version); err != nil {
		return fmt.Errorf("%w: invalid v2 default: %v", ErrInvalidOpenCodeSelection, err)
	}
	return nil
}

func validateOpenCodeSelection(selection OpenCodeSelection) error {
	if selection.SchemaVersion != 1 || selection.Revision == 0 {
		return fmt.Errorf("%w: unsupported schema version or revision", ErrInvalidOpenCodeSelection)
	}
	packageName, ok := openCodePackageForFamily(selection.Family)
	if !ok || selection.Package != packageName {
		return fmt.Errorf("%w: family and package do not match", ErrInvalidOpenCodeSelection)
	}
	if selection.Source != OpenCodeSourceManaged && selection.Source != OpenCodeSourceNative {
		return fmt.Errorf("%w: unknown runtime source %q", ErrInvalidOpenCodeSelection, selection.Source)
	}
	if err := validateFamilyVersion(selection.Family, selection.AppliedDefaultVersion); err != nil {
		return fmt.Errorf("%w: invalid applied default: %v", ErrInvalidOpenCodeSelection, err)
	}
	if selection.Source == OpenCodeSourceNative && selection.SelectedVersion != "" {
		return fmt.Errorf("%w: native runtimes cannot have a managed version override", ErrInvalidOpenCodeSelection)
	}
	if selection.SelectedVersion != "" {
		if err := validateFamilyVersion(selection.Family, selection.SelectedVersion); err != nil {
			return fmt.Errorf("%w: invalid selected version: %v", ErrInvalidOpenCodeSelection, err)
		}
	}
	return nil
}

func openCodePackageForFamily(family OpenCodeFamily) (string, bool) {
	switch family {
	case OpenCodeFamilyV1:
		return OpenCodeV1Package, true
	case OpenCodeFamilyV2:
		return OpenCodeV2Package, true
	default:
		return "", false
	}
}

func validateFamilyVersion(family OpenCodeFamily, version string) error {
	parsed, err := ParseStableVersion(version)
	if err != nil {
		return err
	}
	expectedMajor, ok := ExpectedMajorForPackageForFamily(family)
	if !ok || parsed.Major() != expectedMajor {
		return fmt.Errorf("version %q does not belong to family %q", version, family)
	}
	return nil
}

func ExpectedMajorForPackageForFamily(family OpenCodeFamily) (uint64, bool) {
	packageName, ok := openCodePackageForFamily(family)
	if !ok {
		return 0, false
	}
	return ExpectedMajorForPackage(packageName)
}

func openCodeDefaultVersion(family OpenCodeFamily, defaults OpenCodeRuntimeDefaults) string {
	if family == OpenCodeFamilyV1 {
		return defaults.V1Version
	}
	return defaults.V2Version
}
