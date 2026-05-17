---
status: draft
created: 2026-05-16
owner: tbd
---

# SSH Executor

## Why

Today the only ways to run an agent are: (1) locally on the user's own machine (`local_pc`, `local_docker`), or (2) in a Sprites-hosted ephemeral sandbox. There is no option for **"run on a Linux box I already own"** — a user's VPS, homelab server, beefy desktop, EC2/Hetzner instance, or shared team dev box.

That gap matters because:

- **Cost control.** Sprites are convenient but metered; a $5/mo VPS or an already-paid-for desktop is effectively free capacity for long-running tasks.
- **Hardware control.** Users who need a specific GPU, a lot of RAM, a particular CPU arch, or pre-installed proprietary tools (databases, CAD, internal services) cannot get there with Sprites.
- **Privacy / compliance.** Some users cannot send source code through a third-party cloud sandbox but are fine running an agent on a server they fully control.
- **"Move my Claude Code to the cloud" use case.** Running on a remote host means closing the laptop lid doesn't kill the agent, and the same task is reachable from a phone, tablet, or any browser.

SSH is the lowest-common-denominator transport for reaching a remote Linux box. If we can put an agent on anything reachable over SSH — VPS, homelab, EC2, raspberry pi, lab machine, jump-host-protected private box — we unlock all of the above in one feature without inventing a kandev-specific cloud.

## What (v1)

A new executor type `ssh` joins `local_pc` / `local_docker` / `sprites` in the existing executor framework. From the user's perspective: configure an SSH target once in settings, then pick it as the executor for any task; tasks run on the remote box exactly as they would on a Sprite.

### Remote layout: mirrors local `~/.kandev/tasks/`

Kandev's local convention is one directory per task containing one or more repo worktrees:

```
~/.kandev/tasks/
├── <task-dir-name-A>/
│   ├── <repo-1>/           # git worktree on the task's feature branch
│   └── <repo-2>/           # second repo worktree, same task
└── <task-dir-name-B>/
    └── <repo-1>/
```

The SSH executor mirrors this exact layout on the remote host. Workspace root defaults to `~/.kandev/` on the remote and is **configurable per profile** (so one host can serve `~/.kandev-team-a/tasks/` and `~/.kandev-team-b/tasks/` for different profiles on the same SSH user). Each task gets `<workdir_root>/tasks/<task-dir-name>/<repo-name>/` cloned + worktree'd just like the local case. Concurrent sessions on the same task share this directory; concurrent tasks on the same host get different `<task-dir-name>` siblings.

### Per-session, not per-task: agentctl process, remote port, local forward

Execution is keyed per session (matching local + Sprites). Each session on an SSH host gets:

- Its **own agentctl process**, launched on a unique remote port chosen by binding to `:0` on the remote and reading back the chosen port.
- Its **own SSH local port forward**: a fresh `127.0.0.1:0` listener that the kandev backend dials for HTTP + WebSocket streams to that session's agentctl. All forwards ride the single shared SSH connection for the host (see below).
- A **session-scoped runtime dir** at `<workdir_root>/tasks/<task-dir-name>/.kandev/sessions/<session-id>/` for the agentctl PID file, port file, and log — kept under the task dir so cleanup follows the task, not orphaned across the filesystem.

Multiple sessions in the *same* task share the same worktree on disk (same files, same branch state); they have independent agentctl processes and independent UI streams. Multiple sessions across *different* tasks on the same host have independent task dirs as well.

### Auth, connectivity, and host-key trust

- **Auth: SSH key file + system ssh-agent.** Authentication via `golang.org/x/crypto/ssh` using either an explicit `IdentityFile` or the user's running `ssh-agent` (`$SSH_AUTH_SOCK`) — covers 1Password / Secretive / Yubikey / forwarded-agent users who don't keep raw keys on disk. **Passphrase-protected keys are not handled in kandev** — users must load them into `ssh-agent` themselves. Password and keyboard-interactive auth are not supported in v1.

- **`~/.ssh/config` inheritance.** When a `host_alias` is configured, kandev parses `~/.ssh/config` to inherit `HostName`, `Port`, `User`, `IdentityFile`, `ProxyJump`, and `IdentitiesOnly` from the user's existing config. A user whose terminal already does `ssh prod` can paste `prod` into kandev and have it just work.

