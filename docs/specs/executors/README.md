---
status: draft
system: executors
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Executor system

## Purpose

The executor system owns the runtime environments that execute agent work,
including local, container, and SSH execution boundaries.

## Ownership

This system owns executor profiles, environment construction, SSH lifecycle,
runtime resource admission, process and port safety, and executor-specific
failure and recovery contracts.

## Exclusions

- Agent identity and provider capabilities belong to the [agent
  system](../agents/README.md).
- Task ownership of worktrees belongs to the [task system](../tasks/README.md).
- Desktop process supervision belongs to the [desktop system](../desktop/README.md).

## Specification map

### Requirements



- [Executor-Profile Environment Precedence](requirements/executor-profile-env-precedence.md)
- [Port collision and backend ownership safety](requirements/port-collision-safety.md)
- [SSH Executor](requirements/ssh-executor.md)
- [Remote SSH task-directory reclamation](requirements/remote-task-directory-reclamation.md)

### System design



- [Executor-Profile Environment Precedence System Design Part 1](system-design/executor-profile-env-precedence-01.md)
- [Executor-Profile Environment Precedence System Design Part 2](system-design/executor-profile-env-precedence-02.md)
- [Executor-Profile Environment Precedence System Design Part 3](system-design/executor-profile-env-precedence-03.md)
- [Executor-Profile Environment Precedence System Design Part 4](system-design/executor-profile-env-precedence-04.md)
- [Executor-Profile Environment Precedence System Design Part 5](system-design/executor-profile-env-precedence-05.md)
- [SSH Executor](system-design/ssh-executor.md)
- [Remote SSH task-directory reclamation](system-design/remote-task-directory-reclamation.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Agents](../agents/README.md): supplies the agent command and profile.
- [Tasks](../tasks/README.md): owns task-scoped execution lifecycle.
