package providers

import (
	"fmt"
	"strings"
	"testing"
)

// breakoutPayload is an attacker-controlled task title/body arriving verbatim
// from an external integration (Sentry/Linear/Jira issue title). The leading
// `\"` defeats quote-only escaping: escaping the quote turns `\"` into `\\"`,
// which AppleScript reads as one escaped backslash followed by the REAL closing
// delimiter — so the `display notification "..."` literal terminates early and
// the remaining text runs as code. `do shell script (...)` spells `touch
// /tmp/pwn` via quote-less ASCII-character concatenation; `--` comments out the
// trailing ` with title "..."`.
const breakoutPayload = `\"` + "\n" +
	`do shell script (ASCII character 116 & ASCII character 111 & ASCII character 117 & ASCII character 99 & ASCII character 104 & ASCII character 32 & ASCII character 47 & ASCII character 116 & ASCII character 109 & ASCII character 112 & ASCII character 47 & ASCII character 112 & ASCII character 119 & ASCII character 110)` + "\n" +
	`--`

// legacyBuildAppleScript reproduces the vulnerable pre-fix implementation
// (quote-only escaping, string interpolation) so the exploit stays documented
// and the regression is provable in-tree.
func legacyBuildAppleScript(title, body string) string {
	escapedTitle := strings.ReplaceAll(title, `"`, `\"`)
	escapedBody := strings.ReplaceAll(body, `"`, `\"`)
	return fmt.Sprintf(`display notification "%s" with title "%s"`, escapedBody, escapedTitle)
}

// applescriptStringLiteral faithfully simulates how AppleScript scans a
// double-quoted string literal starting at src[start] (the opening quote).
// Inside the literal a backslash escapes the next character (\" -> ", \\ -> \);
// an UNescaped " terminates it. Returns the decoded content and the index just
// past the closing quote. Whatever follows that index is parsed as CODE.
func applescriptStringLiteral(src string, start int) (content string, end int, closed bool) {
	var b strings.Builder
	i := start + 1 // skip opening quote
	for i < len(src) {
		c := src[i]
		if c == '\\' && i+1 < len(src) {
			b.WriteByte(src[i+1])
			i += 2
			continue
		}
		if c == '"' {
			return b.String(), i + 1, true
		}
		b.WriteByte(c)
		i++
	}
	return b.String(), i, false
}

// TestPoC_LegacyAppleScriptBreakout documents the vulnerability: with the old
// interpolating builder, the crafted payload breaks out of the notification
// string literal and exposes `do shell script` as executable AppleScript.
func TestPoC_LegacyAppleScriptBreakout(t *testing.T) {
	script := legacyBuildAppleScript("Kandev", breakoutPayload)
	t.Logf("legacy osascript -e argument:\n%s", script)

	const opener = `display notification "`
	if !strings.HasPrefix(script, opener) {
		t.Fatalf("unexpected prefix: %q", script)
	}

	content, end, closed := applescriptStringLiteral(script, len(opener)-1)
	if !closed {
		t.Fatalf("literal never closed: %q", script)
	}
	remainder := script[end:]

	// The body literal terminates prematurely (decodes to a lone backslash) and
	// `do shell script` lands OUTSIDE the string, as code.
	if strings.Contains(content, "do shell script") {
		t.Fatalf("payload stayed inside literal — not the bug we expect: %q", content)
	}
	trimmed := strings.TrimLeft(remainder, "\n\r\t ")
	if !strings.HasPrefix(trimmed, "do shell script") {
		t.Fatalf("expected exposed `do shell script`, got remainder=%q", remainder)
	}
	t.Log("CONFIRMED (legacy): display notification literal closes early; " +
		"`do shell script` would execute on the host. RCE reproduced.")
}

// TestOsascriptNotifyArgs_NoInjection is the regression guard: the fixed
// argv-based builder must pass title/body as opaque `run` arguments, never
// interpolated into any `-e` AppleScript source. This FAILS against the legacy
// interpolating implementation and PASSES after the fix.
func TestOsascriptNotifyArgs_NoInjection(t *testing.T) {
	title := "Kandev"
	args := osascriptNotifyArgs(title, breakoutPayload)

	// Split the `-e` script fragments from the trailing run arguments.
	var scriptFragments, runArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-e" && i+1 < len(args) {
			scriptFragments = append(scriptFragments, args[i+1])
			i++
			continue
		}
		runArgs = append(runArgs, args[i])
	}

	// 1. The untrusted title/body must appear ONLY as trailing run arguments,
	//    byte-for-byte, and never embedded in any AppleScript source fragment.
	if len(runArgs) != 2 || runArgs[0] != title || runArgs[1] != breakoutPayload {
		t.Fatalf("title/body not passed as opaque run args: %#v", runArgs)
	}
	for _, frag := range scriptFragments {
		if strings.Contains(frag, "do shell script") {
			t.Fatalf("attacker text leaked into AppleScript source: %q", frag)
		}
		if strings.Contains(frag, "Kandev") || strings.Contains(frag, `\"`) {
			t.Fatalf("title/body interpolated into AppleScript source: %q", frag)
		}
	}

	// 2. The script fragments must reference argv, not interpolated literals.
	joined := strings.Join(scriptFragments, "\n")
	if !strings.Contains(joined, "item 2 of argv") || !strings.Contains(joined, "item 1 of argv") {
		t.Fatalf("expected argv-referencing display notification, got: %q", joined)
	}
	t.Logf("safe osascript args: %#v", args)
}
