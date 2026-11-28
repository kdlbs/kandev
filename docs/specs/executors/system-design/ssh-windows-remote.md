---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
  - REQ-EXECUTORS-SSH-WINDOWS-002
created: 2026-11-22
owners:
  - kandev
---

# SSH Executor Windows Remote Hosts

## Purpose and boundaries

This design extends the [SSH executor design](ssh-executor.md) to Windows hosts.
The SSH connection pool, host-key trust, port forwarding, control handshake,
launch receipts, and task-row ownership stay unchanged. The design replaces the
POSIX remote command surface with a PowerShell surface for Windows hosts.

The executor system owns platform detection, the remote command surface, remote
layout, and controller lifecycle. The release system owns helper publication
through the [compact runtime distribution design](../../release/system-design/compact-runtime-distribution.md).
This design changes that design's helper set from four helpers to five.

Remote Docker stays limited to Linux and macOS hosts. SSH passthrough rejects
Windows hosts.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-EXECUTORS-SSH-WINDOWS-001.1`, `.2` | Platform detection and persistence |
| `AC-EXECUTORS-SSH-WINDOWS-001.3`, `.4`, `.17` | Shell policy |
| `AC-EXECUTORS-SSH-WINDOWS-001.5`, `.9` | PowerShell command transport |
| `AC-EXECUTORS-SSH-WINDOWS-001.6`, `.7` | Helper upload and remote layout |
| `AC-EXECUTORS-SSH-WINDOWS-001.8`, `.10` | Preparation, cleanup, and credentials |
| `AC-EXECUTORS-SSH-WINDOWS-001.11`, `.12` | Controller launch, readiness, and resume |
| `AC-EXECUTORS-SSH-WINDOWS-001.13`, `.14`, `.15` | Stop, identity, orphan sweep, and reconciliation |
| `AC-EXECUTORS-SSH-WINDOWS-001.16` | Web copy |
| `AC-EXECUTORS-SSH-WINDOWS-002.1` through `.4` | Helper distribution |

## Platform detection and persistence

`detectRemoteInfo` in `executor_ssh_operations.go` keeps the `uname -m` and
`uname -s` probe. When either command fails, `detectWindowsRemoteInfo` runs a
fixed PowerShell probe through `powershell`, then `pwsh`. The probe prints one
`KANDEV_WIN_INFO` line with `Windows_NT`, the native processor architecture
(`PROCESSOR_ARCHITEW6432` before `PROCESSOR_ARCHITECTURE`), and the OS version.

`normalizeSSHRemoteOS` maps `windows`, `Windows_NT`, and the `MINGW*`, `MSYS*`,
and `CYGWIN*` uname prefixes to `windows`. Those prefixes appear when the host's
`DefaultShell` is a POSIX emulation layer. `normalizeSSHRemoteArch` also accepts
`arm64v8`.

Three gates check platforms separately:

- `requireRemoteHelperPlatform` in `remote_helper_manifest.go` accepts exactly
  the platforms in `remoteHelperPlatformAssets`. `AgentctlResolver` uses it.
  The resolver can serve `windows/amd64` because a helper ships for it.
- `requireSupportedSSHPlatform` is the SSH executor gate. It accepts the four
  Linux and macOS platforms plus `windows/amd64`. `windows/arm64` normalizes
  successfully but fails this check with the unsupported-platform error.
- `requireSupportedRemotePlatform` and its exported
  `SSHRequireSupportedRemotePlatform` keep the four Linux and macOS platforms.
  Remote Docker and plugin executor platform parsing keep calling them, so
  neither accepts Windows.

`reportedRemotePlatform` gives every gate the same raw uname detail for an
unmapped platform.

`CreateInstance` records the detected platform as `ssh_remote_platform`
(`MetadataKeySSHRemotePlatform`), and `persistentMetadataKeys` retains it.
Resume, status, liveness, recovery, and cleanup read the platform through
`sshPlatformFromMetadata`.

## Platform dispatch rule

A function that receives an `SSHRemotePlatform` dispatches on
`isWindowsRemotePlatform`. Callers that have a platform pass it explicitly.
Optional variadic platform parameters are not used.

A leaf helper that receives only remote paths dispatches on
`isWindowsRemotePath`. Kandev-created POSIX layouts are rooted at `/` and never
start with a drive letter, `/C:/`, or `\\server\share`, so path shape is
unambiguous for records that Kandev creates. `sshPlatformFromMetadata` is the
only place that infers a platform from paths. It does so only when the persisted
platform is missing.

## Shell policy

`SSHDefaultShellForPlatform` returns `powershell` for Windows.
`candidateShellsForPlatform` in `internal/ssh/handlers.go` probes `powershell`
and `pwsh` for Windows hosts. It probes through `Get-Command` under PowerShell.

`sshShellForRemote` keeps saved-shell precedence. Before preparation,
`CreateInstance` validates a Windows host's resolved shell with
`isPowerShellShell`. Any other shell fails with a typed error that names the
saved shell and `powershell`/`pwsh`. That check prevents a POSIX branch from
receiving Windows paths. The same pre-preparation check rejects
`ssh_passthrough` launches on Windows hosts.

## PowerShell command transport

Win32 OpenSSH passes each exec request to its `DefaultShell`. When that setting
is unset, `cmd.exe /c` receives the request, and `cmd.exe` accepts at most 8,191
command-line characters. `-EncodedCommand` encodes UTF-16LE as base64, so the
script grows by about 2.7 times. The fork transport encoded the full script and
produced a 13,171-character default prepare command. That command fails under
stock OpenSSH.

`WrapPowerShell` therefore encodes only a fixed loader:

```text
<exe> -NoLogo -NoProfile -NonInteractive -InputFormat Text -OutputFormat Text
      -ExecutionPolicy Bypass -EncodedCommand <base64(UTF-16LE loader)>
