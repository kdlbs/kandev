---
status: active
system: integrations
created: 2026-08-27
updated: 2026-08-27
owners:
  - kandev
---

# GitHub task pull request sync coordination Requirements

## Overview

Several task surfaces can display the same GitHub pull request association at
the same time. Each surface currently starts its own freshness request, retry
schedule, and reconnect recovery for the same task. The duplicate requests do
not change the result, but they increase WebSocket traffic, backend work, and
diagnostic noise.

This requirement makes task pull request synchronization a shared operation
within one active workspace context while preserving the existing detection and
recovery behavior.

## Requirements

### REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001: Coordinate task pull request synchronization

**Intent:** All mounted consumers of one task pull request association must
share one synchronization lifecycle for the same application store, workspace
context, and task.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.1:** Concurrent
  consumers in the same application store, workspace ID, workspace-context
  generation, and task ID share one initial freshness request and one retry
  schedule.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.2:** An automatic or
  manual refresh joins the active request for that context. A refresh starts a
  new request when no request is active.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.3:** Different task IDs,
  workspace IDs, workspace-context generations, or application stores remain
  independent and do not share requests or results.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.4:** Removing one
  consumer does not interrupt the lifecycle used by remaining consumers.
  Removing the last consumer stops future scheduled retries and releases the
  context's connection observer.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.5:** Empty responses,
  transient failures, permanent failures, retry exhaustion, pull request
  discovery, and reconnect transitions retain the existing retry and recovery
  outcomes, with only one owner for each context.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.6:** A response from an
  inactive workspace context cannot update the active workspace's pull request
  associations or synchronization state.
- **AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.7:** Every consumer of a
  shared context observes the same loaded state and the same store-backed pull
  request associations. Existing manual refresh and unlink behavior remains
  available through the task pull request hook.

## Scope

### In scope

- Frontend ownership of the `github.task_pr.sync` request lifecycle.
- Initial freshness synchronization, bounded retries, and reconnect recovery.
- Shared loading completion for all consumers of one task context.
- Cleanup when consumers or workspace contexts change.

### Out of scope

- Backend request coalescing or rate-limit policy changes.
- Changes to `github.task_pr.sync` payloads or responses.
- Pull request association persistence, unlink semantics, or presentation.
- Tooltip-only HTTP hydration and workspace-wide pull request loading.
- Session Git status subscription behavior.

## Regression scenarios

- **GIVEN** two mounted components request the same task in one workspace
  context, **WHEN** the task has no pull request, **THEN** one initial request
  and one request per retry tick are sent.
- **GIVEN** two consumers share a request, **WHEN** both request a manual refresh
  before it settles, **THEN** both receive the same completion and only one
  WebSocket request is sent.
- **GIVEN** two consumers share a retry lifecycle, **WHEN** one unmounts,
  **THEN** retries continue for the remaining consumer; **WHEN** the last one
  unmounts, **THEN** no later retry is sent.
- **GIVEN** a retry budget is exhausted or marked permanent, **WHEN** the
  connection later transitions into connected, **THEN** one immediate resync
  starts one fresh retry window.
- **GIVEN** an older workspace context has an unresolved request, **WHEN** the
  active workspace context changes before that request settles, **THEN** the
  older response does not update the active context.

## Verification

Focused frontend tests count outbound synchronization requests and observe
shared hook state across multiple consumers. They also cover context isolation,
consumer cleanup, manual refresh, retry exhaustion, permanent responses, and
reconnect recovery.

This change has no rendered or interaction change. Desktop and mobile surfaces
continue to consume the same hook contract, so no new viewport-specific E2E
scenario is required.