- **Connectivity: direct, ProxyJump, mesh-VPN.** Direct TCP to `host:port` (default 22). `ProxyJump` (single bastion in v1; chained jumps deferred) implemented natively via the Go SSH client. Tailscale / WireGuard / corporate VPN: "just works" when the kandev backend process is on the same network namespace.

- **Create flow: test-connection gate + explicit fingerprint trust.** The Sprites executor already gates creation behind a "Test Connection" step. The SSH executor adopts the same pattern and extends it with an explicit host-key trust step:
  1. **User fills the form.**
  2. **User clicks "Test connection".** Backend `POST /api/v1/ssh/test` dials with a permissive host-key callback that records (but does not pin) the observed fingerprint, then probes `uname -a`, arch, `git --version`, and the agentctl-cache-or-upload status.
  3. **UI shows the result** with per-step badges and a prominent fingerprint block with a **"Trust this host"** checkbox that must be ticked before Save is enabled.
  4. **On Save**, the trusted fingerprint is written to the executor's `Config` as `host_fingerprint`. On every subsequent connection (CreateInstance, RecoverInstances, status polling), a mismatch is a hard error — no silent re-pin.

### One SSH connection per host, fanned out to per-session forwards

- One SSH connection per `(host, user, identity-source)`, opened lazily and reused across all the executor's sessions on that host. Each session adds:
  - A local `net.Listen("tcp", "127.0.0.1:0")` to grab a free local port.
  - A goroutine that, for each accepted local connection, opens an SSH channel of type `direct-tcpip` to `127.0.0.1:<remoteAgentctlPort>` and bidirectionally copies bytes.
  - A teardown path that closes the local listener and any in-flight channels on `StopInstance`.
- SFTP transfers (agentctl upload, credential push) ride the same SSH connection.
- Connection-level keepalives (~30s) so a dropped connection is noticed quickly and triggers reconnect + re-forwarding of all live sessions' ports.
- **Failure model:** if the shared connection drops mid-session, all sessions on that host go to a "reconnecting" state simultaneously; one reconnect brings them all back.

### Workspace lifecycle on the remote

- On `CreateInstance` for a task that has no existing remote task dir: SSH `mkdir -p <workdir_root>/tasks/<task-dir-name>`, then the prepare-script + scriptengine pipeline (`git clone`, worktree, checkout) runs the same way it does for Sprites.
- On subsequent sessions for the same task on the same host: detect the existing task dir, skip clone, attach a new agentctl to it.
- On `StopInstance` of the *last* session in a task: leave the task dir intact (so a later resume keeps history); only remove it on explicit task-level cleanup (out of scope for v1 — see below).

### agentctl binary upload with content-hash cache (amd64 only in v1)

- On `CreateInstance`, resolve the local `agentctl-linux-amd64` binary via the existing `AgentctlResolver`.
- Detect remote architecture via `uname -m` over SSH. **If the remote is not `x86_64`, fail with a clear error**: "SSH executor v1 supports linux/amd64 only; this host reports `<arch>`". Matches the current Docker executor's reality — the Makefile only produces `agentctl-linux-amd64`.
- Compute SHA256 locally; check `~/.kandev/bin/agentctl.sha256` on the remote via `sha256sum`. Upload only if missing or mismatched.
- Upload via SFTP to `~/.kandev/bin/agentctl` (chmod 755), then write the sha256 sidecar.
- Binary is shared across all tasks and sessions on the host.
- **arm64 is a separate follow-up** that needs a new `build-agentctl-linux-arm64` Makefile target and an arch-aware resolver — benefits Docker too.

### Credential push reuses the existing `FileUploader` seam

- New `sshFileUploader` implements `FileUploader` (write file at path with mode) via SFTP.
- The existing `executor_sprites_credentials.go` selection + upload logic for `remote_credentials` / `remote_auth_secrets` is the reference; SSH v1 wires the uploader and writes credentials to the SSH user's home dir.
- GitHub CLI token: same special case as Sprites (run `gh auth token` locally, inject as env var).

