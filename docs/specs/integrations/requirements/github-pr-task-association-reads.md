---
status: active
system: integrations
created: 2026-10-06
owners:
  - kandev
---

# GitHub PR Task Association Reads Requirements

## Overview

Users browsing GitHub pull requests need linked Kandev tasks from the requested
workspace context. A task from a previous workspace must not become a navigation
target merely because both workspaces reference the same GitHub pull request.
Integrations owns this provider association contract, including its desktop and
phone outcomes. Workspace selection and task navigation retain their owners.

## Terminology

- **Requested workspace:** The workspace selected by the browse consumer.
- **Active context:** The application's current workspace and context generation.
- **Eligible associations:** Cached associations belonging to both the requested
  workspace and the active context. Returning to the same workspace ID does not
  make an earlier generation eligible.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001: Scoped PR-to-task reads

**Intent:** Browse consumers expose task associations only for their current
workspace context, independently of the timing of association loading.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.1:** When the requested
  workspace matches the active workspace and cached associations belong to that
  context, the reverse read shall return those associations, including before a
  pending freshness request settles.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2:** When the requested
  workspace is absent, differs from the active workspace, or differs from the
  association cache's workspace, the reverse read shall return no associations.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3:** Associations from an
  earlier context generation shall be excluded even when workspace IDs match,
  including after switching from A to B and back to A. Unidentified cache context
  shall not establish eligibility.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.4:** Changes to the requested
  workspace, active context, or cached association context/content shall update
  the reverse read without requiring a remount or a network response. This
  includes context changes while the association collection remains unchanged.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.5:** While a read is pending
  or fails, ineligible associations shall remain excluded. When eligible data is
  published, it shall become readable; an eligible successful empty result shall
  produce an empty reverse read. This contract does not guarantee an immediate
  new request or automatic recovery.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.6:** Eligible reads shall
  preserve the existing owner/repository/PR-number grouping, association record
  identities and ordering, multiple tasks per PR, and multiple repositories or
  PRs per task. Equal PR keys in different contexts shall not combine tasks.
- **AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.7:** Independent application
  stores shall derive independent results. Desktop and phone PR rows shall use
  the same eligibility: ineligible links show the existing empty task indicator
  without an old task button, and eligible links retain the current task button
  or task group and its navigation.

## Out of scope

- Association request scheduling, retries, coalescing, or recovery guarantees.
- Provider permissions, backend filtering, persistence, unlink or reconciliation.
- Changes to GitHub PR identity normalization, task navigation or row layout/copy.
- Other providers and issue associations.

## Related contracts

- [Task PR synchronization](github-task-pr-sync-coordination.md) owns task-scoped
  freshness requests, not browse reverse reads.
- [GitHub phone dashboard](github-dashboard-mobile.md) owns touch presentation.
- [System design](../system-design/github-pr-task-association-reads.md).