```

The loader sets UTF-8 console encodings, `$ErrorActionPreference = 'Stop'`, and
`$ProgressPreference = 'SilentlyContinue'`. Without that last setting,
PowerShell 5.1 writes `#< CLIXML` progress records to stderr when output is redirected.
It reads one JSON envelope from standard input:

```json
{"login": true, "env": {"KEY": "value"}, "script": "...", "input": "..."}
```

The loader sets `env` entries in process scope. For `login`, it merges the
Machine and User `Path` values and dot-sources `$PROFILE` with output and errors
suppressed. It exposes `input` as `$KandevInput` and runs `script` as a script
block. A terminating error exits with status 1. `exit <n>` and a failing
native command's `$LASTEXITCODE` pass through unchanged.

`buildPowerShellEnvelope` validates environment keys with
`posixSSHEnvIdentifier` and JSON-encodes the envelope. Environment values never
enter a command line. Every command that uses `WrapPowerShell` sends the
envelope through `runSSHCommandStdin` or `runSSHCommandStdinBounded`. The loader
command line has a fixed length, and a unit test bounds it below 8,191
characters.

`-ExecutionPolicy Bypass` applies only to that process. It permits the profile
dot-source under a restrictive policy and changes no machine or user policy.

## Helper upload and remote layout

`agentctlBinaryName` appends `.exe` for Windows. `AgentctlResolver.cachePath`
names the cached helper `agentctl.exe` for a Windows record. The helper path is
`~/.kandev/bin/agentctl.exe`, and its hash sidecar is
`~/.kandev/bin/agentctl.exe.sha256`.

`expandWindowsRemoteHome` resolves `~` from `$env:USERPROFILE`.
`cleanWindowsRemotePath` and `joinWindowsRemotePath` in
`executor_ssh_windows_paths.go` normalize paths without using the control
plane's `filepath`. They uppercase drive letters, use `\` separators, and
resolve `.` and `..` without crossing the volume or UNC share root.

OpenSSH `sftp-server` on Windows addresses `C:\x` as `/C:/x`. Every SFTP call
uses `toSFTPPath`. A Windows server can return `SSH_FX_OP_UNSUPPORTED` for
`chmod`, and `isSFTPOpUnsupported` ignores that error.

`ensureWindowsAgentctlOnHost` reuses an installed helper only when the SHA-256
of the installed `agentctl.exe`, computed on the host with `Get-FileHash`,
equals the local helper hash. The sidecar is written after the install, so an
interruption can leave it stale. A missing or mismatched helper triggers an
upload. A stale sidecar therefore cannot cause reuse of a different helper.
The upload then goes to a temporary name. Windows rename does not overwrite an
existing file, and Windows does not delete the image of a running process.
Windows does allow a running image to be renamed. The upload therefore renames
any existing `agentctl.exe` to `agentctl.exe.old-<random>`, renames the
temporary file into place, and then writes the sidecar. If the final rename
fails, it renames the previous helper back. After a successful replacement, it
deletes stale `agentctl.exe.old-*` files on a best-effort basis. It keeps any
file that is still in use for a later attempt. Running sessions keep their
original image.

