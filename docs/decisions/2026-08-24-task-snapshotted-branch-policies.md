# Snapshot Repository Branch Policies on Tasks

**Status:** proposed
**Date:** 2026-08-24
**Area:** backend, frontend, persistence
**Amends:** [ADR 0032](0032-configurable-worktree-branch-names.md)

## Context

A Gitflow-style repository needs several named combinations of base branch,
branch-name template, and pull-request target. Registering the same local path
several times makes repository identity ambiguous and was removed by deliberate
repository deduplication.

Branch policies can change or be deleted after a task starts. If running tasks
read live repository policy rows, an administrator edit could change a later
branch rename or pull-request target without the task author choosing it.

## Decision

One canonical repository owns zero or more branch policies. A policy is reusable
configuration, not another repository identity and not a synthetic Git branch.

Task creation sends a policy ID. The backend authorizes and resolves that ID in
the task-create transaction and snapshots the policy name, base branch, branch
template, and pull-request target onto the task-repository row. Runtime behavior
uses the snapshot. Agent instructions for pull-request creation also use this
snapshot, not the live policy. The policy ID is historical provenance and is
not a foreign key, so deleting configuration cannot erase or mutate the
snapshot.

The existing repository `worktree_branch_template` remains the fallback for
raw branch selections and older tasks. No existing repository template is
automatically converted into a policy.

## Consequences

- Policy edits affect future tasks only.
- Task behavior remains explainable after the originating policy is deleted.
- Task-repository storage duplicates a small amount of policy data.
- Every branch-producing runtime path must resolve the same effective snapshot
  before falling back to repository configuration.
- Every policy-backed agent context must name the snapshotted pull-request
  target. Policy edits cannot change this instruction for an existing task.
- The selector must use typed policy options; display text cannot carry domain
  identity.

## Alternatives considered

### Read the live policy at runtime

Rejected because changes would retroactively alter task behavior and deletion
would make task execution ambiguous.

### Copy only the policy ID and forbid deletion while referenced

Rejected because it couples repository housekeeping to task retention and still
lets edits change running tasks.

### Register one repository per Gitflow branch type

Rejected because a filesystem path is one repository identity. Duplicate rows
also split repository settings and make task/repository authorization harder to
reason about.

### Replace the repository fallback template

Rejected because existing task payloads and repositories need a policy-free
compatibility path.
