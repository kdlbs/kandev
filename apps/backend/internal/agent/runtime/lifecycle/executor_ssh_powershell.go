package lifecycle

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"

	"golang.org/x/crypto/ssh"
)

// powerShellLoaderScript is the only script placed on a Windows command line.
// Stock Win32 OpenSSH passes exec requests through cmd.exe, which accepts at
// most 8,191 characters, so every script body, environment value, and input
// travels on stdin in one JSON envelope. The loader preserves -Command exit
// semantics: `exit n` returns n, a terminating error returns 1, and a failing
// last statement returns its native exit code or 1.
const powerShellLoaderScript = `$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$__kEnvelope = [Console]::In.ReadToEnd() | ConvertFrom-Json
if ($null -ne $__kEnvelope.env) {
  foreach ($__kProp in $__kEnvelope.env.PSObject.Properties) {
    [Environment]::SetEnvironmentVariable($__kProp.Name, [string]$__kProp.Value, 'Process')
  }
}
if ($__kEnvelope.login) {
  foreach ($__kScope in @('Machine', 'User')) {
    $__kPath = [Environment]::GetEnvironmentVariable('Path', $__kScope)
    if ($__kPath) {
      foreach ($__kEntry in ($__kPath -split ';')) {
        if ($__kEntry -and (($env:PATH -split ';') -notcontains $__kEntry)) {
          $env:PATH = if ($env:PATH) { "$env:PATH;$__kEntry" } else { $__kEntry }
        }
      }
    }
  }
  if ($PROFILE -and (Test-Path -LiteralPath $PROFILE)) {
    try { . $PROFILE *>$null } catch {}
  }
  $ErrorActionPreference = 'Stop'
}
$KandevInput = [string]$__kEnvelope.input
$__kScript = [ScriptBlock]::Create([string]$__kEnvelope.script + "` + "`" + `n` + "`" + `$__kLastOk = ` + "`" + `$?")
Remove-Variable -Name __kEnvelope, __kProp, __kScope, __kPath, __kEntry -ErrorAction SilentlyContinue
$__kLastOk = $true
try {
  . $__kScript
} catch {
  [Console]::Error.WriteLine($_.Exception.Message)
  exit 1
}
if (-not $__kLastOk) {
  if ($LASTEXITCODE -is [int] -and $LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  exit 1
}
`

// powerShellLoaderArgument is the encoded loader passed to -EncodedCommand.
var powerShellLoaderArgument = encodePowerShellCommand(powerShellLoaderScript)

// powerShellEnvelope is the stdin payload the loader reads. Login merges the
// Machine and User Path values and loads $PROFILE before Script runs. Input is
// exposed to the script as $KandevInput.
type powerShellEnvelope struct {
	Login  bool              `json:"login,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	Script string            `json:"script"`
	Input  string            `json:"input,omitempty"`
}

// powerShellCommand is one Windows exec request: the fixed loader command line
// and the envelope it reads from stdin.
type powerShellCommand struct {
	line  string
	stdin string
}

func newPowerShellCommand(shell string, envelope powerShellEnvelope) (powerShellCommand, error) {
	for key := range envelope.Env {
		if !posixSSHEnvIdentifier.MatchString(key) {
			return powerShellCommand{}, fmt.Errorf("invalid SSH environment variable key %q", key)
		}
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return powerShellCommand{}, fmt.Errorf("encode PowerShell envelope: %w", err)
	}
	line := powerShellExecutable(shell) +
		" -NoLogo -NoProfile -NonInteractive -InputFormat Text -OutputFormat Text -ExecutionPolicy Bypass -EncodedCommand " +
		powerShellLoaderArgument
	return powerShellCommand{line: line, stdin: string(payload)}, nil
}

// runPowerShell runs script on a Windows host without extra environment.
func runPowerShell(ctx context.Context, client *ssh.Client, shell, script string, login bool) (string, string, error) {
	return runPowerShellEnvelope(ctx, client, shell, powerShellEnvelope{Login: login, Script: script})
}

func runPowerShellEnvelope(ctx context.Context, client *ssh.Client, shell string, envelope powerShellEnvelope) (string, string, error) {
	cmd, err := newPowerShellCommand(shell, envelope)
	if err != nil {
		return "", "", err
	}
	return runSSHCommandStdin(ctx, client, cmd.line, strings.NewReader(cmd.stdin))
}

func runPowerShellEnvelopeBounded(
	ctx context.Context,
	client *ssh.Client,
	shell string,
	envelope powerShellEnvelope,
	maxStdout int,
) (string, string, bool, error) {
	cmd, err := newPowerShellCommand(shell, envelope)
	if err != nil {
		return "", "", false, err
	}
	return runSSHCommandStdinBounded(ctx, client, cmd.line, strings.NewReader(cmd.stdin), maxStdout)
}

func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func isPowerShellShell(shell string) bool {
	cleaned := strings.Trim(strings.TrimSpace(shell), `"'`)
	if cleaned == "" {
		return false
	}
	normalized := strings.ReplaceAll(cleaned, `\`, "/")
	if idx := strings.LastIndexByte(normalized, '/'); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	normalized = strings.TrimSuffix(strings.ToLower(normalized), ".exe")
	return normalized == defaultWindowsLoginShell || normalized == windowsPowerShellCoreBinary
}

func powerShellExecutable(shell string) string {
	cleaned := strings.Trim(strings.TrimSpace(shell), `"'`)
	if cleaned == "" || !isPowerShellShell(cleaned) {
		return defaultWindowsLoginShell
	}
	if strings.ContainsAny(cleaned, " \t") {
		return `"` + cleaned + `"`
	}
	return cleaned
}

func encodePowerShellCommand(script string) string {
	utf16Units := utf16.Encode([]rune(script))
	raw := make([]byte, len(utf16Units)*2)
	for i, unit := range utf16Units {
		binary.LittleEndian.PutUint16(raw[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
