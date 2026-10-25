package sandbox

import (
	"encoding/hex"
	"github.com/kandev/kandev/internal/common/processcontainment"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const LaunchCapability = "sandbox-launch.v1"

// Launch is an immutable core-owned envelope for a local execution host.
type Launch struct {
	ChildHome       string                       `json:"child_home,omitempty"`
	Containment     *processcontainment.Identity `json:"containment,omitempty"`
	MCPURL          string                       `json:"mcp_url,omitempty"`
	Version         int                          `json:"version"`
	AttemptID       string                       `json:"attempt_id"`
	Generation      uint64                       `json:"generation"`
	PolicyDigest    string                       `json:"policy_digest"`
	PackageDigest   string                       `json:"package_digest"`
	Root            string                       `json:"root"`
	Prefix          []string                     `json:"prefix"`
	Digests         map[string]string            `json:"digests"`
	EnvironmentKeys []string                     `json:"environment_keys,omitempty"`
}

func PrepareLaunch(launch *Launch, inner, environment []string) ([]string, []string, error) {
	if launch == nil {
		return slices.Clone(inner), slices.Clone(environment), nil
	}
	if err := launch.Validate(); err != nil {
		return nil, nil, err
	}
	if len(inner) == 0 || !filepath.IsAbs(inner[0]) {
		return nil, nil, ErrInvalidPolicy
	}
	for _, arg := range inner {
		if strings.ContainsRune(arg, 0) {
			return nil, nil, ErrInvalidPolicy
		}
	}
	final := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if !found || key == "" {
			return nil, nil, ErrInvalidPolicy
		}
		final[key] = value
	}
	env, err := IsolatedEnvironment(final, launch.EnvironmentKeys, runtimeDirectories(launch.Root))
	if err != nil {
		return nil, nil, err
	}
	return append(slices.Clone(launch.Prefix), inner...), env, nil
}

func (l *Launch) Validate() error {
	if l == nil || l.Version != 1 || l.AttemptID == "" || len(l.AttemptID) > 128 || strings.ContainsFunc(l.AttemptID, unicode.IsControl) || l.Generation == 0 ||
		!validLaunchDigest(l.PolicyDigest) || !validLaunchDigest(l.PackageDigest) || !validRuntimeDirectory(l.Root) {
		return ErrInvalidPolicy
	}
	if l.ChildHome != "" && !validRuntimeDirectory(l.ChildHome) {
		return ErrInvalidPolicy
	}
	if !validMCPRoute(l.MCPURL) {
		return ErrInvalidPolicy
	}
	if err := l.validatePrefix(); err != nil {
		return err
	}
	if _, err := canonicalEnvironmentKeys(l.EnvironmentKeys); err != nil {
		return err
	}
	return verifyRuntimeFiles(l.Root, l.Digests)
}

func validLaunchDigest(digest string) bool {
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32 && digest == strings.ToLower(digest)
}

func runtimeDirectories(root string) RuntimeDirectories {
	return RuntimeDirectories{Home: filepath.Join(root, "home"), Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Temp: filepath.Join(root, "tmp")}
}

// Clone retains the admitted envelope without sharing caller-owned collections.
func (l *Launch) Clone() *Launch {
	if l == nil {
		return nil
	}
	result := *l
	if l.Containment != nil {
		group := *l.Containment
		result.Containment = &group
	}
	result.Prefix = slices.Clone(l.Prefix)
	result.Digests = maps.Clone(l.Digests)
	result.EnvironmentKeys = slices.Clone(l.EnvironmentKeys)
	return &result
}

func (l *Launch) validatePrefix() error {
	if len(l.Prefix) == 0 || len(l.Prefix) > 128 || l.Prefix[0] != filepath.Join(l.Root, "runtime") {
		return ErrInvalidPolicy
	}
	for _, arg := range l.Prefix {
		if strings.ContainsRune(arg, 0) || !utf8.ValidString(arg) {
			return ErrInvalidPolicy
		}
	}
	return nil
}
