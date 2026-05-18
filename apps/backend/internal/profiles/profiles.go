// Package profiles owns the prod / dev / e2e runtime defaults declared
// in profiles.yaml. The file lives at the repo root (symlinked to
// profiles.yaml in this package) and is //go:embed-ed into the binary,
// so release artifacts always ship with whatever was checked in.
//
// At startup the backend calls ApplyProfile which:
//
//  1. Detects the active profile from env (KANDEV_E2E_MOCK >
//     KANDEV_DEBUG_DEV_MODE > prod).
//  2. Walks every leaf of profiles.yaml.
//  3. For each leaf, picks the value for the active profile (falling
//     back to prod when a leaf doesn't declare one for the active env).
//  4. Calls os.Setenv ONLY when the var is not already present in the
//     environment, and skips empty-string values (which mean "leave
//     this var unset").
//
// Steps 3 and 4 give the precedence chain we want:
//
//	shell env / launcher env  >  profiles.yaml  >  Go zero values
//
// A self-hoster setting KANDEV_FEATURES_OFFICE=true in their k8s
// manifest wins over the YAML's prod default of "false". A playwright
// spec setting AGENTCTL_AUTO_APPROVE_PERMISSIONS=false (the inverse of
// the e2e default) likewise wins — because the spec sets it before
// spawning the backend, ApplyProfile sees it as already-set and skips.
//
// See docs/decisions/0007-runtime-feature-flags.md.
package profiles

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed profiles.yaml
var profilesYAML []byte

// Environment identifies the active runtime profile.
type Environment string

const (
	EnvProd Environment = "prod"
	EnvDev  Environment = "dev"
	EnvE2E  Environment = "e2e"
)

// DetectEnvironment picks the active profile from process env. e2e
// wins over dev because the playwright fixtures set KANDEV_E2E_MOCK
// while dev mode is often also on under e2e (the harness inherits the
// developer's env). The default is prod so production releases never
// pick up dev/e2e values just because something leaked in.
func DetectEnvironment() Environment {
	if isTruthy(os.Getenv("KANDEV_E2E_MOCK")) {
		return EnvE2E
	}
	if isTruthy(os.Getenv("KANDEV_DEBUG_DEV_MODE")) || isTruthy(os.Getenv("KANDEV_DEBUG_PPROF_ENABLED")) {
		return EnvDev
	}
	return EnvProd
}

// profilesFile is the parsed shape of profiles.yaml. Sections at the
// top level are purely for human grouping (features / mocks / debug /
// e2eTuning); the loader walks every leaf the same way.
//
//	section -> envVarName -> profileName -> value
type profilesFile map[string]map[string]map[string]string

// ApplyProfile reads the embedded profiles.yaml, resolves every leaf
// for the active profile (returned by DetectEnvironment), and writes
// the resulting env vars onto the current process — but only for vars
// that aren't already set, and only when the resolved value is
// non-empty. Returns the count of env vars actually written and the
// active profile, for logging.
//
// Safe to call from main() before any other env reads. Idempotent:
// calling twice is a no-op because the second call sees every var
// already set.
func ApplyProfile() (count int, env Environment, err error) {
	env = DetectEnvironment()
	file, err := parse(profilesYAML)
	if err != nil {
		return 0, env, err
	}
	for _, vars := range file {
		for name, perProfile := range vars {
			value := resolve(perProfile, env)
			if value == "" {
				// Empty means "leave unset". Skip even if the var is
				// also currently unset — we don't want to introduce a
				// noisy empty-string entry that os.LookupEnv would
				// then report as set.
				continue
			}
			if _, alreadySet := os.LookupEnv(name); alreadySet {
				continue
			}
			if err := os.Setenv(name, value); err != nil {
				return count, env, fmt.Errorf("setenv %q: %w", name, err)
			}
			count++
		}
	}
	return count, env, nil
}

// parse decodes profiles.yaml into the typed shape. A parse error
// here means someone committed a malformed profiles.yaml; fail loud
// so CI catches it before a release ships.
func parse(raw []byte) (profilesFile, error) {
	var f profilesFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse profiles.yaml: %w", err)
	}
	return f, nil
}

// resolve picks the value for env, falling back to prod when the leaf
// doesn't declare one. A leaf with no prod value at all is treated as
// empty — ApplyProfile will skip it.
func resolve(perProfile map[string]string, env Environment) string {
	if env != EnvProd {
		if v, ok := perProfile[string(env)]; ok {
			return v
		}
	}
	return perProfile[string(EnvProd)]
}

func isTruthy(s string) bool {
	switch s {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// FeatureFlagDefaults returns the resolved value of every entry under
// the `features:` section, for the active profile. The config package
// uses it to seed Viper's `features.*` keyspace via SetDefault so the
// typed Config struct populates correctly even before Viper's
// AutomaticEnv reads (e.g., in tests that build Config by hand).
//
// Keys are the *short* feature names, lowercased — e.g.
// KANDEV_FEATURES_OFFICE becomes "office". Values are the resolved
// strings ("true" / "false" / "").
func FeatureFlagDefaults() (map[string]string, error) {
	file, err := parse(profilesYAML)
	if err != nil {
		return nil, err
	}
	env := DetectEnvironment()
	out := map[string]string{}
	for name, perProfile := range file["features"] {
		short, ok := stripFeaturePrefix(name)
		if !ok {
			// Defensive: a non-conforming key under `features:` is a
			// commit bug, but skipping it is safer than panicking on
			// startup of an otherwise-working binary.
			continue
		}
		out[short] = resolve(perProfile, env)
	}
	return out, nil
}

const featurePrefix = "KANDEV_FEATURES_"

func stripFeaturePrefix(name string) (string, bool) {
	if len(name) <= len(featurePrefix) {
		return "", false
	}
	if name[:len(featurePrefix)] != featurePrefix {
		return "", false
	}
	return toLower(name[len(featurePrefix):]), true
}

// toLower is a tiny ASCII-only lowercaser; we don't import strings to
// keep the package's dependency surface minimal.
func toLower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// ProfilesYAML returns the raw embedded YAML so a future "what
// shipped with this binary" debug endpoint (and the test suite) can
// read the source without re-parsing it.
func ProfilesYAML() []byte {
	out := make([]byte, len(profilesYAML))
	copy(out, profilesYAML)
	return out
}

// SortedEnvVars returns every env-var name declared in profiles.yaml,
// sorted. Useful for tests that assert against the full list without
// hard-coding ordering.
func SortedEnvVars() ([]string, error) {
	file, err := parse(profilesYAML)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, vars := range file {
		for name := range vars {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
