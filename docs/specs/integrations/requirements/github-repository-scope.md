---
status: active
system: integrations
created: 2026-11-12
owners:
  - kandev
---

# GitHub Repository Scope Requirements

## Overview

A workspace needs one GitHub scope that includes selected organizations and
individual repositories together. An administrator selects **All repositories**
or supplies either selection, or both.

The integration system owns this authorization policy and its settings UI.
Repository discovery remains a separate picker contract. Authentication,
workspace ownership, and task repository attachment retain their existing owners.

This requirement defines the behavior requested by
[issue #5284](https://github.com/kdlbs/kandev/issues/5284). It proposes a change
to the exclusive scope choices in
[ADR 0030](../../../decisions/0030-workspace-scoped-integration-settings.md).
The identity boundaries in
[ADR 0047](../../../decisions/0047-github-authentication-ownership.md) still apply.

## Terminology

- **All repositories:** Every repository otherwise accessible to the applicable
  workspace connection and user. This selection grants no provider permissions.
- **Selected scope:** The union of selected owners and exact repositories.
  The organization picker also accepts a GitHub account login, as the existing
  picker permits.
- **Selected owner:** Every repository owned by that selected organization or
  account, subject to the applicable credential permissions.
- **Inactive selection:** A stored list that the previous exclusive mode ignored.
  Its presence does not prove authorization.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001: Combined GitHub repository scope

**Intent:** Express one workspace policy without exclusive organization and
repository modes, while preserving the configured access of existing workspaces.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.1:** Repository Scope shall
  expose an **All repositories** toggle. When enabled, the workspace shall
  retain all access permitted by its existing credential and identity boundaries.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.2:** When the toggle is off,
  a repository shall qualify if at least one selected owner or exact repository
  matches. Comparisons shall ignore letter case and surrounding whitespace.
  With both selections empty, the scope shall permit no repositories.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.3:** When the toggle is off,
  both pickers shall be available together. Shared settings Save and Discard
  shall apply to the complete selection. A failed save shall preserve the draft
  for retry. Changes during a pending save shall remain editable and unsaved.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.4:** An upgrade shall preserve
  each workspace's configured repository allowset. Previously inactive entries
  shall never become authorized through the upgrade or a later restart.
  This includes empty restricted selections and records with both lists stored.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.5:** Backend checks shall
  apply the same selected scope to repository operations, PR and issue search,
  polling, watches, automations, and mentions where scope is already enforced.
  Personal credentials shall not expand the workspace policy or automation visibility.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.6:** PR and issue searches
  shall include results from both selections. An overlapping repository shall
  contribute each result once. Scope qualifier composition shall preserve custom
  query constraints and exclude results outside the workspace policy.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.7:** A watch without an
  explicit repository filter shall use the workspace union. An explicit filter
  shall further restrict that union. Empty workspace scope shall return no
  candidates, without an unrestricted repository search or task dispatch.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.9:** The settings API shall
  preserve the existing field names and list shapes. Canonical GET responses
  shall return `all` or `selected`, replacing the previous `orgs` and `repos`
  response values. Legacy exclusive requests shall retain exclusive meaning.
  Unknown modes shall fail without widening
  access. An explicit empty list shall clear that selection. Omitted or null
  lists shall preserve their current value within an unchanged selected policy.
  An explicit mode change shall apply that mode's list-reset behavior.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.10:** Settings copy and workspace
  clone shall preserve both active selections. Settings copy shall not copy
  credentials or installation bindings. Workspace clone shall retain its separate
  authentication contract. A GitHub host replacement shall reset scope to an
  empty selected scope. Reconnecting the same host shall preserve its policy.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.11:** Watch and automation
  pickers shall accept repositories from either selection. They shall permit
  an owner wildcard only when the workspace permits that entire owner.
  One selected repository shall not authorize its owner's other repositories.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.12:** Phone users shall edit,
  save, discard, and reload the same union as desktop users. The settings shall
  reuse the shared pickers' accessible phone behavior. Labels, help, empty states,
  and errors shall be localized.

## Deferred enforcement

Draft criterion `AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.8` is withdrawn
from this package. Its identifier is reserved and shall not be reused.
It proposed a new managed-broker workspace-scope boundary, including live
scope reduction during credential redemption. That boundary does not exist in
the inspected implementation. It requires separate follow-up work.

This package changes existing scope checks and preserves the App token's
current per-repository restriction. It does not claim to satisfy a broader
interpretation of the issue's task-credential enforcement request.

## Out of scope

- Additional GitHub connections, cross-host identities, or new provider permissions.
- New workspace-scope enforcement in credential-broker admission, redemption,
  refresh, or reissue. Existing token restrictions remain in place.
- Task repository attachment and the task creation repository picker.
- Changes to watch filter storage or automation trigger selection semantics.
- Provider search limits, global result ordering, or a new search engine.
- Restriction of credentials that a trusted task already received, or explicit
  executor token overrides and executor-inherited authentication.
- Rebuilding the shared pickers supplied by
  [PR #5283](https://github.com/kdlbs/kandev/pull/5283).

## System design and delivery

- [Repository scope design](../system-design/github-repository-scope.md)
- [Implementation package](../../../plans/github-repository-scope-union/plan.md)
