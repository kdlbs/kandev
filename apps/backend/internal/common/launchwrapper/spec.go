// Package launchwrapper prepares authorized profile wrappers on the execution host.
// Callers own authorization, child-command admission, and process lifetime.
package launchwrapper

import (
	"errors"
	"strings"
)

const (
	Version         = 1
	SourceProfile   = "profile"
	Capability      = "launch-wrapper.v1"
	WorkDir         = "{{kandev.work_dir}}"
	ManagedNPMDir   = "{{kandev.managed_npm_dir}}"
	referencePrefix = "{{kandev."
)

var (
	ErrInvalid     = errors.New("invalid launch wrapper configuration")
	ErrUnavailable = errors.New("launch wrapper executable is unavailable")
	ErrContext     = errors.New("launch wrapper runtime context is unavailable")
)

// Spec carries an unresolved, authorized prefix independently of the child argv.
type Spec struct {
	Version int      `json:"version"`
	Source  string   `json:"source"`
	Argv    []string `json:"argv"`
}

// ValidateTokens checks the prefix without filesystem access or expansion.
func ValidateTokens(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" || strings.HasPrefix(strings.TrimSpace(argv[0]), "-") {
		return ErrInvalid
	}
	for i, arg := range argv {
		if strings.ContainsRune(arg, 0) {
			return ErrInvalid
		}
		if !strings.Contains(arg, referencePrefix) {
			continue
		}
		if i == 0 {
			return ErrInvalid
		}
		if arg == WorkDir || arg == ManagedNPMDir || escapedReference(arg) != "" {
			continue
		}
		return ErrInvalid
	}
	return nil
}

func escapedReference(arg string) string {
	if strings.HasPrefix(arg, "{"+referencePrefix) && strings.HasSuffix(arg, "}}}") {
		inner := arg[1 : len(arg)-1]
		name := strings.TrimSuffix(strings.TrimPrefix(inner, referencePrefix), "}}")
		if name != "" && !strings.ContainsAny(name, "{}") {
			return inner
		}
	}
	return ""
}

// Validate checks an explicitly supplied wrapper envelope.
func (s Spec) Validate() error {
	if s.Version != Version || s.Source != SourceProfile {
		return ErrInvalid
	}
	return ValidateTokens(s.Argv)
}
