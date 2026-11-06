---
status: active
system: workspaces
created: 2026-10-18
owners:
  - kandev
---

# Repository Branch Inventory Requirements

## Overview

Consumers of a local repository's branch inventory need every distinct branch
available for selection. The workspace system owns this contract because it
owns repository identity and branch data, including inventories consumed by
task creation and branch policies.

## Terminology

- **Local branch identity:** A branch in the repository's local-head namespace,
  identified by its complete branch name.
- **Remote-tracking branch identity:** A branch in the repository's
  remote-tracking namespace, identified by remote name and complete branch name.
- **Inventory:** The mixed collection of eligible local and remote-tracking
  branch identities already present in a local repository. Remote `HEAD`
  aliases are excluded. Distinct identities may point to the same commit.

## Requirements

### REQ-WORKSPACES-BRANCH-INVENTORY-001: Complete distinct branch inventory

**Intent:** Repository consumers can discover each existing branch without a
different branch's name hiding it.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-INVENTORY-001.1:** For a readable, stable local
  repository, the inventory shall contain exactly one entry for every eligible
  branch identity in the union of its local and remote-tracking namespaces,
  and no additional identities. This is a condition over the entire set;
  finding at least one local or remote branch is insufficient. Local and
  remote-tracking identities shall remain distinct even when their names share
  namespace-like prefixes, including local `remotes/origin/topic` alongside
  remote `origin/topic`.
- **AC-WORKSPACES-BRANCH-INVENTORY-001.2:** The same inventory guarantee shall
  hold for loose refs, packed refs, and mixed storage. Multiple stored
  representations of the same exact identity shall yield one entry. A loose
  representation shall retain precedence over its packed representation.
- **AC-WORKSPACES-BRANCH-INVENTORY-001.3:** Each entry shall retain its complete
  branch name, local or remote type, and remote name where applicable. Results
  shall retain local-before-remote ordering, alphabetical local names, and
  remote-name-then-branch-name ordering for remote entries. Remote `HEAD`
  aliases shall remain excluded. Current-branch metadata shall report the
  checked-out local branch independently of ref storage, or retain the existing
  empty result when detached or unreadable.
- **AC-WORKSPACES-BRANCH-INVENTORY-001.4:** Inventory reads by a saved local
  repository identity and by an explicit local path shall expose the same
  branch identities for the same repository. A linked worktree shall expose
  the shared repository inventory while reporting its own checked-out branch.
  Listing shall not mutate repository refs, commits, or checkout state.

## Out of scope

- Provider API inventories, fetch, push, pull, checkout, rebase, or execution.
- New validation, normalization, path authorization, grants, or ref eligibility.
- Public response fields, persisted selections, branch-option encoding, policy
  selection, UI layout, localization, or viewport interaction changes.
- Frontend read freshness, caching, refresh scheduling, and save ordering, owned
  independently by [repository branch reads](repository-branch-reads.md).
- Concurrent Git mutation snapshots or support for additional Git storage formats.

## Traceability

- [System design](../system-design/repository-branch-inventory.md)
- [Implementation plan](../../../plans/branch-inventory-identities/plan.md)
