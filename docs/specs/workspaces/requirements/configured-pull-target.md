---
status: active
system: workspaces
created: 2026-10-13
owners:
  - kandev
---

# Configured Pull Target Requirements

## Overview

Pull integrates the checkout's tracked source even when its remote or source
branch differs from `origin` or the local branch name. A successful Pull must
not leave the checkout behind that source merely because a different remote
was already up to date.

Workspaces owns this contract because it owns repository Git state and remotes.
The [configured push contract](configured-push-target.md) owns publication
destinations separately. [Managed contributions](../../tasks/requirements/remote-contribution-tasks.md)
retain their explicit source routing; [workspace status](../../platform/requirements/workspace-git-status.md)
owns the upstream observations consumed by Changes.

## Terminology

- **Configured upstream:** The single upstream Git reports for the current
  local branch, defined by the checkout's Git configuration.
- **Pull source:** That upstream's configured remote and source ref in the
  remote repository. A remote-tracking ref's local storage name is not the
  source ref's name.
- **Managed source:** A task's existing remote-contribution source binding.
  A contribution-destination binding selects a push destination separately.

## Requirements

### REQ-WORKSPACES-CONFIGURED-PULL-TARGET-001: Configured upstream Pull

**Intent:** Bring the checkout up to date with the source whose upstream
commits Changes identifies as available to Pull.

#### Acceptance criteria

- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.1:** When no managed source applies
  and Git reports a configured upstream, Pull shall integrate that upstream's
  source remote and ref. This shall work for a non-`origin` remote and for a
  differently named source branch, independently or together. For a clean
  working tree with no local-ahead commits and exactly one commit behind that
  source, successful Pull shall leave HEAD equal to the source commit rather
  than report success against another remote.
- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.2:** The configured source selection
  shall apply to both existing Pull request modes. A rebase request shall retain
  its existing rebase behavior, including replay of non-conflicting local work.
  Existing integration failures and conflict recovery shall remain observable.
- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.3:** An existing managed source shall
  retain precedence over ordinary configured upstream selection. A push-only
  contribution destination or Git push-remote setting shall not replace the
  ordinary Pull source. Pull shall not alter remote URLs, upstream configuration,
  push routing, or the caller's selected repository.
- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.4:** When Git reports no upstream and
  no managed source applies, Pull shall retain its existing `origin/main`, then
  `origin/master`, then current-local-branch fallback. An ordinary same-name
  `origin` upstream shall continue to work.
- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.5:** Source selection shall preserve
  configured remote and source-ref identity, including safe remote names with
  slashes, custom remote-tracking storage names, and Git's local `.` upstream.
  A reported upstream whose source cannot be resolved to safe operands under
  Kandev's existing boundary shall fail rather than select a different source.
  Pull shall not accept an arbitrary repository URL as a new target.
- **AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.6:** Desktop and phone Changes
  shall receive the same configured-source outcome through their existing Pull
  actions and result contract. Existing upstream-based eligibility, request
  inputs, success/error reporting, and presentation shall remain unchanged.

## Out of scope

- New target selectors, new request/result fields, UI copy or control changes.
- Multi-source octopus integration, discovering an unmaterialized upstream,
  or changing the existing no-upstream detection policy.
- Push, preflight, upstream or remote mutation; signing, hooks, credentials,
  transport selection, automatic retries, command admission or deadlines.
- Status refresh, caches, journals, lifecycle changes, or permission changes.

## System design

See the [configured Pull design](../system-design/configured-pull-target.md).
