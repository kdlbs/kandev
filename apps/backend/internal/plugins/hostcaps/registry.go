// Package hostcaps is the closed registry of opt-in host capabilities that an
// executor provider plugin can require. It imports nothing from the lifecycle
// or store packages so the manifest validator, the lifecycle gate, and the
// plugin status read all use it without a cycle.
package hostcaps

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Entry is one capability row. Versions are integers; a provider requires an
// exact version inside [Min, Max].
type Entry struct {
	Name         string
	Min          int
	Max          int
	IntroducedIn string
	DeprecatedIn string
	RetireIn     string
	Replacement  string
	Requires     []string
}

// Requirement is one element of a manifest's requires_host_capabilities list.
type Requirement struct {
	Name    string
	Version int
}

// RetiredPair is a capability name and version that no host accepts any more.
// Its identity is never reused for a different meaning.
type RetiredPair struct {
	Name      string
	Version   int
	RetiredIn string
}

// Key is the stable identity used by the append-only golden file.
func (p RetiredPair) Key() string { return p.Name + "@" + strconv.Itoa(p.Version) }

const (
	firstRelease       = "0.99.0"
	destroyCheckpoints = "destroy_checkpoints"
	// TunnelTransport is the capability that lets a provider return a host_tunnel lease.
	TunnelTransport = "tunnel_transport"
)

var registry = []Entry{
	{Name: "workspace_descriptor", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "repository_intent", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: TunnelTransport, Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "api_url_forward", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "drain_receipt", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: destroyCheckpoints, Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "durable_bootstrap", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "admission_context", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "pending_placement", Min: 1, Max: 1, IntroducedIn: firstRelease,
		Requires: []string{destroyCheckpoints, "provision_refusal", "durable_bootstrap"}},
	{Name: "provider_operations", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "provider_maintenance", Min: 1, Max: 1, IntroducedIn: firstRelease, Requires: []string{"provider_operations"}},
	{Name: "launch_policy", Min: 1, Max: 1, IntroducedIn: firstRelease, Requires: []string{"workspace_descriptor"}},
	{Name: "preparation_hints", Min: 1, Max: 1, IntroducedIn: firstRelease, Requires: []string{"repository_intent"}},
	{Name: "typed_profile_values", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "secret_references", Min: 1, Max: 1, IntroducedIn: firstRelease},
	{Name: "provision_refusal", Min: 1, Max: 1, IntroducedIn: firstRelease, Requires: []string{destroyCheckpoints}},
}

// retired is append-only. testdata/retired.golden holds the previous release's
// list and the registry test fails when a pair is removed or reused.
var retired []RetiredPair

// Registry returns a copy of the capability rows in registry order.
func Registry() []Entry {
	out := make([]Entry, len(registry))
	for i, entry := range registry {
		entry.Requires = slices.Clone(entry.Requires)
		out[i] = entry
	}
	return out
}

// Retired returns a copy of the append-only retired list.
func Retired() []RetiredPair { return slices.Clone(retired) }

// Lookup returns the row for name.
func Lookup(name string) (Entry, bool) {
	for _, entry := range registry {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

// ValidateRequirements checks names, duplicates, versions, retirement, and
// dependency closure. It is a pure function of the registry and its argument.
func ValidateRequirements(reqs []Requirement) []error {
	if len(reqs) > len(registry) {
		return []error{fmt.Errorf("requires_host_capabilities must list at most %d capabilities", len(registry))}
	}
	var errs []error
	declared := make(map[string]bool, len(reqs))
	for _, req := range reqs {
		entry, known := Lookup(req.Name)
		switch {
		case declared[req.Name]:
			errs = append(errs, fmt.Errorf("duplicate capability %q", req.Name))
			continue
		case req.Version < 1:
			errs = append(errs, fmt.Errorf("capability %q version must be an integer of at least 1", req.Name))
		}
		declared[req.Name] = true
		if pair, isRetired := retiredPair(req.Name, req.Version); isRetired {
			errs = append(errs, fmt.Errorf("capability_retired: capability %q version %d was retired in %s", pair.Name, pair.Version, pair.RetiredIn))
			continue
		}
		if !known {
			errs = append(errs, fmt.Errorf("unknown capability %q", req.Name))
			continue
		}
		if req.Version >= 1 && (req.Version < entry.Min || req.Version > entry.Max) {
			errs = append(errs, fmt.Errorf("capability %q version %d is outside the supported range %d-%d", req.Name, req.Version, entry.Min, entry.Max))
		}
	}
	for _, req := range reqs {
		entry, known := Lookup(req.Name)
		if !known {
			continue
		}
		for _, dependency := range entry.Requires {
			if !declared[dependency] {
				errs = append(errs, fmt.Errorf("capability %q requires %q", req.Name, dependency))
			}
		}
	}
	return errs
}

func retiredPair(name string, version int) (RetiredPair, bool) {
	for _, pair := range retired {
		if pair.Name == name && pair.Version == version {
			return pair, true
		}
	}
	return RetiredPair{}, false
}

// HighestIntroducedIn returns the latest introduced_in release among the
// declared capabilities, or "" when none is declared. Releases compare as
// dotted integers.
func HighestIntroducedIn(reqs []Requirement) string {
	highest := ""
	for _, req := range reqs {
		entry, ok := Lookup(req.Name)
		if !ok {
			continue
		}
		if highest == "" || compareDotted(entry.IntroducedIn, highest) > 0 {
			highest = entry.IntroducedIn
		}
	}
	return highest
}

// DeprecationWarnings names the replacement of every declared deprecated
// version. A deprecated version still installs.
func DeprecationWarnings(reqs []Requirement) []string {
	var warnings []string
	for _, req := range reqs {
		entry, ok := Lookup(req.Name)
		if !ok || entry.DeprecatedIn == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("capability %q version %d is deprecated since %s and retires in %s; use %s",
			req.Name, req.Version, entry.DeprecatedIn, entry.RetireIn, entry.Replacement))
	}
	return warnings
}

// CheckDeprecationWindows fails when a row retires fewer than two minor
// releases after its deprecation.
func CheckDeprecationWindows() error {
	for _, entry := range registry {
		if entry.DeprecatedIn == "" {
			continue
		}
		if entry.RetireIn == "" || minorDistance(entry.DeprecatedIn, entry.RetireIn) < 2 {
			return fmt.Errorf("capability %q must stay accepted for two releases after deprecation in %s", entry.Name, entry.DeprecatedIn)
		}
	}
	return nil
}

func minorDistance(from, to string) int {
	a, b := dotted(from), dotted(to)
	if a[0] != b[0] {
		return (b[0]-a[0])*1000 + b[1] - a[1]
	}
	return b[1] - a[1]
}

func compareDotted(a, b string) int {
	left, right := dotted(a), dotted(b)
	for i := range left {
		if left[i] != right[i] {
			return left[i] - right[i]
		}
	}
	return 0
}

func dotted(version string) [3]int {
	var out [3]int
	for i, part := range strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3) {
		out[i], _ = strconv.Atoi(part)
	}
	return out
}
