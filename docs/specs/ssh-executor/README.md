---
status: active
system: ssh-executor
specification_version: "1"
migration: in_progress
owners:
  - kandev
---

# SSH executor system

## Purpose

The SSH executor runs Kandev agent sessions on a remote host the user already
owns, reached over SSH. It owns the remote workspace layout, the per-session
`agentctl` process and port forward, host-key trust, remote binary and
credential delivery, and the lifecycle of everything Kandev writes to that
host.

## Ownership

This system owns the remote `<workdir_root>/tasks/<task-dir-name>/` layout, the
prepare and cleanup hooks that run on the SSH target, remote `agentctl`
placement and startup, SSH connection reuse and port forwarding, host
fingerprint pinning, and the reclamation of remote task directories that Kandev
itself created.

## Exclusions

- Local worktree creation and removal belong to the
  [workspace system](../workspaces/).
- The durable task-resource cleanup job that drives terminal cleanup belongs to
  the [task system](../tasks/); this system contributes the SSH-specific step it
  runs.
- Sprites and Docker executor behavior is out of scope.

## Specification map

### Requirements

- [Remote task-directory reclamation](requirements/remote-task-directory-reclamation.md)

### System designs

- [Remote task-directory reclamation](system-design/remote-task-directory-reclamation.md)

### Legacy documents

[`spec.md`](spec.md) remains the authoritative v1 record for everything this
system does apart from the capabilities listed above. It is not yet migrated
into per-capability requirement and design records.
[`e2e-plan.md`](e2e-plan.md) records the end-to-end test plan.
