---
title: "Executors"
description: "Choose and configure local, worktree, Docker, SSH, or Sprites task environments."
---

# Executors

An executor determines where Kandev creates a task environment and runs `agentctl`, the selected agent, terminals, and Git commands. An executor profile supplies reusable settings for that executor. A task environment is the concrete workspace created for one task; several sessions may reuse it.

## Current support

| Executor      | Current status                                                                  | Workspace                                                               | Use it when                                                              |
| ------------- | ------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| Worktree      | Supported; normal default                                                       | Dedicated Git worktree on the Kandev host                               | Parallel coding on a trusted machine                                     |
| Local         | Supported                                                                       | The selected checkout, or an explicit folder for a repository-free task | One controlled task must work in that exact folder                       |
| Local Docker  | Supported when the global Docker runtime is enabled and its daemon is reachable | `/workspace` in a new Docker container                                  | You need a repeatable container boundary                                 |
| Sprites.dev   | Supported, provider-dependent                                                   | `/workspace` in a provider sandbox                                      | You need remote compute and accept provider lifecycle/billing            |
| SSH           | Supported for repository sources on a trusted host                              | A task folder on a trusted SSH host                                     | You need a remote host with SSH, SFTP, forwarding, and clone credentials |
| Remote Docker | **Not implemented**                                                             | None                                                                    | Do not select or create this type                                        |

`mock_remote` also exists in backend models for tests. It is not a product executor.

Remote Docker deserves explicit treatment: the backend registers the runtime type, but its create and stop methods return `remote_docker runtime is not yet implemented`. The current **Settings > Executors** hub does not offer it. Older routes and stored fields such as `docker_host`, `docker_tls_verify`, and `docker_cert_path` do not make it operational.

## Create and select a profile

Open **Settings > Executors**, then choose **Local**, **Worktree**, **Docker**, **Sprites.dev**, or **SSH** under **Create New Profile**. Local and Worktree profiles already exist in a new database.

<DocsVideo
  webm="./media/feature-guides/profile-executor-selection.webm"
  mp4="./media/feature-guides/profile-executor-selection.mp4"
  poster="./media/feature-guides/profile-executor-selection.webp"
  title="Choose an agent and executor profile"
  caption="Agent, model, repository, and executor choices are reviewed before starting a task."
/>

A profile stores:

- its name;
- environment variables, either as a literal value or a Kandev secret reference;
- a prepare script and cleanup script;
- an MCP policy JSON object;
- runtime-specific configuration.

Literal environment values are stored with the profile. Use secret references for credentials. Resolved values and copied credential files normally become accessible to the agent and commands in that environment. SSH is narrower: its remote agent process receives only the credential allowlist documented below, not arbitrary profile variables.

The MCP editor checks only that the value is a JSON object. Its presets cover stdio, HTTP, and SSE transport allowances, server allowlists, and URL rewrites. Test restrictive policies with the actual MCP servers the agent needs; see [Automation and MCP](automation-and-mcp.md).

Profile edits apply when Kandev provisions a launch, but a Docker container or Sprite resume can reconnect to the already provisioned process, image, environment, credentials, and files. Use **Reset Environment** or explicitly destroy the resource when a change must take effect on a fresh environment. Deleting or editing a profile does not tear down an already-running resource.

### Script behavior is runtime-specific

Do not treat the two script fields as universal hooks:

| Runtime          | Prepare script                                                                                                        | Profile cleanup script                                                                                                                                                                                                                                                  |
| ---------------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local / Worktree | Runs on the host during preparation. A failure is shown but is non-fatal, so the agent can still start for diagnosis. | Not executed by the executor runtime. Repository-level worktree cleanup is a separate repository setting.                                                                                                                                                               |
| Local Docker     | Runs inside the container before `agentctl`. Failure is logged but `agentctl` still starts.                           | Not executed.                                                                                                                                                                                                                                                           |
| Sprites          | Runs inside a newly created sandbox. Failure aborts the launch and destroys that new sandbox.                         | Runs, with a 60-second limit, only when a live execution is stopped with a task/session archived or deleted reason; failure does not prevent the subsequent destroy attempt. Plain Stop, **Reset Environment**, and profile-page direct destroy do not run this script. |
| SSH              | **Currently not executed.**                                                                                           | **Currently not executed.**                                                                                                                                                                                                                                             |

Keep working prepare scripts noninteractive and idempotent. Kandev resolves supported placeholders and appends its managed branch checkout for Docker and Sprites after the user script. A profile cleanup script must never remove paths outside the environment it owns.

Two current preparation exceptions are easy to miss:

- A repository-free Local task bypasses the environment-preparer stage, even when it uses an explicit workspace folder. Its profile prepare script therefore does not run.
- A Worktree task with two or more attached repositories runs each repository's setup script while creating that repository's worktree, but the current multi-repository preparer does not run the executor profile's task-level prepare script.

