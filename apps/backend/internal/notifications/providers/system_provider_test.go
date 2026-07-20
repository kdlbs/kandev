package providers

import (
	"fmt"
	"strings"
	"testing"
)

// doShellTouch is a quote-less `do shell script` invocation spelling
// `touch /tmp/pwn` via `ASCII character N` concatenation. It carries no literal
// `"`, so buildAppleScript's quote-escaping never touches it — the payload
// survives interpolation intact.
const doShellTouch = `do shell script (ASCII character 116 & ASCII character 111 & ASCII character 117 & ASCII character 99 & ASCII character 104 & ASCII character 32 & ASCII character 47 & ASCII character 116 & ASCII character 109 & ASCII character 112 & ASCII character 47 & ASCII character 112 & ASCII character 119 & ASCII character 110)`

// breakoutPayload is the MULTI-LINE attacker input (Sentry-style: exception
// text can contain newlines). The leading `\"` defeats quote-only escaping —
// escaping the quote turns `\"` into `\\"`, which AppleScript reads as one
// escaped backslash followed by the REAL closing delimiter, so the `display
// notification "..."` literal terminates early. The newline is a STATEMENT
// separator, so `do shell script (...)` runs as a second top-level statement;
// `--` comments out the trailing ` with title "..."`.
const breakoutPayload = `\"` + "\n" + doShellTouch + "\n" + `--`

// singleLineBreakoutPayload is the SINGLE-LINE attacker input (Jira summary /
// Linear title — typically no newline). It proves a newline is NOT required: the
// same `\"` break-out lands in code context, and instead of a second statement
// the payload injects an EXPRESSION — `& (do shell script (...))` — into the
// body-argument position. `do shell script` executes as a side effect of
// evaluating that expression and returns its stdout as text, which concatenates
// with the `\`-string; the whole line stays ONE valid `display notification …
// with title "Kandev"` statement. Expression injection needs no separator.
const singleLineBreakoutPayload = `\" & (` + doShellTouch + `)`

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

// TestPoC_SingleLineExpressionInjection settles the severity question raised in
// review: does the SINGLE-LINE case (Jira summary / Linear title, no newline)
// still achieve host code execution, or does it merely DoS the notification?
//
// It does execute. A newline (statement separator) is only needed if you want a
// SECOND statement. This payload instead injects an EXPRESSION into the existing
// `display notification <body>` argument: after the `\"` break-out, `& (do shell
// script (...))` concatenates the `\`-string with the TEXT that `do shell
// script` returns — evaluating that expression runs the command as a side
// effect. The line remains a single valid statement, so osascript compiles and
// executes it. Conclusion: single-line inputs are host RCE, not just DoS.
func TestPoC_SingleLineExpressionInjection(t *testing.T) {
	script := legacyBuildAppleScript("Kandev", singleLineBreakoutPayload)
	t.Logf("legacy osascript -e argument (single line):\n%s", script)

	// No newline anywhere: this is the Jira/Linear single-line shape.
	if strings.ContainsAny(script, "\n\r") {
		t.Fatalf("payload unexpectedly contains a line break: %q", script)
	}

	const opener = `display notification "`
	if !strings.HasPrefix(script, opener) {
		t.Fatalf("unexpected prefix: %q", script)
	}

	content, end, closed := applescriptStringLiteral(script, len(opener)-1)
	if !closed {
		t.Fatalf("literal never closed: %q", script)
	}
	remainder := script[end:]

	// The body literal still closes early (decodes to a lone backslash)...
	if content != `\` {
		t.Fatalf("expected body literal to decode to a lone backslash, got %q", content)
	}
	// ...and the very next token is the `&` concatenation operator — i.e. we are
	// in EXPRESSION context with NO statement separator in between.
	trimmed := strings.TrimLeft(remainder, " \t")
	if !strings.HasPrefix(trimmed, "&") {
		t.Fatalf("expected `&` expression injection immediately after literal, got %q", remainder)
	}
	if !strings.Contains(remainder, "do shell script") {
		t.Fatalf("expected `do shell script` in the injected expression, got %q", remainder)
	}
	// The trailing ` with title "Kandev"` keeps it one valid statement.
	if !strings.Contains(remainder, `with title "Kandev"`) {
		t.Fatalf("expected statement to close with the title argument, got %q", remainder)
	}
	t.Log("CONFIRMED (legacy, single line): no newline needed — `do shell script` " +
		"executes as an injected body expression. Single-line inputs are host RCE.")
}

// splitOsascriptArgs mirrors osascript's option parsing: `-e X` builds script
// fragments, `--` terminates option parsing so every following token is a
// positional `run` argument. Returns the collected script fragments and run
// args.
func splitOsascriptArgs(args []string) (scriptFragments, runArgs []string) {
	optsDone := false
	for i := 0; i < len(args); i++ {
		if optsDone {
			runArgs = append(runArgs, args[i])
			continue
		}
		switch {
		case args[i] == "--":
			optsDone = true
		case args[i] == "-e" && i+1 < len(args):
			scriptFragments = append(scriptFragments, args[i+1])
			i++
		default:
			runArgs = append(runArgs, args[i])
		}
	}
	return scriptFragments, runArgs
}

// TestOsascriptNotifyArgs_NoInjection is the regression guard: the fixed
// argv-based builder must pass title/body as opaque `run` arguments, never
// interpolated into any `-e` AppleScript source. This FAILS against the legacy
// interpolating implementation and PASSES after the fix.
func TestOsascriptNotifyArgs_NoInjection(t *testing.T) {
	const title = "Kandev"
	// Both attack shapes must be neutralized: the multi-line (statement-
	// separator) payload AND the single-line (expression-injection) payload.
	cases := map[string]string{
		"multi_line_statement":     breakoutPayload,
		"single_line_expression":   singleLineBreakoutPayload,
		"flag_like_do_shell_quote": `do shell script "touch /tmp/pwn"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			args := osascriptNotifyArgs(title, body)
			scriptFragments, runArgs := splitOsascriptArgs(args)

			// 1. The untrusted title/body must appear ONLY as trailing run
			//    arguments, byte-for-byte, never embedded in any source fragment.
			if len(runArgs) != 2 || runArgs[0] != title || runArgs[1] != body {
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

			// 2. The script fragments reference argv, not interpolated literals.
			joined := strings.Join(scriptFragments, "\n")
			if !strings.Contains(joined, "item 2 of argv") || !strings.Contains(joined, "item 1 of argv") {
				t.Fatalf("expected argv-referencing display notification, got: %q", joined)
			}
		})
	}
}