### Recovery after backend restart

- Persist in `ExecutorRunning.Metadata` (allow-listed in `persistentMetadataKeys`): `ssh_host`, `ssh_port`, `ssh_user`, `ssh_host_fingerprint`, `ssh_remote_task_dir`, `ssh_remote_session_dir`, `ssh_remote_agentctl_port`, `ssh_remote_agentctl_pid`, `ssh_local_forward_port`, `ssh_workdir_root`, `ssh_proxy_jump`, `ssh_identity_source`, `ssh_identity_file`.
- `ResumeRemoteInstance` re-opens the SSH connection per `(host, user)` (one connection serves all surviving sessions on that host), re-establishes each session's local port forward to its recorded remote port, verifies the remote agentctl is alive (`kill -0 <pid>` for liveness, then HTTP probe on the forwarded port), and re-binds the stream manager. If the remote process is gone, the resume fails and the manager falls back to creating a fresh instance.

### Settings UI

- New `apps/web/components/settings/ssh-settings.tsx` exporting two cards:
  - **`SSHConnectionCard`** — the form + "Test connection" gate. Fields: name, `host` (or `host_alias` from `~/.ssh/config`), `port`, `user`, identity source (`agent` | `file`), optional `identity_file`, optional `proxy_jump`. Save disabled until a successful test produced a fingerprint and the "Trust this host" checkbox is ticked.
  - **`SSHSessionsCard`** — mirrors `SpritesInstancesCard` and `DockerContainersCard`. Table columns: task ID, session ID, host (`user@host`), remote agentctl port, local forward port, uptime, status badge. Data sourced from `GET /api/v1/ssh/executors/{id}/sessions`. Refreshes every 90 seconds.
- Per-profile settings: `workdir_root` stored on `ExecutorProfile.Config` (so the same host can serve different profiles into different roots).

### Edit-with-live-sessions UX

- On save, if the executor has any running sessions, a confirm modal warns: *"This executor has N running session(s). They will keep running on the current host. Only new sessions started after save will use the updated config."*
- Backend accepts the save unconditionally — the warning is pure UX. Live sessions retain their `ssh_host` / `ssh_port` / `ssh_user` snapshot in `ExecutorRunning.Metadata`, which is already how recovery works after a restart, so they're insulated from config changes by construction.

### Telemetry & errors

- Same step-progress callback used by Sprites (`OnProgress`) reports the per-step status: "Connecting → Detecting remote OS → Uploading agent controller → Preparing task directory → Starting agent controller → Connecting to agent controller".
- Connection errors surface verbatim (host unreachable, auth failed, host key mismatch, arm64 unsupported) — no swallowing into generic "executor failed".

## Scenarios

- **GIVEN** a user has filled the SSH executor form and clicked Test Connection, **WHEN** the test succeeds, **THEN** the host fingerprint is shown in the UI, the user must tick "Trust this host" before Save is enabled, and on Save the fingerprint is pinned to the executor — silent first-connect TOFU never happens.

- **GIVEN** a user has a configured + trusted SSH executor pointed at `dev.example.com`, **WHEN** they start a task and select that executor, **THEN** kandev SSHes in (reusing any open connection for this host), uploads agentctl to `~/.kandev/bin/agentctl` if the sha256 differs, clones the repo into `<workdir_root>/tasks/<task-dir-name>/<repo-name>/`, starts a per-session agentctl on a fresh remote port with a fresh local SSH forward, and the user sees the same chat/shell/git UI as for any other executor.

- **GIVEN** a user keeps their SSH key in 1Password (no key file on disk) with their agent running, **WHEN** they select "ssh-agent" as identity source, **THEN** the connection succeeds without kandev ever touching key material; passphrase-protected keys without an agent are explicitly rejected with a "load this key into ssh-agent first" message.

- **GIVEN** a task already has session A running on an SSH host, **WHEN** the user opens a second session B on the same task, **THEN** kandev reuses the existing task dir (no re-clone), launches a second agentctl on a different remote port with its own local SSH forward over the same SSH connection, and both sessions stream independently against the same worktree.

