---
status: draft
system: executors
created: 2026-11-22
owners:
  - kandev
---

# SSH Executor Windows Remote Hosts

## Overview

The [SSH executor](ssh-executor.md) runs agents on Linux and macOS hosts that a
user already owns. Many users also own Windows workstations and servers with the
built-in OpenSSH Server. They can reach those hosts over SSH, but Kandev rejects
them as an unsupported platform.

This capability extends the same SSH executor outcome to Windows hosts. The
user keeps the same profile, connection test, trust, launch, resume, stop, and
cleanup flow. Remote commands use PowerShell instead of a POSIX login shell.

## Terms

- **Windows host:** An SSH target whose operating system is Windows on x64
  and whose OpenSSH Server provides the `sftp` subsystem.
- **PowerShell shell:** Windows PowerShell (`powershell`) or PowerShell 7
  (`pwsh`), used as the remote command shell for a Windows host.
- **Stock OpenSSH configuration:** A Windows OpenSSH Server whose
  `DefaultShell` setting is unset, so `cmd.exe` receives each exec request.

## Requirements

### REQ-EXECUTORS-SSH-WINDOWS-001: Windows remote host support

**Intent:** As a user with a Windows machine that I can reach over SSH, I want
the SSH executor to run agents there, so that I can use that machine without
installing Linux or WSL.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-WINDOWS-001.1:** When a connection test or launch targets
  an x64 Windows host, the system shall identify the platform as
  `windows/amd64`, including when the host provides no `uname` command. The
  connection test shall report that platform. An ARM64 Windows host shall be
  identified as `windows/arm64` and rejected as unsupported.
- **AC-EXECUTORS-SSH-WINDOWS-001.2:** When a host reports an unsupported
  platform, the SSH executor error shall list `linux/{amd64,arm64}`,
  `darwin/{amd64,arm64}`, and `windows/amd64` as supported. Remote
  Docker shall continue to reject Windows hosts with an error that lists only
  its own supported platforms.
- **AC-EXECUTORS-SSH-WINDOWS-001.3:** When a Windows host profile has no saved
  shell, the system shall use `powershell`. The shell probe for a Windows host
  shall offer only `powershell` and `pwsh` when they resolve on that host.
- **AC-EXECUTORS-SSH-WINDOWS-001.4:** When a Windows host profile has a saved
  shell other than `powershell` or `pwsh`, launch shall fail before task
  preparation. The error shall name the saved shell and the supported shells.
- **AC-EXECUTORS-SSH-WINDOWS-001.5:** When the host has a stock OpenSSH
  configuration, every Kandev-generated remote command shall run. This includes
  the default prepare script and a custom prepare or cleanup script of at least
  64 KiB.
- **AC-EXECUTORS-SSH-WINDOWS-001.6:** When a Windows launch needs `agentctl`,
  the system shall upload the helper for the host architecture to
  `%USERPROFILE%\.kandev\bin\agentctl.exe`. It shall reuse a helper whose
  recorded content hash matches and replace one whose hash differs. Replacement
  shall succeed while another session on that host still runs the previous
  helper, and that session shall keep running.
- **AC-EXECUTORS-SSH-WINDOWS-001.7:** On a Windows host, a leading `~` in the
  workdir root shall resolve to `%USERPROFILE%`. The task directory shall be
  `<workdir root>\tasks\<task-dir-name>`. The session runtime directory shall be
  `<task directory>\.kandev\sessions\<session-id>`. Paths that contain spaces,
  apostrophes, or non-ASCII characters shall work.
- **AC-EXECUTORS-SSH-WINDOWS-001.8:** When the profile prepare script is empty,
  a Windows launch shall produce the observable results of the SSH default
  preparation: clone or reuse a matching primary checkout at the task root,
  abort on a conflicting origin, run repository setup, and select the Kandev
  branch. A non-empty profile prepare or cleanup script shall run as PowerShell.
- **AC-EXECUTORS-SSH-WINDOWS-001.9:** A Windows remote agent shall receive the
  same managed credential allowlist and approved repository keys as a POSIX
  remote agent. Environment values shall not appear in a remote process command
  line. Prepare output shall apply the same secret redaction as POSIX hosts.
