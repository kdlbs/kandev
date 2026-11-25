package lifecycle

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

// windowsCmdLineLimit is the longest command line cmd.exe accepts. Stock
// Win32 OpenSSH runs every exec request through cmd.exe /c.
const windowsCmdLineLimit = 8191

// decodeTestPowerShell returns the script that a fake Windows host would run
// for one exec request. It fails closed unless the command line is the fixed
// loader under the cmd.exe limit and stdin carries a valid envelope.
func decodeTestPowerShell(command, stdin string) (string, bool) {
	envelope, ok := decodeTestPowerShellEnvelope(command, stdin)
	return envelope.Script, ok
}

func decodeTestPowerShellEnvelope(command, stdin string) (powerShellEnvelope, bool) {
	if len(command) >= windowsCmdLineLimit {
		return powerShellEnvelope{}, false
	}
	const flag = "-EncodedCommand "
	idx := strings.Index(command, flag)
	if idx < 0 {
		return powerShellEnvelope{}, false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(command[idx+len(flag):]))
	if err != nil || len(raw)%2 != 0 {
		return powerShellEnvelope{}, false
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	if string(utf16.Decode(units)) != powerShellLoaderScript {
		return powerShellEnvelope{}, false
	}
	var envelope powerShellEnvelope
	if err := json.Unmarshal([]byte(stdin), &envelope); err != nil {
		return powerShellEnvelope{}, false
	}
	return envelope, true
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.5
func TestSSHWindowsCommandLineStaysBelowCmdLimit(t *testing.T) {
	r := &SSHExecutor{}
	req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
		"repository_clone_url": "https://github.com/example-org/example-repository.git",
		"repository_branch":    "main",
	}}
	prepare, err := r.resolveWindowsPrepareScript(req, `C:\Users\Kandev User\.kandev\tasks\task-1`)
	if err != nil {
		t.Fatal(err)
	}
	large := "# " + strings.Repeat("x", 64*1024) + "\nWrite-Output done"
	for _, shell := range []string{"powershell", "pwsh", `C:\Program Files\PowerShell\7\pwsh.exe`} {
		var lines []string
		for _, script := range []string{prepare, large, "Write-Output ok"} {
			for _, login := range []bool{true, false} {
				cmd, err := newPowerShellCommand(shell, powerShellEnvelope{Login: login, Script: script})
				if err != nil {
					t.Fatalf("newPowerShellCommand(%q): %v", shell, err)
				}
				if len(cmd.line) >= windowsCmdLineLimit {
					t.Fatalf("command line for shell %q has %d characters, limit %d", shell, len(cmd.line), windowsCmdLineLimit)
				}
				if got, ok := decodeTestPowerShell(cmd.line, cmd.stdin); !ok || got != script {
					t.Fatalf("envelope did not round-trip the script for shell %q", shell)
				}
				lines = append(lines, cmd.line)
			}
		}
		for _, line := range lines[1:] {
			if line != lines[0] {
				t.Fatalf("command line for shell %q depends on the script; want one fixed loader", shell)
			}
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.9
func TestSSHWindowsEnvelopeKeepsSecretsOffCommandLine(t *testing.T) {
	cmd, err := newPowerShellCommand("powershell", powerShellEnvelope{
		Env:    map[string]string{"GITHUB_TOKEN": "ghs_secret_value"},
		Script: "Write-Output $env:GITHUB_TOKEN.Length",
		Input:  "reconcile-body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cmd.line, "ghs_secret_value") || strings.Contains(cmd.line, "reconcile-body") {
		t.Fatal("environment or input value leaked into the command line")
	}
	envelope, ok := decodeTestPowerShellEnvelope(cmd.line, cmd.stdin)
	if !ok || envelope.Env["GITHUB_TOKEN"] != "ghs_secret_value" || envelope.Input != "reconcile-body" {
		t.Fatalf("envelope = %#v, ok=%v; want env and input on stdin", envelope, ok)
	}
	if _, err := newPowerShellCommand("powershell", powerShellEnvelope{Env: map[string]string{"BAD KEY": "x"}}); err == nil {
		t.Fatal("expected an invalid environment key to be rejected")
	}
}

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
