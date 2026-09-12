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

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to
find current sources.

The [Kubernetes executor foundation](../kubernetes-executor/spec.md) remains
the current lifecycle contract. The three Kubernetes pairs own additive
presets, launch diagnostics, and retained-compute visibility. They do not
replace the foundation's resource ownership, recovery, or cleanup rules.

## Related systems

- [Agents](../agents/README.md): supplies the agent command and profile.
- [Tasks](../tasks/README.md): owns task-scoped execution lifecycle.
