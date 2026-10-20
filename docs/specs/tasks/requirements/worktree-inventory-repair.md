---
status: active
system: tasks
created: 2026-09-28
owners:
  - kandev
---

# Worktree inventory repair requirements

## Overview

An older installation can retain healthy Git checkouts whose saved repository,
branch, or task-root identities disagree with the filesystem. Cleanup and resume
must continue to refuse uncertain ownership. An operator needs a bounded repair
that restores agreement while preserving the work and the previous audit record.
The task system owns this contract because task environments own the inventory
used by session admission and durable cleanup jobs.

This is explicit maintenance of selected records. It does not extend automatic
[Git metadata recovery](worktree-metadata-recovery.md) to foreign task roots.

## Requirements

### REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001: Exact, reversible inventory repair

**Intent:** Repair selected legacy records without adopting or losing unrelated work.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1:** Inspection shall identify the exact
  environment, repository slot, saved and observed branch, checkout, and root
  owner. Preview shall not modify the database, checkouts, refs, or ownership markers.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2:** Application shall require an explicit
  repair description with expected original identities and proposed replacements.
  A changed record, ambiguous repository, live backend, live workspace consumer,
  unsafe path, or unverifiable Git registration shall refuse application.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.3:** Repair shall preserve all commits,
  existing refs, tracked, untracked, and ignored files, the index, modes, and links.
  It shall not change HEAD, reset a checkout, or turn an old task root into a
  different task's root by overwriting its ownership marker.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.4:** A checkout explicitly selected for
  relocation into its shared environment's canonical root shall keep its content
  and Git registration. Only its exact inventory slot and dependent workspace
  references shall change. Every relocation shall explicitly include a workspace
  repair for its environment and every bound session. Parent-child membership
  alone shall not authorize automatic adoption of a foreign root.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.5:** Repair shall retain a private backup
  and durable progress record. Interruption shall allow the same operation to
  continue or roll back after identity checks; it shall not permit the backend
  to serve a partially repaired inventory.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.6:** Read-only and read-write repair
  database connections shall open the database file at its canonical absolute
  path. This includes a Windows drive-letter path and a path containing a space,
  `#`, `%`, or, where file names allow it, `?`. Read-only connections shall
  reject writes. Neither mode shall create or open another file.

### REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002: Audit-preserving cleanup recovery

**Intent:** Resume the affected cleanup without rewriting historical evidence.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1:** A repository identity repair shall
  be accepted only for one authorized repository whose Git registration and
  common directory match the exact selected checkout. Missing or competing
  candidates shall leave the original records and checkout unchanged.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.2:** Repair shall retain the original
  cleanup snapshot and any source manifest. An incomplete old job that needs
  different evidence shall be superseded by a linked recovery attempt observing
  the current state; new observations shall not be presented as historical ones.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3:** After repair, cleanup shall still
  require current archive identity, no active borrowers, proven path ownership,
  stopped owned runtimes, and the normal source-capture and branch-preservation
  rules. Checkout dirtiness shall follow the current
  [archive policy](dirty-worktree-archive.md), including immediate removal under
  REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004. An unaudited change to branch or checkout
  identity shall still block removal. Repair itself shall preserve local files
  until an authorized cleanup removes the checkout.

### REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003: Accurate recovery diagnostics

**Intent:** Distinguish an already-absent execution from a failed stop operation.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.1:** When a cleanup stop identifies an
  execution as absent through the runtime's typed result, the executor shall not
  emit a failed-stop warning. The result shall remain identifiable to its caller.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.2:** A timeout, transport error, unknown
  liveness, or other stop failure shall remain a visible failure and continue to
  prevent destructive cleanup where required by the runtime cleanup contract.

### REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004: Legacy duplicate ownership reconciliation

**Intent:** Let operators repair older task inventories that give one physical
checkout multiple saved owners or paths, without losing task conversations or work.

This extension is delivered by the
[legacy duplicate repair package](../../../plans/legacy-worktree-alias-repair/plan.md).
Existing repair guarantees remain applicable.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.1:** Read-only discovery shall identify
  duplicate active checkout identities across an installation, including every
  claiming environment and repository slot. Results shall be bounded and
  resumable. Discovery shall not change task state or inspect arbitrary folders.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.2:** Repair shall require an explicit
  canonical selection and the complete set of competing records. Exactly one
  selected checkout shall satisfy repository, branch, registration, root-owner,
  and shared-workspace evidence. A newer timestamp, empty branch label, task
  title, or parent relationship alone shall not establish ownership.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.3:** Reconciliation shall remove only
  explicitly selected redundant metadata. It shall preserve the canonical
  checkout, repository slots for other branches, source files, index, refs,
  ownership markers, task identities, hierarchy, conversations, and resume tokens.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.4:** An inherited child with a redundant
  environment shall keep its sessions bound to the proven canonical shared
  environment after repair. Every repository owned by the redundant environment
  shall have an identical canonical counterpart. An independent repository,
  foreign member, conflicting owner, or unsupported dependent record shall
  refuse the complete selected repair without mutation.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.5:** Application shall require an offline
  installation and confirmed absence of every affected process. Idle session
  state alone shall neither authorize nor prohibit repair. Repair shall retain
  session state and use a private backup and recoverable journal. A stale preview
  or interruption shall never publish a partially reconciled ownership graph.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.6:** Existing cleanup snapshots,
  manifests, and receipts shall remain unchanged. A pending cleanup, recovery
  operation, or immutable reference that cannot survive the selected changes
  shall block this repair. A mixed selection shall not repair its safe subset
  while concealing a blocked member.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.7:** After repair and restart, ordinary
  archive and delete shall inspect the canonical checkouts. A child operation
  shall preserve the parent's workspace while another member retains it. Dirty
  files, uncertain ownership, and changed commits shall retain their normal
  refusal behavior. Metadata verification shall not imply task removal succeeded.
- **AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8:** Ordinary identity reads and writes
  shall reject competing active ownership instead of selecting or changing an
  arbitrary claimant. Session joins and historical records shall not be mistaken
  for additional active owners. A rejected operation shall preserve all claimants.

## Exclusions

- General automatic worktree adoption, remote filesystem repair, or PostgreSQL
  maintenance tooling in this package.
- Weakening [source evidence](archive-source-manifest.md),
  [runtime cleanup](runtime-cleanup.md), or
  [archive cleanup](dirty-worktree-archive.md) requirements.
- New browser controls, provider conversation resets, branch deletion, force
  cleanup, or automatic restart of the user's active backend.

## Related documents

- [System design](../system-design/worktree-inventory-repair.md)
- [Implementation plan](../../../plans/worktree-inventory-repair/plan.md)
- [Legacy duplicate repair package](../../../plans/legacy-worktree-alias-repair/plan.md)
