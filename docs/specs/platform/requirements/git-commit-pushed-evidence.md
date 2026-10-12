---
status: active
system: platform
created: 2026-10-03
owners:
  - kandev
---

# Git commit pushed evidence requirements

## Overview

Local commit readers need faithful evidence of publication even when no provider
change request exists. Platform owns this shared Git metadata contract.
Workspaces owns repository identity and comparison bases; Tasks owns provider
contribution provenance and version-resolution policy. This document records
the existing meaning of the published `pushed` field without extending either
adjacent contract.

## Terminology

- **Tracked upstream:** The current repository branch's configured tracking ref,
  resolved from local Git data. It can lag the server; reading history does not
  refresh it.
- **Reachable:** The commit is the upstream tip or an ancestor through any parent.
  Similar messages, patches, authors, or dates do not establish reachability.

## Requirements

### REQ-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001: Faithful local publication evidence

**Intent:** A bounded history read must not classify local work as published
because another part of the graph consumed its evidence window.

#### Acceptance criteria

- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1:** For every returned local commit, when upstream reachability can be established, `pushed` shall be true if and only if that exact commit is reachable from the resolved tracked upstream. Reachability through the upstream's second or later merge parents shall count. Absence from an incomplete or differently selected history sample shall not establish publication.
- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.2:** A branch-range read shall retain its existing first-parent membership and order even when newer merged side commits remain local. A recent-history read without a base shall retain full-graph membership and its requested or default limit, including side commits. Both modes shall classify each returned row correctly in mixed published/local histories.
- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.3:** When no tracked upstream exists or optional reachability evidence fails, every affected returned row shall retain conservative `pushed=false`. A successful commit-list read shall remain usable with its existing success and error fields; false under unavailable evidence shall not claim confirmed remote absence.
- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.4:** Selected and aggregate repository reads shall use each repository's own upstream independently. The same commit SHA in two repository contexts may have different pushed values; aggregate repository identity shall remain attached to each row.
- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.5:** Correcting pushed evidence shall preserve all published fields, commit membership, order, parent identity, timestamps, file statistics, comparison bases, limit semantics, and provider overlays. Desktop and phone Changes consumers shall receive the corrected local evidence through their existing projection without changing provider provenance or remote-action authorization.
- **AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.6:** History reads shall retain batched, bounded optional evidence work and existing command environment, admission, deadlines, and cancellation. They shall not fetch, push, change tracking configuration, or mutate the checkout or index to establish publication.

## Out of scope

- Provider history, PR/MR association, contribution relation classification, and mutation authorization.
- Comparison-base recovery, commit-file navigation, merge-detail presentation, or diff-file status.
- New API fields, caches, graph services, configuration, UI layout, copy, or touch behavior.

## Design

- [Git commit pushed evidence design](../system-design/git-commit-pushed-evidence.md)
