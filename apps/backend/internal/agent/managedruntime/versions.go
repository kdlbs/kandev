package managedruntime

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var ErrInvalidStableVersion = errors.New("invalid stable semver")
var ErrNoStableVersions = errors.New("no stable runtime versions available")

// Operation is the backend-derived action for a managed runtime target.
type Operation string

const (
	OperationUpdate     Operation = "update"
	OperationRollback   Operation = "rollback"
	OperationRepair     Operation = "repair"
	OperationUpToDate   Operation = "up_to_date"
	OperationUseDefault Operation = "use_default"
)

// VersionOption is one selectable version in the bounded catalogue.
type VersionOption struct {
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

// Catalogue contains the bounded UI projection and the complete stable set
// used to authorize a submitted target.
type Catalogue struct {
	Versions []VersionOption
	Latest   string
	all      map[string]struct{}
}

var stableVersionPattern = regexp.MustCompile(
	`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`,
)

// ParseStableVersion accepts only strict, prerelease-free SemVer values.
func ParseStableVersion(value string) (*semver.Version, error) {
	if !stableVersionPattern.MatchString(value) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStableVersion, value)
	}
	parsed, err := semver.NewVersion(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrInvalidStableVersion, value, err)
	}
	if parsed.Prerelease() != "" {
		return nil, fmt.Errorf("%w: prerelease %q", ErrInvalidStableVersion, value)
	}
	return parsed, nil
}

// SortStableVersions validates, deduplicates, and sorts versions newest first.
func SortStableVersions(values []string) ([]string, error) {
	type parsedVersion struct {
		value  string
		parsed *semver.Version
	}
	unique := make(map[string]parsedVersion, len(values))
	for _, value := range values {
		parsed, err := ParseStableVersion(value)
		if err != nil {
			return nil, err
		}
		unique[value] = parsedVersion{value: value, parsed: parsed}
	}
	versions := make([]parsedVersion, 0, len(unique))
	for _, version := range unique {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		compare := versions[i].parsed.Compare(versions[j].parsed)
		if compare == 0 {
			return versions[i].value > versions[j].value
		}
		return compare > 0
	})
	out := make([]string, len(versions))
	for i, version := range versions {
		out[i] = version.value
	}
	return out, nil
}

// BuildCatalogue filters npm metadata to strict stable versions, orders it
// newest first, and keeps the newest 50 plus any active/current extras. The
// complete stable set remains private so callers can validate targets that
// are outside the bounded UI projection.
func BuildCatalogue(published []string, latest string, extras ...string) (Catalogue, error) {
	stable, seen := filterStableVersions(published)
	if len(stable) == 0 {
		return Catalogue{}, ErrNoStableVersions
	}
	ordered, err := SortStableVersions(stable)
	if err != nil {
		return Catalogue{}, err
	}
	latest = normalizedLatest(latest, ordered, seen)
	visibleValues := visibleCatalogueVersions(ordered, seen, extras...)
	visibleValues, err = SortStableVersions(visibleValues)
	if err != nil {
		return Catalogue{}, err
	}
	options := make([]VersionOption, 0, len(visibleValues))
	for _, value := range visibleValues {
		options = append(options, VersionOption{Version: value, Latest: value == latest})
	}
	return Catalogue{Versions: options, Latest: latest, all: seen}, nil
}

func filterStableVersions(values []string) ([]string, map[string]struct{}) {
	stable := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, err := ParseStableVersion(value); err != nil {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		stable = append(stable, value)
	}
	return stable, seen
}

func normalizedLatest(latest string, ordered []string, published map[string]struct{}) string {
	latest = strings.TrimSpace(latest)
	if _, err := ParseStableVersion(latest); err != nil {
		return ordered[0]
	}
	if _, exists := published[latest]; !exists {
		return ordered[0]
	}
	return latest
}

func visibleCatalogueVersions(
	ordered []string,
	published map[string]struct{},
	extras ...string,
) []string {
	visible := make(map[string]struct{}, 50+len(extras))
	for _, value := range ordered {
		if len(visible) >= 50 {
			break
		}
		visible[value] = struct{}{}
	}
	for _, value := range extras {
		if _, err := ParseStableVersion(value); err != nil {
			continue
		}
		if _, exists := published[value]; exists {
			visible[value] = struct{}{}
		}
	}
	values := make([]string, 0, len(visible))
	for value := range visible {
		values = append(values, value)
	}
	return values
}

// Has reports whether version is one of the published stable versions.
func (c Catalogue) Has(version string) bool {
	if _, err := ParseStableVersion(version); err != nil {
		return false
	}
	_, ok := c.all[version]
	return ok
}

// ClassifyOperation derives the operator-facing action. Empty or malformed
// observed values are treated as unknown and therefore require repair.
func ClassifyOperation(active, current, target string) (Operation, error) {
	targetVersion, err := ParseStableVersion(target)
	if err != nil {
		return "", err
	}
	activeVersion, activeOK := parseKnownVersion(active)
	currentVersion, currentOK := parseKnownVersion(current)
	if activeOK && currentOK && active == target && current == target {
		return OperationUpToDate, nil
	}
	if !currentOK {
		return OperationRepair, nil
	}
	baseValue := current
	baseVersion := currentVersion
	if activeOK {
		baseValue = active
		baseVersion = activeVersion
	}
	if target == baseValue {
		return OperationRepair, nil
	}
	compare := targetVersion.Compare(baseVersion)
	if compare > 0 {
		return OperationUpdate, nil
	}
	if compare < 0 {
		return OperationRollback, nil
	}
	return OperationRepair, nil
}

// ClassifyEffectiveOperation derives the operator action from the effective
// version used by future commands. An active selection is distinct from the
// effective version because choosing the shipped default removes that
// selection only after the candidate has been validated.
func ClassifyEffectiveOperation(
	useDefault bool,
	active string,
	effective string,
	current string,
	target string,
	defaultVersion string,
) (Operation, error) {
	targetParsed, err := ParseStableVersion(target)
	if err != nil {
		return "", err
	}
	if useDefault && active != "" && defaultVersion != "" {
		if _, err := ParseStableVersion(defaultVersion); err != nil {
			return "", err
		}
		if target == defaultVersion {
			return OperationUseDefault, nil
		}
	}
	effectiveParsed, effectiveOK := parseKnownVersion(effective)
	_, currentOK := parseKnownVersion(current)
	if !effectiveOK || !currentOK {
		return OperationRepair, nil
	}
	if effective == target && current == target {
		return OperationUpToDate, nil
	}
	compare := targetParsed.Compare(effectiveParsed)
	if compare > 0 {
		return OperationUpdate, nil
	}
	if compare < 0 {
		return OperationRollback, nil
	}
	return OperationRepair, nil
}

func parseKnownVersion(value string) (*semver.Version, bool) {
	if value == "" {
		return nil, false
	}
	parsed, err := ParseStableVersion(value)
	return parsed, err == nil
}