## Worktree

Worktree creates a dedicated host Git worktree and runs the standalone `agentctl` service against it. It separates branches and files between tasks, but the process still has the Kandev user's host permissions, network access, and readable credentials.

Repository settings control base branch, branch naming, pull-before-create, repository setup/cleanup scripts, and optional copies of ignored files. Copy ignored files narrowly: `.env` and similar files often contain production secrets. Multi-repository tasks receive one materialized worktree per attachment; use the per-repository setup scripts because the profile-level prepare script is currently skipped for that path.

Normal stop keeps the task environment available. Task deletion or **Reset Environment** removes the tracked worktree when configured to clean worktrees. Preserve or push valuable changes first; see [Git Operations](git-operations.md).

Typical failures:

- dirty or conflicting source repository state;
- base branch missing locally or remotely;
- worktree path already registered in Git metadata;
- setup dependencies absent on the host;
- repository cleanup failure leaving a stale worktree.

Use `git worktree list --porcelain` in the source repository when diagnosing stale registrations. Do not delete a worktree directory by hand before checking whether Git still tracks it.

## Local

Local runs directly in the selected checkout. It provides no file isolation: concurrent tasks, the user, and other tools can edit the same files. When sources are attached, Local uses each user-owned repository's current checkout; Kandev does not switch its branch.

Use Local for an intentionally shared checkout, a controlled single task, or a repository-free task with an explicit workspace folder. Prefer Worktree for parallel coding. Stop ends the agent process but does not clean the checkout or undo its changes.

## Workspace sources

An idle, non-archived repository-backed task can add sources from its **Files** panel. Repository sources (saved workspace repository, local Git repository, or remote Git repository) are supported on **Worktree**, **Local/Local PC**, **Local Docker**, **SSH**, and **Sprites**. Worktree materializes Remote Git from Kandev's owned host cache. Docker, SSH, and Sprites clone local Git sources and therefore require a cloneable origin; Worktree and Local/Local PC can use the host repository directly.

Every repository row records a base branch. Worktree, Docker, SSH, and Sprites may also materialize an existing checkout branch for repository rows. Local/Local PC always uses the repository's current checkout and does not offer or perform a branch switch.

Arbitrary folders are supported only on **Worktree** and **Local/Local PC**. They remain live host paths; Kandev links them into its task workspace and never copies, moves, or deletes their contents. Docker and remote executors do not offer folders and reject a forged folder request. Remote Docker remains unavailable because its runtime is not implemented.