`ensureWindowsRemoteTaskDir` and `ensureWindowsRemoteSessionDir` create the
layout from `AC-EXECUTORS-SSH-WINDOWS-001.7`.
`buildSSHCreateInstanceRequest` builds the durable journal path with
`joinRemotePath` and the platform, which keeps `\` separators on Windows.

## Preparation, cleanup, and credentials

`defaultWindowsSSHPrepareScript` in `executor_ssh_windows_scripts.go` is the
PowerShell counterpart of `defaultSSHPrepareScript`.
`isBuiltinPOSIXSSHPrepareScript` maps a stored built-in POSIX script to the
Windows default. `windowsBranchCheckoutPostlude` appends the managed branch
checkout. The Windows placeholder providers quote values with `powerShellQuote`.

`verifyWindowsPrimaryCheckout`, `buildSSHCleanupInvocation`, and
`sshWindowsRemoteContributionScript` provide the PowerShell paths for checkout
verification, cleanup, and contribution setup. All of them use the envelope
transport and `redactSSHScriptOutput`.

`gitCredentialHelperCommand` creates the managed helper value
`!'C:/Users/<user>/.kandev/bin/agentctl.exe' git-credential`. Git for Windows
runs helper values through its bundled `sh`, which accepts forward slashes.
Each apostrophe in the path is written as `'\''` inside the single-quoted
segment, so a user such as `O'Brien` yields
`!'C:/Users/O'\''Brien/.kandev/bin/agentctl.exe' git-credential`.
`runWindowsBrokerReachability` runs the managed broker preflight through the
envelope.

POSIX helper paths keep their existing format unless they contain whitespace or
quotes, which now require quoting.

## Controller launch, readiness, and resume

Win32 OpenSSH puts every process started from an exec channel in a job object,
and it ends that job when the channel closes. A `Start-Process` child therefore
dies with its launch channel. The real-host evidence below shows this.
`startRemoteWindowsAgentctlOnPort` uses the Windows Management Instrumentation
service instead. WMI creates processes outside the channel's job:

1. Create an empty `agentctl.log` in the session directory.
2. Build the child environment block from the launching PowerShell process
   environment, overlaid with the envelope `env` and `AGENTCTL_PORT`. When
   `Win32_ProcessStartup.EnvironmentVariables` is set, it replaces the child's
   whole environment. A partial block loses `PATH` and `USERPROFILE`.
3. Call `Invoke-CimMethod Win32_Process Create`. Pass the task directory as
   `CurrentDirectory`, a hidden window, that environment block, and this command
   line:
   `cmd.exe /d /c ""<agentctl.exe>" --workdir "<task dir>" > "<log>" 2>&1"`.
   `agentctl` has no log-file option, so the `cmd.exe` wrapper owns the
   redirection.
4. Poll `Win32_Process` with a bound of 10 seconds for the `agentctl.exe` child
   whose `ParentProcessId` is the wrapper PID. Write that child's PID to
   `agentctl.pid` and print it. The recorded PID must be the `agentctl.exe` PID,
   because launch-receipt validation compares it with the PID that `agentctl`
   reports. The wrapper exits when `agentctl` exits.

Environment values reach the child through WMI, never through a command line.
`retryRemoteAgentctlPort` and the existing handshake retry keep their POSIX
behavior.

`awaitRemoteAgentctlReadyForPlatform` reads the log with a shared-read file
stream. It checks liveness with `Get-Process`. The control handshake and launch
receipt validation are unchanged.

`ResumeRemoteInstance` resolves the helper path and liveness probe from the
persisted platform. Recovery compares `info.WorkspacePath` with
`sshRemotePathsEqual`, which is case-insensitive for Windows paths.
`readWindowsSSHRecoveryControlPortFromLog` reads the first and last 32 KiB of
the log to recover the control port.

## Stop, identity, orphan sweep, and reconciliation

`remoteWindowsAgentctlStopCommand` terminates the recorded PID with
`taskkill /T /F`, falls back to `Stop-Process -Force`, and polls for exit.
After the process exits, it removes the session directory with bounded retries.
Before a persisted-record stop, `verifyRemoteWindowsAgentctlIdentity` reads the
PID's `Win32_Process.CommandLine`. `remoteWindowsAgentctlCommandLineMatches`
then requires the recorded `--workdir` value under Win32 argument parsing and
case-insensitive path comparison. The session `agentctl.pid` must name the same
PID. When the command line does not match, the stop signals nothing.

