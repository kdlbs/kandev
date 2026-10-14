package hostcli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// VersionTimeout bounds one `--version` invocation.
const VersionTimeout = 5 * time.Second

// ErrVersionNotFound reports output without a MAJOR.MINOR.PATCH token.
var ErrVersionNotFound = errors.New("no version found in command output")

var versionPattern = regexp.MustCompile(`(?:^|[^0-9A-Za-z.])v?(\d+\.\d+\.\d+)(?:[^0-9A-Za-z.]|$)`)

// ParseVersion extracts the first MAJOR.MINOR.PATCH token from the output of
// a version command. Prerelease and build suffixes are dropped so the result
// compares against npm's stable version list.
//
//	"2.1.220 (Claude Code)" -> "2.1.220"
//	"codex-cli 0.155.1"     -> "0.155.1"
func ParseVersion(output string) (string, error) {
	match := versionPattern.FindStringSubmatch(output)
	if match == nil {
		return "", ErrVersionNotFound
	}
	return match[1], nil
}

// DetectVersion runs the CLI's version command with a bounded timeout and
// parses the result. The error message is short enough to show in the UI.
func DetectVersion(ctx context.Context, runner Runner, path string, args []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(ctx, VersionTimeout)
	defer cancel()
	argv := append([]string{path}, args...)
	output, err := runner.Output(ctx, argv)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("version check timed out after %s", VersionTimeout)
	}
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return "", ErrNotInstalled
		}
		// A non-zero exit can still print a version (some CLIs exit 1 when
		// an update check fails); prefer the parsed value when present.
		if version, parseErr := ParseVersion(output); parseErr == nil {
			return version, nil
		}
		return "", fmt.Errorf("version command failed: %s", summarizeOutput(output, err))
	}
	version, err := ParseVersion(output)
	if err != nil {
		return "", fmt.Errorf("version command printed no version: %s", summarizeOutput(output, nil))
	}
	return version, nil
}

// summarizeOutput returns the first non-empty line of output (or the error)
// so a failure reason fits on one line in the settings card.
func summarizeOutput(output string, err error) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			return line
		}
	}
	if err != nil {
		return err.Error()
	}
	return "empty output"
}