Source batches are atomic: if validation, cloning, or runtime adoption fails, Kandev removes the new records and Kandev-owned entries while preserving existing task contents. Persisted attachments are reapplied after reload, relaunch, or **Reset Environment**; a previously attached folder that later disappears is reported instead of silently skipped. See [Tasks and workflows](tasks-and-workflows.md#add-sources-to-an-existing-task).

## Local Docker

### Prerequisites and profile creation

Install a reachable Docker Engine and leave `docker.enabled: true` (the non-containerized backend default). The published Kandev service image overrides this to `false`; see [Docker](docker.md#using-docker-for-agent-environments). The runtime health method is currently a no-op and the client is initialized lazily, so a green control-plane startup does not prove daemon access; image build or first task launch is the effective check.

Choose **Settings > Executors > Docker**. The current UI requires an image tag, Dockerfile content, and a successful **Build Image** operation before it creates the profile. **Use defaults** supplies:

- image tag `kandev/multi-agent:latest`;
- `node:22-slim`;
- `git`, CA certificates, and `curl`;
- `/workspace` as the working directory.

The build request sends a single Dockerfile-only context to the configured daemon. `COPY` cannot see repository files. Every Dockerfile instruction runs with the daemon's authority, so profile creation is an administrative operation on that daemon.

At launch Kandev:

1. uses the profile's `image_tag`;
2. creates `kandev-agent-<execution-prefix>` with Kandev task/session labels;
3. bind-mounts a released Linux `agentctl` helper read-only at `/usr/local/bin/agentctl`;
4. publishes control and agent ports to random ports on Docker-host loopback;
5. runs the resolved prepare script, which normally clones attached repositories into `/workspace` and checks out the Kandev branch;
6. starts `agentctl` even if prepare failed, then creates the agent instance.

The repository workspace itself is not a normal host bind mount. For a local filesystem clone URL, Kandev temporarily mounts that local clone source read-only so the in-container `git clone` can read it. Images need the selected agent's dependencies; they do not need to contain `agentctl`.

The daemon connection comes from global Kandev configuration. At present, the client uses `docker.host` and optional `docker.apiVersion`. The accepted `docker.tlsVerify`, `docker.defaultNetwork`, and `docker.volumeBasePath` settings are not applied by the current Docker client/container manager. Per-executor `docker_host` values are also not used by this runtime.

The current container manager always selects the Linux/amd64 `agentctl` helper. Use a Linux/amd64-compatible agent image and daemon (native or correctly emulated); native ARM64 agent containers are not yet wired to the released ARM64 helper.

Kandev passes each agent definition's CPU and memory limits to Docker. These are agent implementation defaults, not executor-profile controls. Apply additional daemon, cgroup, storage, and network policy outside Kandev when required.

### Credentials and security

Docker profiles can inject resolved environment secrets. For agent file-based authentication, Kandev selectively seeds a per-execution directory under `<KANDEV_HOME_DIR>/agent-sessions/` and mounts that directory at the agent's expected config path. It does not intentionally mount the entire host home.

A container is a useful boundary, not a hostile-code security sandbox. The Docker daemon has host-level power, bind mounts expose their sources, the agent can use every injected secret, and the default image has outbound network access. Kandev does **not** mount the Docker socket into agent containers automatically.

Plain Stop preserves a healthy container for resume. A later launch reconnects to an existing running container, or starts one in a stopped/exited state; if reconnect fails, it creates a fresh container. Archive, delete, stale cleanup, explicit removal in the profile page, and **Reset Environment** can stop or force-remove it. Inspect matching containers before manual cleanup:

```bash
docker ps -a --filter label=kandev.managed=true
```

## Sprites.dev

### Configure

1. Save the provider token as a Kandev secret.
2. Choose **Settings > Executors > Sprites.dev**.
3. Select that secret for the required `SPRITES_API_TOKEN` profile environment variable.
4. Review remote credential methods, Git identity, prepare/cleanup scripts, and network policy.

New Sprites profiles initially select the local `gh` CLI token method. Kandev may also copy explicitly selected agent credential files, resolve selected Kandev secrets into agent environment variables, or run an agent auth setup script. Credential upload is best-effort: provisioning can continue while later agent authentication fails. The remote sandbox receives highly sensitive data; use a scoped provider token and least-privilege repository credentials.

Network rules are stored in `sprites_network_policy_rules` as JSON entries with `domain`, `action` (`allow` or `deny`), and optional `include`. Kandev applies them only on fresh sandbox creation, and currently does so after credential upload, prepare, controller startup, and agent-instance creation. Bootstrap traffic can therefore occur before the profile policy is installed. A parse/provider failure is reported as skipped and does not abort launch. Provider semantics remain authoritative; do not treat this late, best-effort step as a security boundary, and test the resulting policy.

Fresh launch creates a sandbox named `kandev-<execution-prefix>`, uploads the Linux/amd64 `agentctl`, uploads credentials, runs prepare, starts the controller, and opens a local proxy to its control port. The current Sprites path does not probe sandbox architecture; it assumes x86-64. A failed fresh launch destroys the new sandbox. Resume reconnects to the recorded sandbox; if it no longer exists or has expired, Kandev warns and provisions a fresh one on the recorded branch.

Plain Stop preserves the sandbox and workspace for resume. Archive/delete terminal stops attempt to destroy it, and the profile page can list and explicitly destroy Kandev-named sandboxes with the selected provider token. **Reset Environment** also requests sandbox destruction, but the current direct-reset path does not carry the profile's Sprites secret into that destroy request; after a reset or backend restart, verify the old sandbox in the profile page and destroy it there if it remains. Provider retention, quotas, network behavior, and billing remain provider-dependent. Destroying a sandbox out of band breaks any session that still references it.

## SSH

SSH is implemented as a separate remote connection per session. Kandev uploads a platform-matched `agentctl` helper over SFTP, starts it in the remote task directory, and forwards its port to local loopback.

### Host requirements

- Linux `amd64`/`arm64` or macOS `amd64`/`arm64`;
- SSH public-key authentication and SFTP;
- `bash` on Linux or `zsh` on macOS by default, or a compatible configured login shell;
- TCP forwarding enabled by `sshd` and enough `MaxSessions` capacity;
- the selected agent command already installed and visible to a login shell;
- writable remote home and adequate disk/process capacity.

Released Kandev bundles include helpers for all four platform combinations. The full automated SSH task E2E target currently exercises a Linux/amd64 container; other platform gates and helper selection are unit-tested.

### Create the connection

Choose **Settings > Executors > SSH**. Enter a name plus either a Host or a host alias from `~/.ssh/config`. The backend resolver can inherit `HostName`, `Port`, `User`, `IdentityFile`, and one `ProxyJump`; explicit form values win. The current create form defaults and persists Port `22` and identity source `ssh-agent`, so enter a non-22 alias port and desired identity source explicitly instead of assuming those two values inherit. `IdentitiesOnly` and arbitrary OpenSSH directives are not consumed by Kandev.

Authentication choices are:

- `ssh-agent (SSH_AUTH_SOCK)`; or
- an unencrypted private-key file.

Password and keyboard-interactive authentication are not supported. A passphrase-protected key file must first be loaded with `ssh-add`, then used through ssh-agent.

Run **Test Connection**, independently verify the observed SHA256 host fingerprint, select **Trust this host**, then save. Kandev pins the final target fingerprint and refuses a changed key. With ProxyJump, the target remains pinned, but bastion handling is weaker: Kandev checks `~/.ssh/known_hosts` when available and rejects a changed known key, while an unknown bastion key is accepted on first use. Verify and pre-populate the bastion key yourself.

The profile editor exposes remote shell and agent-readiness checks. Backend/API configuration also recognizes `ssh_workdir_root` (default `~/.kandev`) and `ssh_shell`; the current profile UI exposes `ssh_shell` but not a workdir-root field.

The remote-auth card is built from the currently enabled agents. Depending on an agent's declared methods, it can copy selected local credential files, resolve a stored secret into that agent's authentication environment variable, or run an agent-specific setup script on the remote host. GitHub can instead use a stored `GITHUB_TOKEN` or the local `gh auth token`. These transfers write sensitive material under the remote user's home and are best-effort—verify authentication on the remote after saving. Although the profile editor also stores Git name/email controls for SSH, the current SSH runtime does not apply them; configure Git identity on the remote host yourself.

### Repository sources and cleanup

SSH materializes attached repository sources in the remote task directory. It still ignores profile prepare and cleanup scripts. Ensure the remote host has the required Git credentials and can reach every selected remote; folders cannot be attached to SSH tasks.

The runtime preflights the selected agent command and reports an installation hint when missing; it does not install the agent or its toolchain. Only these resolved credential environment names are forwarded to the remote agent: `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `GITHUB_TOKEN`, and `GH_TOKEN`. Arbitrary profile variables and control-plane process variables are not forwarded to that agent process. Agent-specific auth setup scripts can still consume a selected stored secret and materialize their own remote login state.

Stop attempts to kill the session's remote `agentctl` and remove only the remote session-runtime directory, then closes forwarding and SSH. Remote cleanup is best-effort: when the connection has already failed, the process or session directory can remain. The task directory always remains and no background sweeper currently removes it. The cached helper and checksum at `~/.kandev/bin/agentctl` and `agentctl.sha256` also remain for later sessions. Periodically audit the remote process list, session directories, and `<workdir-root>/tasks/` after confirming no session needs the data. Resume re-dials SSH and reuses a live recorded PID when possible; otherwise Kandev starts a fresh remote controller.

## Lifecycle and cleanup

The task environment reports `creating`, `ready`, `stopped`, or `failed`; individual execution records have finer states. Stop is deliberately not synonymous with destroy for resumable Docker and Sprites environments.

Use a task's **Reset Environment** action when you need a clean materialization. Kandev blocks reset while a task session is starting or running, can optionally push the current branch, and requests teardown of the recorded worktree/container/sandbox. It normally keeps the environment record when teardown returns an error. The current Sprites credential-context limitation described above can instead report success while leaving the provider sandbox, so verify it separately. SSH task-directory cleanup remains manual as described above.

Before deleting any environment, push or otherwise preserve uncommitted work. Profile deletion and provider/daemon-side deletion can bypass normal lifecycle safeguards.

## Troubleshooting

- **Profile missing at launch:** ensure the executor is active, the profile still exists, and the task/workflow references the correct IDs.
- **Prepare reports failure but agent starts:** expected for Local, Worktree, and Docker; inspect the failed step output and retry commands inside the same environment.
- **Docker unavailable:** verify global `docker.enabled`, effective `docker.host`, daemon permission, image existence, and the released Linux helper path.
- **Docker clone fails:** test the clone URL, base branch, DNS, CA trust, and token scope from inside the selected image.
- **Sprite cannot resume:** check provider token, quota, sandbox existence, expiration, and network policy; a missing sandbox triggers fresh provisioning.
- **SSH handshake fails:** test ssh-agent/key access, host fingerprint, bastion trust, SFTP, TCP forwarding, and remote OS/architecture.
- **SSH agent is missing:** run the reported `command -v` check through the configured login shell and install the agent on that host.
- **Disk usage grows:** inspect **Settings > System > Disk usage**, Docker containers, provider sandboxes, host worktrees, and retained SSH task directories before removal.

Related guides: [Docker](docker.md), [Git Operations](git-operations.md), [Operations](operations.md), and [Windows Support](windows-support.md).