- **AC-EXECUTORS-SSH-WINDOWS-001.10:** When Kandev manages GitHub credentials,
  Git on a Windows host shall obtain them through the uploaded helper. The
  managed broker reachability preflight shall run before preparation, as it does
  on POSIX hosts.
- **AC-EXECUTORS-SSH-WINDOWS-001.11:** After a Windows `agentctl` reports
  readiness, it shall keep running after the SSH channel that launched it
  closes. It shall also keep running when the Kandev backend stops.
- **AC-EXECUTORS-SSH-WINDOWS-001.12:** When the backend restarts while a
  Windows `agentctl` is alive, resume shall reattach to that controller without
  a replacement launch. When the controller is confirmed absent, resume shall
  follow the [SSH executor](ssh-executor.md) replacement behavior.
- **AC-EXECUTORS-SSH-WINDOWS-001.13:** When a Windows session stops, the system
  shall terminate the recorded `agentctl` process and its descendants, then
  remove the session runtime directory. A process that reuses the recorded PID
  without the recorded task directory in its command line shall survive.
- **AC-EXECUTORS-SSH-WINDOWS-001.14:** The orphan sweep shall apply the same
  ownership and preservation rules to Windows hosts as SSH executor criteria
  `AC-EXECUTORS-SSH-EXECUTOR-001.13` through `.15`. It shall stop a proven
  orphan with forced process-tree termination and report the same counts.
- **AC-EXECUTORS-SSH-WINDOWS-001.15:** Directory-loss reconciliation shall treat
  a Windows controller as one without supported launch proof. It shall retain
  the guarded-stop and confirmed-absence behavior that macOS controllers use.
- **AC-EXECUTORS-SSH-WINDOWS-001.16:** SSH executor descriptions in the web UI
  shall name Linux, macOS, and Windows hosts in every shipped locale.
- **AC-EXECUTORS-SSH-WINDOWS-001.17:** When an SSH passthrough launch targets a
  Windows host, launch shall fail before task preparation. The error shall state
  that passthrough mode is unavailable on Windows hosts.

### REQ-EXECUTORS-SSH-WINDOWS-002: Windows helper distribution

**Intent:** Windows hosts need an `agentctl.exe` that matches their
architecture. Each release must provide that helper through the same channels
as the Linux and macOS helpers.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-WINDOWS-002.1:** Each Stable release shall publish
  a compressed `windows/amd64` helper asset with a checksum sidecar. The
  remote helper manifest shall list that asset and no `windows/arm64` asset.
- **AC-EXECUTORS-SSH-WINDOWS-002.2:** When a compact runtime first needs a
  Windows helper, it shall download the asset for the running release. It shall
  verify that asset's manifest checksum before use and cache it.
- **AC-EXECUTORS-SSH-WINDOWS-002.3:** Complete offline bundles, Desktop runtime
  resources, and the container image shall contain the `windows/amd64` helper.
  Bundle validation shall reject a complete bundle that lacks it.
- **AC-EXECUTORS-SSH-WINDOWS-002.4:** Each release shall download Windows
  helpers only from the `kdlbs/kandev` release for the running version. No
  environment variable or profile setting shall redirect that download.

## Exclusions

- Remote Docker on Windows hosts.
- Windows on ARM64 hosts, including through x64 emulation.
- `cmd.exe`, Git Bash, MSYS2, Cygwin, and other POSIX shells as the remote
  command shell for a Windows host.
- WSL distributions as Windows hosts. Connect to a WSL `sshd` as a Linux host.
- Windows hosts without the OpenSSH `sftp` subsystem.
- Launch-proof reconciliation for Windows controllers.
- Graceful termination signals for Windows controllers. Stop uses forced
  process-tree termination.
- Automatic installation of PowerShell 7, Git, or agent CLIs on the host.
- Authenticode signing of the Windows remote helpers. The helper build
  remains reproducible and unsigned, unlike the signed native Windows runtime.
- SSH passthrough mode on Windows hosts until a separate requirement specifies
  remote Windows terminal behavior.

## System design

The [Windows remote host design](../system-design/ssh-windows-remote.md)
defines detection, command transport, layout, lifecycle, and distribution.