`sshWindowsOrphanInventoryCommand` lists `agentctl.exe` processes with
`Get-CimInstance Win32_Process` and reads pidfiles below the tasks root.
`parseSSHWindowsOrphanInventory` produces the shared `sshOrphanInventory`, so
classification uses the POSIX rules. `stopSSHWindowsOrphanProcess` rechecks the
command line before forced tree termination.

The `agentctl ssh-reconcile` command on non-Linux builds returns
`ReasonUnsupportedIdentity`. `runRemoteWindowsSSHReconcile` invokes it through
the envelope with `input`. The executor therefore uses guarded stop and
confirmed absence for Windows controllers, as it does for macOS controllers.

## Web copy

The `descriptionSsh`, `sshConnectionDescription`, and `sshCreateDescription`
keys in `apps/web/src/locales/*/executors.json` name Linux, macOS, and Windows
hosts in every shipped locale. No layout or control changes.

## Helper distribution

`build-agentctl-windows-amd64` in `apps/backend/Makefile` cross-compiles with
CGO disabled, and `build-agentctl-remote` includes it. The release workflow
builds the helper twice, compares the builds, and verifies a PE32+ x86-64
image. The workflow then publishes `agentctl-windows-amd64.exe.gz` with a
`.sha256` sidecar. No `windows/arm64` target exists.

These closed helper tables add the one Windows entry:

- Go: `remoteHelperPlatformAssets` and `requiredAgentctlRemoteHelpers`
- JavaScript: `REMOTE_HELPERS` and `HELPER_ASSET_PATTERN`
- Rust: `REMOTE_AGENTCTL_HELPERS` and `REMOTE_HELPER_MANIFEST_RECORDS`
- Shell: the release helper arrays

`defaultReleaseBaseURL` remains `https://github.com/kdlbs/kandev/releases/download`
with no environment override.

The uncompressed helper measures about 63 MiB, and the compressed asset about
28 MiB. Complete bundles and the container image grow by about 63 MiB. Compact
Stable archives grow only by one manifest entry.

## Failure behavior

- `windows/arm64` and unknown Windows architectures fail with the
  unsupported-platform error.
- A missing `sftp` subsystem fails the upload with the SFTP error.
- A non-PowerShell shell fails with the shell-policy error.
- A loader parse failure exits with status 1 and returns stderr.
- A non-zero `Win32_Process.Create` return value fails the launch, and the
  error includes that return code. If no `agentctl.exe` child appears within the
  poll bound, the launch terminates the wrapper tree and reports the log tail.
- Preparation failures report redacted output through the existing progress
  callback.

## Verification strategy

Unit tests use the fake SSH server in `ssh_fake_server_test.go`. It decodes the
loader and envelope, then returns canned outputs. These tests prove the command
order, parameters, and parsing. They do not prove PowerShell semantics.

The Windows `native` suite in `backend-tests.yml` runs `//go:build windows`
tests. Those tests execute the generated scripts through the local
`powershell.exe` and, when present, `pwsh.exe`. They cover environment import,
exit-code propagation, default preparation against a local Git fixture, stop
with process-tree termination, identity matching, orphan inventory, and log
reads. Real-host evidence for `AC-EXECUTORS-SSH-WINDOWS-001.5` and `.11` comes
from a stock Windows OpenSSH Server.

## Real-host evidence

These probes ran on 2026-11-22 against a Windows 11 x64 host. The host ran the
built-in OpenSSH Server with `DefaultShell` unset, Windows PowerShell 5.1, and
PowerShell 7.

| Probe | Result |
| --- | --- |
| `echo %OS%` over exec | Expanded by `cmd.exe` |
| 6,822-character encoded command | Ran |
| 12,954-character encoded command | `cmd.exe` rejected it as too long |
| `Start-Process` child, checked inside its channel | Alive |
| Same child, 15 seconds after its channel closed | Terminated |
| `Win32_Process.Create` child, 15 seconds after its channel closed | Alive |
| `Win32_ProcessStartup` with only one variable | Child lost `PATH`; `cmd.exe` could not find `powershell.exe` |
| Same, with the merged environment block | Child had the variable, `USERPROFILE`, and Git on `PATH`. Its stdout and stderr reached a log in a path with spaces |
| `taskkill /T /F` on the wrapper | Wrapper and child ended |

Messages from `cmd.exe` itself, such as the length error, use the host's OEM
code page. Kandev reports them as received.
