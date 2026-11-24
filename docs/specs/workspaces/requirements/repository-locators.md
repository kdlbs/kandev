---
status: active
system: workspaces
created: 2026-11-08
owners:
  - kandev
---

# Repository Locator Requirements

## Overview

Workspace materialization accepts credential-free Git locators independently of the repository's position in a task.
The workspace system owns this contract because it owns repository source preparation.
Executors supply transport credentials through their existing runtime environment.

Existing attachment and base-selection requirements remain authoritative for task state and branch selection.
This document defines the previously unspecified locator-admission boundary.

## Terminology

- **Conventional SCP locator:** `git@host:relative/path`, with a DNS or IPv4 host and nonempty repository path.
- **Materialization:** Creation or verified reuse of a repository checkout inside an executor workspace.
- **Removal:** Cleanup of an already-owned checkout whose recorded origin matches the requested locator.

## Requirements

### REQ-WORKSPACES-REPOSITORY-LOCATORS-001: Credential-free remote materialization

**Intent:** Prepare supported Git sources without permitting local-file fetches or embedded credentials.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-LOCATORS-001.1:** Materialization shall accept well-formed conventional SCP locators for GitHub, GitLab, and self-managed hosts.
  Nested namespaces and path segments containing the letter `t` shall remain valid.
  Host and path shall be nonempty. The supported grammar is defined by the paired design.
- **AC-WORKSPACES-REPOSITORY-LOCATORS-001.2:** SCP admission shall reject other users, passwords, malformed hosts, absolute paths, traversal segments, and empty path segments.
  It shall reject whitespace, control characters, backslashes, query or fragment delimiters, and extra colon or authority separators.
  Unsupported SCP forms shall not fall through into acceptance as another locator type.
- **AC-WORKSPACES-REPOSITORY-LOCATORS-001.3:** Materialization shall retain supported credential-free network URLs and reject embedded URL user information, queries, and fragments.
  It shall reject local, file, absolute, drive, UNC, and relative filesystem locators, without a production fixture exception.
  Rejection shall precede Git and filesystem mutation and shall not echo the submitted locator.
- **AC-WORKSPACES-REPOSITORY-LOCATORS-001.4:** A remote task with multiple valid repositories shall prepare every required repository before agent startup.
  A primary checkout prepared by an executor script shall not conceal rejection of a valid secondary locator.
  Single-repository preparation and existing credential selection shall retain their behavior.
- **AC-WORKSPACES-REPOSITORY-LOCATORS-001.5:** Materialization and removal shall agree on supported remote locators.
  Failed batches shall remove only newly created matching checkouts and preserve reused checkouts and unrelated content.
  Existing removal-only recognition of local test origins shall not permit materialization from those origins.

## Out of scope

- Additional SSH users, SCP ports or IPv6 syntax, shell expansion, and arbitrary server-side absolute paths.
- Changing credential issuance, provider discovery, branch selection, or task inventory ownership.
- Rewriting Git origins or stored clone URLs during materialization.
- A guarantee that a syntactically valid remote is reachable or authorized.

## Related contracts

- [Attach workspace sources](../../tasks/requirements/attach-workspace-sources.md), especially AC-001.2 through AC-001.4.
- [Worktree base refresh](worktree-base-refresh.md), including independent required attachment admission.
- [System design](../system-design/repository-locators.md)
