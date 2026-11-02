// Package copilotplugins owns the GitHub Copilot CLI plugin inventory,
// installation-evidence resolution, and native plugin-directory arguments.
//
// It is a provider boundary: it never imports lifecycle, settings controllers,
// browser DTOs, or Cursor MCP strategies. Native stdout, stderr, and
// configuration documents stay inside the package and are discarded after
// parsing.
package copilotplugins

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ProviderID is the concrete Copilot agent that owns profile plugin selection.
const ProviderID = "copilot-acp"

// Native source values observed in `copilot plugin list --json`.
const (
	SourceInstalled = "installed"
	SourceLive      = "live"
	SourceExternal  = "external"
	SourceBuiltin   = "builtin"
)

// Selection limits match the durable profile contract.
const (
	maxSelectedPlugins  = 256
	maxPluginIDBytes    = 512
	maxSelectionBytes   = 128 * 1024
	maxMarketplaceBytes = 512
)

var (
	// ErrInvalidPluginID reports a malformed or non-canonical identity.
	ErrInvalidPluginID = errors.New("invalid copilot plugin identity")
	// ErrInvalidSelection reports an invalid saved plugin selection.
	ErrInvalidSelection = errors.New("invalid copilot plugin selection")
)

// PluginID is the stable identity of one native plugin. Version and
// filesystem root are deliberately absent: selection survives upgrades and
// reinstalls, and roots are resolved for each new process.
type PluginID struct {
	Source      string
	Marketplace string
	Name        string
}

// String returns the canonical wire identity.
func (id PluginID) String() string {
	return encodeSegment(id.Source) + ":" + encodeSegment(id.Marketplace) + "/" + encodeSegment(id.Name)
}

// IsMountable reports whether this initial adapter can resolve the identity to
// a native plugin directory. External manual mounts and built-ins stay visible
// but unavailable.
func (id PluginID) IsMountable() bool {
	return id.Source == SourceInstalled || id.Source == SourceLive
}

// ParsePluginID validates and decodes one canonical stable identity.
func ParsePluginID(value string) (PluginID, error) {
	source, marketplace, name, err := parsePluginIDSegments(value)
	if err != nil {
		return PluginID{}, err
	}
	if len(marketplace) > maxMarketplaceBytes || unsafeSegment(marketplace, true) {
		return PluginID{}, ErrInvalidPluginID
	}
	if name == "" || len(name) > maxPluginIDBytes || unsafeSegment(name, false) {
		return PluginID{}, ErrInvalidPluginID
	}
	return PluginID{Source: source, Marketplace: marketplace, Name: name}, nil
}

func parsePluginIDSegments(value string) (string, string, string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", "", "", ErrInvalidPluginID
	}
	sourceRaw, rest, ok := strings.Cut(value, ":")
	if !ok || sourceRaw == "" || rest == "" {
		return "", "", "", ErrInvalidPluginID
	}
	source, err := decodeSegment(sourceRaw)
	if err != nil || !validSourceSegment(source) {
		return "", "", "", ErrInvalidPluginID
	}
	marketplaceRaw, nameRaw, ok := strings.Cut(rest, "/")
	if !ok || nameRaw == "" || strings.Contains(nameRaw, "/") {
		return "", "", "", ErrInvalidPluginID
	}
	marketplace, err := decodeSegment(marketplaceRaw)
	if err != nil {
		return "", "", "", ErrInvalidPluginID
	}
	name, err := decodeSegment(nameRaw)
	if err != nil {
		return "", "", "", ErrInvalidPluginID
	}
	return source, marketplace, name, nil
}

// NewPluginID builds a canonical identity from parsed native row values.
func NewPluginID(source, marketplace, name string) (PluginID, error) {
	return ParsePluginID(
		encodeSegment(source) + ":" + encodeSegment(marketplace) + "/" + encodeSegment(name),
	)
}

// ValidateSelection checks the saved preference's count, byte bounds,
// canonical identity shape, and duplicate set semantics.
func ValidateSelection(ids []string) error {
	if len(ids) > maxSelectedPlugins {
		return fmt.Errorf("%w: cannot contain more than %d identities", ErrInvalidSelection, maxSelectedPlugins)
	}
	seen := make(map[string]struct{}, len(ids))
	total := 0
	for index, id := range ids {
		if len(id) == 0 || len(id) > maxPluginIDBytes {
			return fmt.Errorf("%w: identity %d must be between 1 and %d bytes", ErrInvalidSelection, index, maxPluginIDBytes)
		}
		parsed, err := ParsePluginID(id)
		if err != nil {
			return fmt.Errorf("%w: identity %d is not canonical", ErrInvalidSelection, index)
		}
		if !parsed.IsMountable() {
			return fmt.Errorf("%w: identity %d is not mountable", ErrInvalidSelection, index)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: duplicate identity %q", ErrInvalidSelection, id)
		}
		seen[id] = struct{}{}
		total += len(id)
	}
	if total > maxSelectionBytes {
		return fmt.Errorf("%w: identities exceed %d bytes", ErrInvalidSelection, maxSelectionBytes)
	}
	return nil
}

// NormalizeSelection validates, de-duplicates, and sorts canonical identities
// for deterministic storage and comparison.
func NormalizeSelection(ids []string) ([]string, error) {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("%w: empty identity", ErrInvalidSelection)
		}
		unique[id] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for id := range unique {
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	if err := ValidateSelection(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// SortSelection returns canonical IDs in stable order. Invalid identities stay
// at the end so callers can still display a repair row.
func SortSelection(ids []string) []string {
	sorted := append([]string{}, ids...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, leftErr := ParsePluginID(sorted[i])
		right, rightErr := ParsePluginID(sorted[j])
		if leftErr != nil || rightErr != nil {
			if leftErr != nil && rightErr != nil {
				return sorted[i] < sorted[j]
			}
			return rightErr != nil
		}
		return left.String() < right.String()
	})
	return sorted
}

func encodeSegment(value string) string {
	return url.PathEscape(value)
}

func decodeSegment(value string) (string, error) {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", err
	}
	if encodeSegment(decoded) != value {
		return "", ErrInvalidPluginID
	}
	return decoded, nil
}

func validSourceSegment(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func unsafeSegment(value string, allowEmpty bool) bool {
	if value == "" {
		return !allowEmpty
	}
	if !utf8.ValidString(value) {
		return true
	}
	if value == "." || value == ".." {
		return true
	}
	if strings.HasPrefix(value, ".") && strings.Trim(value, ".") == "" {
		return true
	}
	for _, r := range value {
		if r == '/' || r == '\\' || r == '\x00' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
