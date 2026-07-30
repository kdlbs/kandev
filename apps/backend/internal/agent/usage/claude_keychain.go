package usage

import (
	"os/exec"
	"runtime"
	"strings"
)

// ClaudeDefaultKeychainService is the login-keychain item Claude Code writes
// for the default profile (CLAUDE_CONFIG_DIR unset, i.e. ~/.claude).
const ClaudeDefaultKeychainService = "Claude Code-credentials"

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
// Returns nil (never an error) on non-darwin platforms, when the item is
// absent, or when the user denies keychain access — every one of those is a
// legitimate "fall back to the file" signal rather than a failure to report.
// Indirected so tests can substitute a fake without shelling out to `security`
// or depending on the developer's own login keychain.
var readKeychainCredentials = readKeychainCredentialsFromSecurity

func readKeychainCredentialsFromSecurity(service string) []byte {
	if runtime.GOOS != "darwin" || service == "" {
		return nil
	}
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return []byte(trimmed)
}
