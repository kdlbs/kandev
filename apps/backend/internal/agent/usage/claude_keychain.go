package usage

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ClaudeDefaultKeychainService is the login-keychain item Claude Code writes
// for the default profile (CLAUDE_CONFIG_DIR unset, i.e. ~/.claude).
const ClaudeDefaultKeychainService = "Claude Code-credentials"

// claudeKeychainLookupTimeout bounds one `security` invocation.
//
// Reading an item owned by another application can raise a blocking
// authorization dialog, and a headless kandev has nobody to answer it. An
// unbounded lookup wedges the usage poll for as long as the dialog stands, so
// the lookup fails closed to the credentials file instead. Two seconds is far
// above a warm keychain read and far below any poll interval.
const claudeKeychainLookupTimeout = 2 * time.Second

// disableKeychainEnv opts a machine out of keychain reads entirely.
//
// Declining the dialog is a per-request answer, not a stored preference: the
// read falls back to the file, that fetch fails, and the failure cache brings
// the dialog straight back. Without an opt-out the only durable choice offered
// to a user who declines is "Always Allow" — the very thing they declined.
const disableKeychainEnv = "KANDEV_DISABLE_CLAUDE_KEYCHAIN"

// keychainGOOS and keychainCommand are seams so this file's behaviour is
// testable without shelling out to `security`, and so the non-darwin path can
// be exercised on the only platform where these tests actually run.
var (
	keychainGOOS = runtime.GOOS

	keychainCommand = func(ctx context.Context, service string) ([]byte, error) {
		// -w prints the secret to stdout; it is captured, never echoed.
		return exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-w").Output()
	}
)

// keychainDisabled reports whether the opt-out is set.
//
// Unset, empty, and an explicit false keep the keychain on; any other value
// turns it off. Erring toward "off" for an unparseable value is deliberate —
// someone who typed `=yes` clearly wants it disabled, and ignoring them would
// do the exact thing they were trying to prevent.
func keychainDisabled() bool {
	raw := os.Getenv(disableKeychainEnv)
	if raw == "" {
		return false
	}
	if parsed, err := strconv.ParseBool(raw); err == nil {
		return parsed
	}
	return true
}

// readKeychainCredentials returns the raw credentials JSON that Claude Code
// stores in the macOS login keychain, or nil when it cannot be read.
//
// This exists because on macOS the keychain — not ~/.claude/.credentials.json —
// holds the live token. Claude Code writes the file once at login and then
// stops updating it, so a machine whose CLI has been refreshing normally for
// weeks still has a file carrying a long-expired accessToken AND a refresh
// token that has since been rotated away. Refreshing from that file fails with
// `invalid_request_error` and no amount of retrying recovers it; the fix is to
// read the credential the CLI actually maintains.
//
// Returns nil (never an error) on non-darwin platforms, when the opt-out is
// set, when the item is absent, when the lookup exceeds its timeout, or when
// the user denies keychain access — every one of those is a legitimate "fall
// back to the file" signal rather than a failure to report.
// Indirected so tests can substitute a fake without shelling out to `security`
// or depending on the developer's own login keychain.
var readKeychainCredentials = readKeychainCredentialsFromSecurity

func readKeychainCredentialsFromSecurity(service string) []byte {
	if keychainGOOS != "darwin" || service == "" || keychainDisabled() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), claudeKeychainLookupTimeout)
	defer cancel()

	out, err := keychainCommand(ctx, service)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return []byte(trimmed)
}