- **GIVEN** two different tasks are running against the same SSH host, **WHEN** the kandev backend restarts, **THEN** `ResumeRemoteInstance` opens one SSH connection to the host, re-forwards each session's recorded remote port through that single connection, verifies `kill -0 <pid>` on each, and the UI reconnects without losing in-progress turns.

- **GIVEN** the user opens the SSH executor's settings page, **WHEN** they scroll to "Active sessions", **THEN** they see each running session on this host (task, session ID, user@host, remote port, local fwd port, uptime, status) — same shape as the Docker containers table and Sprites instances table.

- **GIVEN** a user edits the SSH executor's `host` field while two sessions are running, **WHEN** they click Save, **THEN** a confirm modal warns them the running sessions will keep going on the old host and only new sessions will use the new host. On confirm, the executor row updates and the live sessions are unaffected.

- **GIVEN** a host is reachable only via a bastion, **WHEN** the user's `~/.ssh/config` has `ProxyJump bastion.example.com` for that host alias, **THEN** kandev parses the config, dials through the bastion, and the rest of the flow is unchanged.

- **GIVEN** the remote host's key changes (re-imaged, MITM, key rotation), **WHEN** kandev next connects, **THEN** the connection is refused with a `host key changed: expected <pinned> got <new>` error, the executor status banner shows "Host key changed", and the user must re-run Test Connection and re-tick "Trust this host" in settings — no silent re-pin.

## Out of scope (v1)

- **Remote macOS / Windows.** Linux remote only; agentctl is built for Linux. macOS via SSH is a plausible v2.
- **Remote arm64.** Matches the current state of Docker. Lands when a new `build-agentctl-linux-arm64` Makefile target + arch-aware resolver are added (cross-cutting change benefiting Docker too); not part of this spec.
- **Password / keyboard-interactive auth.** Keys + agent only.
- **Passphrase-protected keys handled inside kandev.** User must load the key into `ssh-agent`.
- **Chained ProxyJump.** Single bastion only; multi-hop deferred.
- **Pushing local uncommitted changes to the remote.** Like Sprites, the remote clones from the git URL — we do not rsync the user's working tree.
- **Auto-installing dependencies on the remote.** If the user's prepare script needs `node`/`go`/`python`/`docker`, those must already be on the host. We do not bootstrap toolchains.
- **Multi-user-per-host with isolation guarantees.** v1 assumes the single SSH user on each host is trusted with everything that user can see; we do not sandbox sessions from each other beyond per-session runtime dirs.
- **Orphaned task-dir cleanup as a background job.** If a task is deleted in kandev, the remote `<workdir_root>/tasks/<task-dir-name>/` is left on disk. A v2 housekeeper can sweep stale dirs.
- **GUI editor for `~/.ssh/config`.** We read existing config; users edit it in their text editor.
- **Bring-your-own-agentctl** (skip upload, point at an existing install). Always uploaded + content-hash-cached in v1.
- **Kubernetes / pod-exec transport.** The `k8s` executor type is its own future feature.

## Future scope

- **Linux arm64 agentctl + executor support** (cross-cutting with Docker).
- **Auto-provision mode**: kandev runs a one-shot install script on the remote (apt/yum/brew) to set up node/docker/etc. before the first task — turns a bare Hetzner box into a ready-to-use kandev host.
- **Multi-user-per-host** with per-task Linux user impersonation (`sudo -u worker-N`) for genuine isolation on shared boxes.
- **Chained ProxyJump** and `ProxyCommand` support for esoteric setups.
- **macOS remote** once an `agentctl-darwin` binary exists.
- **"Push working tree" mode** for users who want to test uncommitted changes against a remote (rsync the worktree instead of cloning).
- **Pooled host capacity**: configure 3 SSH hosts as a pool, kandev round-robins / load-balances tasks across them.
- **SSH-tunneled MCP servers**: expose a user's local MCP servers to a remote-running agent via reverse forward.
- **Orphan task-dir housekeeper** as a backend background job.
- **Stream-richer prepare script execution** in SSHExecutor (currently delegated to the standard prepare-script pipeline; matching Sprites' inline progress output would improve the launch UX).

## Open questions

(All v1 design decisions resolved during planning. None outstanding at spec-approval time.)
