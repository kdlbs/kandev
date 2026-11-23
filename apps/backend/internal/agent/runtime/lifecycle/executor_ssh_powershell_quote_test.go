package lifecycle

import (
	"strings"
	"testing"
)

// PowerShell ends a single-quoted string at any of five quote characters, so
// each one must be doubled. A value typed with a typographic apostrophe must
// stay data instead of closing the string.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.7
func TestSSHWindowsPowerShellQuoteDoublesEveryQuoteCharacter(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"plain":            {in: `C:\Users\kandev`, want: `'C:\Users\kandev'`},
		"ascii apostrophe": {in: "O'Brien", want: "'O''Brien'"},
		"left quote":       {in: "a\u2018b", want: "'a\u2018\u2018b'"},
		"right quote":      {in: `C:\Users\O` + "\u2019" + `Brien`, want: `'C:\Users\O` + "\u2019\u2019" + `Brien'`},
		"low quote":        {in: "a\u201Ab", want: "'a\u201A\u201Ab'"},
		"reversed quote":   {in: "a\u201Bb", want: "'a\u201B\u201Bb'"},
		"injection":        {in: "x\u2019; calc; \u2019", want: "'x\u2019\u2019; calc; \u2019\u2019'"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := powerShellQuote(tc.in); got != tc.want {
				t.Fatalf("powerShellQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A user profile may enable strict mode, which makes reading an omitted
// envelope property a terminating error. The loader resets it after the
// profile so every script runs under the same rules.
func TestSSHWindowsLoaderResetsStrictModeAfterProfile(t *testing.T) {
	profile := strings.Index(powerShellLoaderScript, ". $PROFILE")
	reset := strings.Index(powerShellLoaderScript, "Set-StrictMode -Off")
	input := strings.Index(powerShellLoaderScript, "$KandevInput = [string]$__kEnvelope.input")
	if profile < 0 || reset < profile || input < reset {
		t.Fatalf("want the profile, then Set-StrictMode -Off, then envelope reads; got indexes %d, %d, %d", profile, reset, input)
	}
}
