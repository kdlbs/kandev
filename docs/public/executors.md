---
title: "Executors"
description: "Choose and configure local, worktree, Docker, SSH, remote Docker, or Sprites task environments."
---

# Executors

An executor decides where Kandev materializes a task workspace and runs agentctl, terminals, Git operations, and the agent subprocess. An executor profile stores reusable environment policy for that runtime.

## Runtime choices

| Executor | Workspace and process location | Isolation and prerequisites |
|---|---|---|
| Local | Selected local checkout; process runs on the Kandev host. | Lowest setup cost and lowest isolation. Avoid concurrent writers. |
| Worktree | Dedicated Git worktree on the Kandev host. | Isolates branches/files between tasks, but shares the host OS, user, and accessible credentials. |
| Local Docker | Container managed by the host Docker engine. | Requires Docker and an image/profile. Container is a stronger process/filesystem boundary than a worktree. |
| Remote Docker | Container managed through a configured remote Docker host. | Requires remote daemon connectivity and credential/network planning. |
| SSH | Workspace and agentctl on a reachable Linux host. | Requires SSH authentication, remote dependencies, and enough disk/process capacity. |
| Sprites | Remote sandbox provided through Sprites. | Requires provider credentials and network access; environment lifetime and billing follow the provider. |

Kandev also has mock executor types for tests. They are not production runtime choices.

## Executor profiles

A profile can contain:

- a name and runtime-specific configuration;
- prepare script run while creating/preparing the environment;
- cleanup script run during teardown where supported;
- environment variables as plaintext or Kandev secret references;
- MCP policy;
- image, host, credential, or provider settings specific to the executor.

Scripts run with the executor's permissions and can modify the task environment. Keep them idempotent, fail loudly, avoid interactive input, and pin important tooling. A cleanup script should not delete paths outside the environment it owns.

## Pick an executor by risk and workload

Use **Worktree** for normal parallel coding on one trusted machine. It prevents two task branches from writing the same checkout but does not protect the host from agent commands.

Use **Local Docker** when repository setup is reproducible in an image and you want a container boundary. Plan how source, caches, credentials, ports, and Docker socket access are mounted; mounting the host Docker socket grants powerful host control.

Use **SSH**, **Remote Docker**, or **Sprites** when local CPU, memory, architecture, network placement, or long-running availability is unsuitable. The remote host must be able to fetch every attached repository and reach required package registries/integrations.

Use **Local** only when intentionally operating the selected checkout. It can be correct for no-repository tasks or a controlled single task, but it is easy for concurrent work to collide.

## Task environments and sessions

Kandev tracks task environment state as creating, ready, stopped, or failed. The environment owns the workspace and agentctl control server. Multiple sessions can share it, and each session has an executor-running record with runtime, process/container identifiers, worktree path/branch, health timestamps, and errors.

Stopping a session does not always delete the task environment. Resumable runtimes can retain it for a later turn. Archive/delete/cleanup behavior depends on task action, profile, and runtime.

## Repository and credential behavior

- Git operations execute inside the task environment through agentctl.
- Multi-repository tasks materialize every attachment and address repositories by subpath.
- Worktree tasks can copy selected ignored files, such as `.env` files, from the source repository. Configure this narrowly; ignored files often contain secrets.
- Local host credentials are not automatically available in a container or remote machine. Use scoped secret/profile configuration or the runtime's supported credential copy flow.
- SSH credential-copy options can expose local auth files to the remote environment. Review the selected files and remote host trust first.

## Resource metrics and capacity

Kandev can show host CPU, memory, disk, temperature, and load plus execution-environment metrics for supported remote/container runtimes. Enable the desired metrics in General settings. Metrics are observability, not hard resource limits; set limits in Docker/provider/host policy where needed.

Parallel agents can saturate CPU, memory, disk I/O, process limits, network, or provider quotas. Prefer several bounded remote environments over one overloaded host when tasks are large.

## Troubleshooting

- **Prepare failed:** inspect the environment error and logs, then run the script in the same shell/runtime with noninteractive settings.
- **Agentctl unavailable:** confirm the helper was built/copied for the remote architecture and that its control port is reachable inside the environment.
- **Repository clone fails:** test remote URL, branch, DNS, SSH known hosts, and token scope from the execution host.
- **Container starts but tools are missing:** rebuild/pin the image or install tools in the prepare script.
- **SSH task dies after disconnect:** verify remote process supervision and inspect the stored remote PID/agentctl logs.
- **No disk space:** remove stale task environments/worktrees after confirming branch state, and inspect **Settings > System > Disk usage**.

Related deployment guides: [Docker](docker.md), [Remote environments](remote-cloud-environment.md), [Windows](windows-support.md), and [Operations](operations.md).
