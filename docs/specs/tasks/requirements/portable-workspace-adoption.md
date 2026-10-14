---
status: active
system: tasks
created: 2026-10-11
updated: 2026-10-12
owners:
  - kandev
---

# Portable workspace adoption requirements

## Overview

An operator can explicitly adopt an existing independent clone into one damaged
host Worktree slot while preserving the existing task, session, environment,
attachment and logical worktree. Healthy ordinary checkout admission is already
upstream. This action repairs a binding, not Git contents or deployment policy.

Decision: [explicit checkout adoption](../../../decisions/2026-10-11-portable-checkout-adoption.md).

## Terminology

- **Selected slot:** One active canonical repository entry in the existing session's environment.
- **Candidate:** An existing independent clone under that environment's owned task root.
- **Retired marker:** The exact original Git pointer whose registered administration is missing.
- **Receipt:** Backend provenance for a committed adoption, subject to current ownership validation.

## Requirements

### REQ-TASKS-PORTABLE-ADOPTION-001: Explicit authority

**Intent:** Recovery changes only the operator's selected physical binding.

#### Acceptance criteria

- **AC-TASKS-PORTABLE-ADOPTION-001.1:** Preview and validation are read-only. Apply requires an
  explicit administrator request, ordinary task access and a complete preview token.
  The administrator must authenticate with a verified session or personal access
  token. Auth-disabled synthetic identities and agent-origin requests cannot
  invoke the operator HTTP surface.
- **AC-TASKS-PORTABLE-ADOPTION-001.2:** Only host Worktree environments are eligible. Remote
  and Local executor workspace contracts remain unchanged, including Docker executors.
- **AC-TASKS-PORTABLE-ADOPTION-001.3:** Apply requires the existing idle session incarnation,
  unchanged complete inventory and task attachments, and an exclusive recovery claim.
  Live runtime consumers, cleanup or ownership transfer prevent publication.

### REQ-TASKS-PORTABLE-ADOPTION-002: Content and identity preservation

**Intent:** The operator can select a prepared clone without replacing conversation identity.

#### Acceptance criteria

- **AC-TASKS-PORTABLE-ADOPTION-002.1:** Task, session/incarnation, environment/owner generation,
  repository, task attachment, repository-slot and logical-worktree identifiers are retained.
  Session history, root path, selected branch and sibling ordering are unchanged.
  If a native session's working directory is the selected checkout, its environment
  and bound session paths move to the candidate atomically with the physical binding.
  A working directory at the task root remains unchanged.
- **AC-TASKS-PORTABLE-ADOPTION-002.2:** Both directories, old marker bytes and all tracked,
  staged, dirty, untracked and ignored candidate files remain untouched. Adoption performs
  no clone, branch creation, checkout, reset, content copying, cleanup or prompt replay.
- **AC-TASKS-PORTABLE-ADOPTION-002.3:** The candidate has independent pinned Git metadata,
  the registered origin and an existing commit on the selected branch descended from
  the registered source branch commit. Foreign roots,
  symlinks, redirected common/object directories and competing bindings refuse adoption.
  Ancestry uses raw objects; replacement refs cannot supply proof. Legacy grafts
  and external alternate object storage refuse adoption. Legitimate packed refs remain valid.
- **AC-TASKS-PORTABLE-ADOPTION-002.4:** A malformed or foreign old pointer, restored
  administration, changed marker or changed candidate identity refuses adoption.

### REQ-TASKS-PORTABLE-ADOPTION-003: Durable publication

**Intent:** A retry cannot publish stale provenance or partially change a binding.

#### Acceptance criteria

- **AC-TASKS-PORTABLE-ADOPTION-003.1:** The binding and receipt commit atomically after
  selection CAS and pinned-filesystem revalidation. Failure publishes neither.
- **AC-TASKS-PORTABLE-ADOPTION-003.2:** Identical operation replay returns the original receipt
  only while its current authority is valid. Changed operation contents refuse replay.
- **AC-TASKS-PORTABLE-ADOPTION-003.3:** Receipt validation binds original marker path/hash,
  missing administration, candidate path/origin/branch, owner generation and slot identity.
  Changed ownership or restored metadata invalidates it. No marker exemption or writable
  Git metadata permission is implied.
- **AC-TASKS-PORTABLE-ADOPTION-003.4:** SQLite and PostgreSQL publication obey the same
  transaction and authority contract. Receipts survive restart and schema initialization replay.

## Out of scope

Deployment SQLite scripts, guard integration, existing local receipt import,
ordinary healthy-checkout admission, automatic clone discovery, branch recovery,
lost Git objects/index reconstruction, task Docker execution and live deployment.
