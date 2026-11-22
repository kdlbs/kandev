---
status: current
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
| `AC-EXECUTORS-SSH-WINDOWS-001.8`, `.10`, `.19` | Preparation, cleanup, and credentials |
| `AC-EXECUTORS-SSH-WINDOWS-001.11`, `.12` | Controller launch, readiness, and resume |
| `AC-EXECUTORS-SSH-WINDOWS-001.13`, `.14`, `.15` | Stop, identity, orphan sweep, and reconciliation |
| `AC-EXECUTORS-SSH-WINDOWS-001.16` | Web copy |
| `AC-EXECUTORS-SSH-WINDOWS-001.18` | Task-directory reclamation |
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

`executor_ssh_powershell.go` therefore places only a fixed loader
(`powerShellLoaderScript`) on the command line:

```text
<exe> -NoLogo -NoProfile -NonInteractive -InputFormat Text -OutputFormat Text
      -ExecutionPolicy Bypass -EncodedCommand <base64(UTF-16LE loader)>
```

The loader sets UTF-8 console encodings, `$ErrorActionPreference = 'Stop'`, and
`$ProgressPreference = 'SilentlyContinue'`. It reads one JSON envelope
(`powerShellEnvelope`) from standard input:

```json
{"login": true, "env": {"KEY": "value"}, "script": "...", "input": "..."}
```

The loader sets the `env` entries in process scope. When `login` is set, it
merges the Machine and User `Path` values and dot-sources `$PROFILE`, with output
and errors suppressed. It exposes `input` as `$KandevInput` and dot-sources
`script` as a script block. Exit codes match those of a script passed directly
to `-Command`:

- `exit <n>` exits with `n`.
- A terminating error exits with 1. The loader catches it and writes its message
  as plain text, because PowerShell 5.1 otherwise serializes redirected errors as
  `#< CLIXML`.
- When the last statement fails, the loader exits with that statement's native
  `$LASTEXITCODE`, or 1. A dot-sourced block hides that status, so the loader
  appends `$__kLastOk = $?` to the script text.

The loader defines `Invoke-KandevNative { ... }`, which runs one native command
under a local `$ErrorActionPreference = 'Continue'`. Under `Stop`, Windows
PowerShell 5.1 turns each line of a native command's redirected stderr into a
terminating error, even when the command succeeds. Git writes probe failures and
fetch progress to stderr, so the default preparation failed on its first probe
in a fresh workspace. PowerShell 7 does not have this behavior. Every Windows
script line that redirects native stderr (`*>$null`, `2>$null`, or `2>&1`) runs
inside `Invoke-KandevNative`. `TestSSHWindowsScriptsWrapRedirectedNativeCommands`
scans the script sources and enforces this rule on every platform.

`newPowerShellCommand` validates environment keys with `posixSSHEnvIdentifier`
and JSON-encodes the envelope. `runPowerShell`, `runPowerShellEnvelope`, and
`runPowerShellEnvelopeBounded` send the envelope through `runSSHCommandStdin` or
`runSSHCommandStdinBounded`. Environment values never enter a command line. The
loader command line has a fixed length of about 4,200 characters, and
`TestSSHWindowsCommandLineStaysBelowCmdLimit` bounds it below 8,191. The fake
SSH server's decoder rejects any Windows command line that is not the loader, so
every Windows unit test enforces the same bound.

`WrapLoginShell` stays POSIX-only. Windows builders return PowerShell scripts,
not command strings: `remoteWindowsAgentctlStopScript`,
`removeWindowsRemoteDirScript`, and `sshWindowsOrphanInventoryScript`. Callers
that can reach either host family dispatch explicitly, as
`stopRemoteAgentctl` and `removeRemoteDir` do.

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

`ensureWindowsAgentctlOnHost` compares the sidecar hash with the local helper
hash, then uploads to a temporary name. Windows rename does not overwrite an
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
`runWindowsBrokerReachability` runs the managed broker preflight through the
envelope.

POSIX helper paths keep their existing format unless they contain whitespace or
quotes, which now require quoting.

Remote-authentication setup scripts in the agent catalog are POSIX shell
scripts. `runOneAuthSetupScript` skips them on Windows hosts and logs a warning
(`AC-EXECUTORS-SSH-WINDOWS-001.19`). It never runs them under PowerShell.

## Controller launch, readiness, and resume

Win32 OpenSSH ends every process of an exec channel when the channel closes, so
a `Start-Process` child dies with its launch channel. The real-host evidence
below shows this. `windowsAgentctlLaunchScript` in
`executor_ssh_windows_launch.go` uses the Windows Management Instrumentation
service instead, which creates processes outside the channel:

1. Create an empty `agentctl.log` in the session directory.
2. Build the child environment block from the launching PowerShell process
   environment, overlaid with the envelope `env` and `AGENTCTL_PORT`. When
   `Win32_ProcessStartup.EnvironmentVariables` is set, it replaces the child's
   whole environment. A partial block loses `PATH` and `USERPROFILE`.
3. Call `Invoke-CimMethod Win32_Process Create`. Pass the task directory as
   `CurrentDirectory`, a hidden window, that environment block, and this command
   line:
   `cmd.exe /d /s /c ""<agentctl.exe>" --workdir "<task dir>" > "<log>" 2>&1"`.
   `agentctl` has no log-file option, so the `cmd.exe` wrapper owns the
   redirection. `quoteWindowsArgAlways` quotes every argument, which keeps
   `cmd.exe` metacharacters such as `&` literal. `cmd.exe` still expands
   `%NAME%` inside quotes, so a path containing such a sequence is unsupported.
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

## Task-directory reclamation

`SSHTaskDirReclaimer.Reclaim` checks for a Windows path before any remote
command runs. When the task directory or the workdir root is a Windows path, it
returns a skip with `SSHReclaimSkipUnsupportedPlatform`
(`unsupported_platform`). The cleanup job treats every skip as success, so it
does not retry. The POSIX reclamation probes in the
[reclamation design](remote-task-directory-reclamation.md) stay unchanged.

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

The Windows `native` suite in `backend-tests.yml` runs `^TestSSHWindows`, and
`make test-windows` runs the same filter. The `TestSSHWindowsExec*` tests in
`executor_ssh_windows_exec_test.go` skip on other platforms. On Windows, a fake
SSH server runs each exec request through `cmd.exe /c` with stdin, as stock
OpenSSH does, so the production Go code drives real `powershell.exe` and, when
installed, `pwsh.exe`.

A copy of the test binary serves as a stand-in `agentctl.exe`. An `init` hook
switches it to controller behavior when its marker variable is set. Only the
raw command-line setter carries a Windows build tag. The tests cover:

- environment and input import
- exit codes
- default preparation against a local Git fixture
- a 64 KiB custom script
- a WMI launch that outlives its request
- identity, orphan inventory, and log reads
- tree-terminating stop
- helper replacement against a running image on NTFS

Real-host evidence for `AC-EXECUTORS-SSH-WINDOWS-001.5`, `.8`, and `.11` comes
from a stock Windows OpenSSH Server.

## Known limitations

These limitations were observed on the validation host. Each needs separate
work:

- The first turn of a new Windows session can miss its turn-change start
  checkpoint. On a fresh clone, agentctl's Git checkpoint took longer than the
  backend's request deadline. The backend logs a warning, and later turns
  capture normally.
- Stop forcibly ends `agentctl.exe`, so agentctl cannot remove its
  `%TEMP%\kandev-github-cli-*` shim directory. Each session leaves one directory
  of links, or of copies when the volume refuses links.

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
| Generated launch script with the real `agentctl.exe`, 15 seconds after its channel closed | `agentctl.exe` alive, with `cmd.exe` as parent. `agentctl.pid` names the `agentctl.exe` PID. `/health` on the `AGENTCTL_PORT` port returns 200, and the log shows `HTTP server bound successfully` |
| `remoteWindowsAgentctlStopScript` for that PID | Exit 0. No `agentctl.exe` or wrapper process remains, and the session directory is removed |
| Launch script with a missing `agentctl.exe` | Exit 1. The error and the log tail report that `cmd.exe` does not recognize the path |
| Production loader with a 64 KiB script, environment values, and input | Ran through `cmd.exe`. Values round-tripped, and exit codes matched `-Command` |
| `& git ... *>$null` under `Stop` | Windows PowerShell 5.1 threw a terminating error; PowerShell 7 did not |
| Fork default prepare with `Invoke-KandevNative` removed, fresh workspace, PowerShell 5.1 | Failed with `fatal: not a git repository` |
| Production executor functions over real SSH: default prepare (fresh, reuse, conflicting origin) under `powershell` and `pwsh`, checkout verification, helper upload, WMI launch, running-helper replacement, identity, orphan inventory, log read, and stop | All behaved as specified. Replacement kept the running image as `agentctl.exe.old-*` |

Messages from `cmd.exe` itself, such as the length error, use the host's OEM
code page. Kandev reports them as received.
