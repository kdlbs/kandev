package providers

import (
	"fmt"
	"strings"
	"testing"
)

// legacyEscapePowerShell reproduces the vulnerable pre-fix implementation
// (quote-only escaping) so the PoC below can demonstrate the injection against
// the unfixed function, mirroring how legacyBuildAppleScript documents the
// AppleScript exploit in system_provider_test.go.
func legacyEscapePowerShell(value string) string {
	return strings.ReplaceAll(value, `"`, "`\"")
}

// TestEscapePowerShell_SubExpressionInjection_PoC demonstrates the
// LOW-severity PowerShell sub-expression injection in playWindowsSound and
// proves the fix closes it.
//
// escapePowerShell used to only backtick-escape the double-quote character; it
// did NOT neutralize the `$(...)` sub-expression syntax, which PowerShell
// evaluates inside a double-quoted string. Because playWindowsSound
// interpolates the (operator-influenced) SoundFile path into a `-c` script
// string, a `$(...)` payload in the path was evaluated as code.
//
// The PoC uses a live-sub-expression detector (a `$(` with no escaping backtick
// in front) so it correctly FLIPS: it fires against the legacy escaper and is
// silenced by the current one.
func TestEscapePowerShell_SubExpressionInjection_PoC(t *testing.T) {
	// A sound path carrying a PowerShell sub-expression payload.
	payload := `C:\sounds\a$(Remove-Item -Recurse C:\important).wav`

	// Legacy escaping leaves a LIVE sub-expression — the vulnerability.
	legacy := legacyEscapePowerShell(payload)
	if !hasLiveSubExpression(legacy) {
		t.Fatalf("PoC expected legacy escaper to leave a live sub-expression, got %q", legacy)
	}
	legacyScript := fmt.Sprintf(`(New-Object Media.SoundPlayer "%s").PlaySync()`, legacy)
	t.Logf("PoC (legacy): playWindowsSound would run: powershell.exe -c %s", legacyScript)

	// The current escaper neutralizes it — the fix.
	fixed := escapePowerShell(payload)
	if hasLiveSubExpression(fixed) {
		t.Fatalf("current escaper still leaves a live sub-expression: %q", fixed)
	}
}

// hasLiveSubExpression reports whether s contains a `$(` that PowerShell would
// evaluate — i.e. a `$(` not defused by a preceding escaping backtick.
func hasLiveSubExpression(s string) bool {
	// Remove every escaped occurrence, then look for any remaining `$(`.
	return strings.Contains(strings.ReplaceAll(s, "`$(", ""), "$(")
}
