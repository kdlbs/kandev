package agents

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// shellMetacharacters are rejected in a command override so the value can only
// ever be a plain filesystem path, never something a shell would interpret.
const shellMetacharacters = "$`\"'\\;&|<>(){}[]*?!#~%^=,"

// ErrInvalidCommandOverride reports a command override that is not a plain
// absolute executable path.
var ErrInvalidCommandOverride = errors.New("invalid command override")

// ValidateCommandOverride checks an operator-configured preinstalled command.
// An empty value means "no override". Otherwise the value is one absolute,
// already-cleaned path with no whitespace, control characters or shell syntax.
// It is used as argv[0] directly: Kandev never runs it through a shell.
func ValidateCommandOverride(value string) error {
	if value == "" {
		return nil
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return fmt.Errorf("%w: must be a clean absolute path", ErrInvalidCommandOverride)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(shellMetacharacters, r) {
			return fmt.Errorf("%w: must be a plain path without whitespace or shell syntax", ErrInvalidCommandOverride)
		}
	}
	return nil
}

// OverrideCommand returns the launch command that runs the preinstalled
// executable in place of the npx invocation. The trailing runtime arguments are
// the same ones the npm package's bin receives. The caller validates path.
func (s ManagedNPMRuntimeSpec) OverrideCommand(path string) Command {
	return NewCommand(append([]string{path}, s.runtimeArgs()...)...)
}

// CommandFor returns the launch command for opts: the operator's preinstalled
// executable when opts.CommandOverride is set and valid, otherwise the managed
// npm command for the exact version. An invalid override never falls back to
// npx silently; the lifecycle rejects it before building the command.
func (s ManagedNPMRuntimeSpec) CommandFor(opts CommandOptions) Command {
	if opts.CommandOverride != "" && ValidateCommandOverride(opts.CommandOverride) == nil {
		return s.OverrideCommand(opts.CommandOverride)
	}
	return s.RuntimeCommand(opts.ManagedRuntimeVersion)
}
