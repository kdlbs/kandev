// Package startupdiag recognizes supported toolchain diagnostics emitted while
// an agent subprocess starts. It returns fixed safe markers and never returns
// the raw line, paths, or command arguments it inspects.
package startupdiag

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// ReasonMiseConfigUntrusted identifies the confirmed mise trust refusal.
	ReasonMiseConfigUntrusted = "mise_config_untrusted"
	// MiseConfigUntrustedDetail is the fixed safe explanation for that refusal.
	MiseConfigUntrustedDetail = "mise did not trust this workspace's configuration."

	maxDiagnosticLineBytes = 2048
	miseConfigPrefix       = "mise ERROR Config files in "
	miseConfigSuffix       = " are not trusted."
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// ProjectLine returns a fixed safe marker and its closed reason when line is
// exactly one supported complete toolchain diagnostic. The input path is
// examined only in memory and never returned.
func ProjectLine(line string) (marker string, reason string) {
	reason = ClassifyLine(line)
	if reason == "" {
		return "", ""
	}
	switch reason {
	case ReasonMiseConfigUntrusted:
		return MiseConfigUntrustedDetail, reason
	default:
		return "", ""
	}
}

// ClassifyLine returns the closed diagnostic reason for a complete supported
// toolchain signature, or an empty string when the line does not match.
func ClassifyLine(line string) string {
	line = strings.TrimSuffix(ansiEscape.ReplaceAllString(line, ""), "\r")
	if len(line) == 0 || len(line) > maxDiagnosticLineBytes || !utf8.ValidString(line) {
		return ""
	}
	if !strings.HasPrefix(line, miseConfigPrefix) || !strings.HasSuffix(line, miseConfigSuffix) {
		return ""
	}
	// The prefix ends with a space and the suffix starts with one. A line can
	// satisfy both predicates without any intervening path, so reject every
	// line whose overlap leaves no nonempty path before slicing.
	if len(line) <= len(miseConfigPrefix)+len(miseConfigSuffix) {
		return ""
	}
	path := line[len(miseConfigPrefix) : len(line)-len(miseConfigSuffix)]
	if path == "" || !strings.HasSuffix(path, "mise.toml") {
		return ""
	}
	if strings.ContainsAny(path, "\x00\r\n") {
		return ""
	}
	return ReasonMiseConfigUntrusted
}
