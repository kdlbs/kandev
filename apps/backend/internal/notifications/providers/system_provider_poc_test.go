package providers

import (
	"fmt"
	"strings"
	"testing"
)

// TestEscapePowerShell_LeavesSubExpressionIntact_PoC demonstrates the
// LOW-severity PowerShell sub-expression injection in playWindowsSound.
//
// escapePowerShell only backtick-escapes the double-quote character. It does
// NOT neutralize the `$(...)` sub-expression syntax (nor a bare backtick),
// which PowerShell evaluates inside a double-quoted string. Because
// playWindowsSound interpolates the (attacker-influenced) SoundFile path into
// a `-c` script string, a `$(...)` payload in the path is evaluated as code.
//
// SoundFile is operator per-user config (self-inflicted, hence Low), but it is
// still a shell-injection sink. This test asserts the CURRENT unsafe behavior;
// the fix flips it (see TestEscapePowerShell_NeutralizesSubExpression).
func TestEscapePowerShell_LeavesSubExpressionIntact_PoC(t *testing.T) {
	// A sound path carrying a PowerShell sub-expression payload.
	payload := `C:\sounds\a$(Remove-Item -Recurse C:\important).wav`

	escaped := escapePowerShell(payload)

	// The sub-expression survives escaping untouched — proof the escaper does
	// not neutralize `$(...)`.
	if !strings.Contains(escaped, `$(Remove-Item -Recurse C:\important)`) {
		t.Fatalf("PoC expected sub-expression to survive escaping, got %q", escaped)
	}

	// Reconstruct the exact `-c` script playWindowsSound would run.
	script := fmt.Sprintf(`(New-Object Media.SoundPlayer "%s").PlaySync()`, escaped)

	// The generated script embeds a live `$(...)` sub-expression inside the
	// double-quoted string — PowerShell would evaluate it before PlaySync().
	if !strings.Contains(script, "$(Remove-Item") {
		t.Fatalf("PoC expected injection sink in generated -c script, got %q", script)
	}
	t.Logf("PoC: playWindowsSound would run: powershell.exe -c %s", script)
}
