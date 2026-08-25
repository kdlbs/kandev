---
status: active
system: executors
created: 2026-05-16
owners:
  - tbd
---
# SSH Executor Requirements

## Overview

Today the only ways to run an agent are: (1) locally on the user's own machine (`local_pc`, `local_docker`), or (2) in a Sprites-hosted ephemeral sandbox. There is no option for **"run on a Linux box I already own"** — a user's VPS, homelab server, beefy desktop, EC2/Hetzner instance, or shared team dev box.

## Requirements

### REQ-EXECUTORS-SSH-EXECUTOR-001: SSH Executor

**Intent:** Today the only ways to run an agent are: (1) locally on the user's own machine (`local_pc`, `local_docker`), or (2) in a Sprites-hosted ephemeral sandbox. There is no option for **"run on a Linux box I already own"** — a user's VPS, homelab server, beefy desktop, EC2/Hetzner instance, or shared team dev box.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-EXECUTOR-001.1:** Its **own agentctl process**, launched on a unique remote port chosen by binding to `:0` on the remote and reading back the chosen port.
- **AC-EXECUTORS-SSH-EXECUTOR-001.2:** Its **own SSH local port forward**: a fresh `127.0.0.1:0` listener that the kandev backend dials for HTTP + WebSocket streams to that session's agentctl. All forwards ride the single shared SSH connection for the host (see below).
- **AC-EXECUTORS-SSH-EXECUTOR-001.3:** A **session-scoped runtime dir** at `<workdir_root>/tasks/<task-dir-name>/.kandev/sessions/<session-id>/` for the agentctl PID file, port file, and log — kept under the task dir so cleanup follows the task, not orphaned across the filesystem.
- **AC-EXECUTORS-SSH-EXECUTOR-001.4:** **Auth: SSH key file + system ssh-agent.** Authentication via `golang.org/x/crypto/ssh` using either an explicit `IdentityFile` or the user's running `ssh-agent` (`$SSH_AUTH_SOCK`) — covers 1Password / Secretive / Yubikey / forwarded-agent users who don't keep raw keys on disk. **Passphrase-protected keys are not handled in kandev** — users must load them into `ssh-agent` themselves. Password and keyboard-interactive auth are not supported in v1.
- **AC-EXECUTORS-SSH-EXECUTOR-001.5:** **`~/.ssh/config` inheritance.** When a `host_alias` is configured, kandev parses `~/.ssh/config` to inherit `HostName`, `Port`, `User`, `IdentityFile`, `ProxyJump`, and `IdentitiesOnly` from the user's existing config. A user whose terminal already does `ssh prod` can paste `prod` into kandev and have it just work.
- **AC-EXECUTORS-SSH-EXECUTOR-001.6:** **Connectivity: direct, ProxyJump, mesh-VPN.** Direct TCP to `host:port` (default 22). `ProxyJump` (single bastion in v1; chained jumps deferred) implemented natively via the Go SSH client. Tailscale / WireGuard / corporate VPN: "just works" when the kandev backend process is on the same network namespace.
- **AC-EXECUTORS-SSH-EXECUTOR-001.7:** **Create flow: test-connection gate + explicit fingerprint trust.** The Sprites executor already gates creation behind a "Test Connection" step. The SSH executor adopts the same pattern and extends it with an explicit host-key trust step: 1. **User fills the form.** 2. **User clicks "Test connection".** Backend `POST /api/v1/ssh/test` dials with a permissive host-key callback that records (but does not pin) the observed fingerprint, then probes `uname -a`, arch, `git --version`, and the agentctl-cache-or-upload status. 3. **UI shows the result** with per-step badges and a prominent fingerprint block with a **"Trust this host"** checkbox that must be ticked before Save is enabled. 4. **On Save**, the trusted fingerprint is written to the executor's `Config` as `host_fingerprint`. On every subsequent connection (CreateInstance, RecoverInstances, status polling), a mismatch is a hard error — no silent re-pin.
- **AC-EXECUTORS-SSH-EXECUTOR-001.8:** One SSH connection per `(host, user, identity-source)`, opened lazily and reused across all the executor's sessions on that host. Each session adds:

## System design

The migrated technical source is split into [part 1](../system-design/ssh-executor.md).