// TestOsascriptNotifyArgs_OptionTerminator guards the second injection vector:
// a title that looks like an osascript flag (e.g. "-e") must not be consumed as
// an option. The `--` terminator must precede the untrusted text so title/body
// always land in argv instead of becoming script fragments.
func TestOsascriptNotifyArgs_OptionTerminator(t *testing.T) {
	// A title of "-e" with a malicious body: without the terminator osascript
	// would treat "-e <body>" as another script fragment, reopening injection.
	title := "-e"
	body := `do shell script "touch /tmp/pwn"`
	args := osascriptNotifyArgs(title, body)

	// The builder must emit a `--` terminator, and it must come before the
	// untrusted title/body (i.e. every token after `--` is untrusted).
	term := -1
	for i, a := range args {
		if a == "--" {
			term = i
			break
		}
	}
	if term == -1 {
		t.Fatalf("missing `--` option terminator: %#v", args)
	}

	// After the terminator, the flag-like title and body must be confined to
	// argv, byte-for-byte, and never surface as script fragments.
	scriptFragments, runArgs := splitOsascriptArgs(args)
	if len(runArgs) != 2 || runArgs[0] != title || runArgs[1] != body {
		t.Fatalf("flag-like title/body not confined to argv: %#v", runArgs)
	}
	for _, frag := range scriptFragments {
		if strings.Contains(frag, "do shell script") {
			t.Fatalf("body leaked into AppleScript source as a fragment: %q", frag)
		}
	}
}

// TestEscapePowerShell_NeutralizesSubExpression is the regression guard for the
// PowerShell sub-expression injection fix. It asserts that escapePowerShell
// backtick-escapes `$` (killing `$(...)` sub-expressions and `$var` expansion),
// doubles bare backticks, and escapes `"` — so a hostile SoundFile path can no
// longer inject code into the playWindowsSound `-c` script.
func TestEscapePowerShell_NeutralizesSubExpression(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "sub-expression is defused",
			input: `a$(Remove-Item C:\x).wav`,
			want:  "a`$(Remove-Item C:\\x).wav",
		},
		{
			name:  "variable expansion is defused",
			input: `$env:TEMP\s.wav`,
			want:  "`$env:TEMP\\s.wav",
		},
		{
			name:  "backtick doubled before other escapes",
			input: "a`$(x).wav",
			want:  "a```$(x).wav",
		},
		{
			name:  "double quote escaped",
			input: `a".wav`,
			want:  "a`\".wav",
		},
		{
			name:  "plain path unchanged",
			input: `C:\Users\me\sound.wav`,
			want:  `C:\Users\me\sound.wav`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapePowerShell(tc.input); got != tc.want {
				t.Fatalf("escapePowerShell(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestPlayWindowsSoundScript_NoLiveSubExpression asserts the reconstructed
// `-c` script contains no live `$(` sub-expression once the path is escaped.
func TestPlayWindowsSoundScript_NoLiveSubExpression(t *testing.T) {
	payload := `C:\sounds\a$(Remove-Item -Recurse C:\important).wav`
	script := fmt.Sprintf(`(New-Object Media.SoundPlayer "%s").PlaySync()`, escapePowerShell(payload))
	// A live sub-expression would appear as `$(` with no escaping backtick in
	// front. After escaping, the only `$(` occurrence must be the escaped form.
	if strings.Contains(script, "$(") && !strings.Contains(script, "`$(") {
		t.Fatalf("script still contains a live sub-expression: %q", script)
	}
	if strings.Contains(strings.ReplaceAll(script, "`$(", ""), "$(") {
		t.Fatalf("script contains an unescaped sub-expression: %q", script)
	}
}
