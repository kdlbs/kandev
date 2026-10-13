---
status: active
system: integrations
created: 2026-10-13
owners:
  - kandev
---

# GitHub CI PR freshness Requirements

## Overview

GitHub CI automation must reuse recently synchronized PR status so automatic
status events cannot repeatedly force provider refreshes. The integration
system owns this provider-read contract; task automation controls remain with
their existing owner.

## Terminology

- **Fresh status:** A linked PR observation with a synchronization timestamp
  no more than the existing 30-second freshness window old.
- **Refresh-eligible evaluation:** An automation evaluation permitted to refresh
  provider status, with auto-fix or auto-merge enabled for the evaluated PR.

## Requirements

### REQ-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001: Reuse fresh PR status in CI automation

**Intent:** Bound automatic GitHub traffic while preserving feedback processing
and the existing merge safety gates.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.1:** When a refresh-eligible evaluation receives fresh
  status, it shall evaluate the enabled automation without forcing another
  PR status synchronization. This applies independently to auto-fix,
  auto-merge, and both switches enabled.
- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2:** When a refresh-eligible evaluation receives status
  older than the freshness window or without a synchronization timestamp, it
  shall request synchronization before evaluating automation. The evaluated PR's
  switches shall determine eligibility even when another linked PR differs.
- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.3:** When auto-fix receives fresh status and actionable
  completed feedback, it shall continue sending or queueing the eligible fix
  prompt under the existing checkpoint and round rules. Reusing status shall
  not suppress the required feedback read.
- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.4:** Auto-merge shall remain subject to the existing
  readiness gates, use the evaluated head commit, and require fresh status. A
  refresh returning stale status shall not authorize merging; an actual refresh
  failure shall retain the existing error and lifecycle behavior.
- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.5:** Evaluations already designated to reuse status,
  and evaluations with neither auto-fix nor auto-merge enabled for that PR,
  shall not gain a forced status synchronization.

- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.6:** A partial feedback
  observation shall not renew status freshness for aggregates it did not fetch.
  It shall retain the preceding status-sync timestamp for the same head and
  base, including a missing timestamp, and invalidate that timestamp when
  either identity changes. Stored state and published updates shall agree.
- **AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.7:** A provider feedback event
  shall invalidate the affected automation feedback cache before evaluation,
  so a recent cached response cannot suppress newly observed feedback.
  Internal feedback-save events shall continue reusing the cache and fresh
  PR status.

## Out of scope

- Review-policy changes, including whether comment-only reviews are pending.
- REST/GraphQL review and commit-status normalization.
- Changing explicit user refreshes, option-save synchronization, poll intervals,
  provider rate admission, cache TTLs, or worker coalescing.
- UI, configuration, schema, or API changes.

## Related contracts

- [Task PR automation controls](../../ui/requirements/ci-pr-automation.md).
- [PR polling admission](github-pr-discovery-health.md).
- [Task PR frontend sync coordination](github-task-pr-sync-coordination.md).
