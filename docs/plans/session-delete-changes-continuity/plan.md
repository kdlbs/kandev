---
created: 2026-08-30
status: implemented
requirements:
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
legacy_specs: []
---

# Implementation Plan: Preserve Changes Data After Session Deletion

## Overview

Correct the environment-stable session selector so deleting its cached session
promotes the surviving active session as the agentctl lookup handle. Add the
selector regression first. Then prove that Changes keeps workspace and matching
pull-request data after same-environment active-session deletion.

## Scope

### In scope

- Preserve the existing no-refetch behavior for ordinary switches between live
  sessions that share a task environment.
- Replace a cached session handle after its environment mapping is purged and a
  surviving active session maps to the same environment.
- Keep environment-keyed Git, cumulative-diff, and pull-request data visible in
  the desktop Changes panel after active-session deletion.
- Cover the shared data path used by the existing mobile Changes surface.

### Out of scope

- Backend session deletion, task-environment ownership, or Git/PR APIs.
- Changes panel layout, navigation, touch behavior, copy, or empty-state design.
- Session-scoped file-review markers and their cache ownership.
- Changing when ordinary same-environment tab switches refetch workspace data.
- Recovering tasks that have no surviving session lookup handle.

## Confirmed root cause

`useEnvironmentSessionId` caches a session ID by environment identity. During
active-session deletion, `useSessionActions.remove` selects the surviving
session first. Then `removeTaskSession` purges the deleted session's environment
mapping. Both sessions have the same environment ID. Thus, the selector keeps
the deleted session after the mapping disappears. Git and commit hooks then use
caches keyed by the deleted session ID. Cumulative-diff requests fail with
`task session not found`. Branch-scoped PR selection also drops the fetched PR
files.

The smallest reliable reproduction uses two sessions that share one task
environment. Load Changes through the first session. Delete that active session
and reopen Changes through the surviving session. The current implementation
shows the empty state. The environment cumulative diff and PR queries still
contain files.

## Technical approach

### Environment-stable lookup handle

Update `apps/web/hooks/use-environment-session-id.ts`. Reuse the cached session
ID only when it is active or its current mapping equals the active environment.
If the cached mapping is absent, use the current active session.
Preserve the session-ID fallback while the active session's environment mapping
loads.

Add `apps/web/hooks/use-environment-session-id.test.ts`. The regression named
`promotes the active same-environment session after the cached session is
purged` must fail before the correction. It must pass after the correction.
Adjacent cases retain the cached handle during a live same-environment switch.
They replace it during an environment switch.

### Visible session-deletion regression

Extend `apps/web/e2e/tests/session/session-tab-management.spec.ts` with
`deleting the active shared-environment session keeps Changes data visible`.
Use the existing two-session setup. Make sure that both sessions reference the
same environment. Seed a workspace change and a branch-matching mocked pull
request. Open Changes through the deletion target. Then delete it through the
visible session-tab UI. Reopen Changes. Make sure that the workspace file and
PR file remain visible through the surviving session.

The desktop test exercises the shared `useChangesPanelData` path. No mobile
Playwright test is required because the correction changes only data-handle
normalization. `MobileChangesPanel` already consumes the same hook. This fix
does not change mobile composition, navigation, scrolling, touch targets, or
viewport behavior.

## Tests

- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9` maps to
  `apps/web/hooks/use-environment-session-id.test.ts`, including the failing
  deleted-cache-handle regression and live same-environment stability case.
- Existing session-runtime purge tests continue to prove that only the deleted
  session mapping is removed while the shared environment cache survives.

## E2E tests

- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9` maps to
  `apps/web/e2e/tests/session/session-tab-management.spec.ts` in the Chromium
  project. The scenario deletes the active shared-environment session through
  the UI and proves workspace and PR files remain visible in Changes.
- Mobile parity is covered at the shared data-hook boundary because this fix
  makes no rendered or responsive change. The shipped exemplar remains
  `apps/web/components/task/mobile/mobile-changes-panel.tsx`, which reuses
  `useChangesPanelData`, `ChangesPanelHeader`, and `ChangesPanelBody`.

## Work orders

- [x] [Task 01: Preserve the live Changes lookup handle](task-01-preserve-live-changes-handle.md)

## Verification results

- The selector regression failed before the production correction because it
  retained `session-first` after that session's environment mapping was purged.
- The focused Vitest suite passes with four cases.
- The web TypeScript check passes.
- The focused Chromium production-build E2E case passes with the workspace and
  PR files visible after deletion.

## Risks

- Cache invalidation on every active-session switch causes needless
  subscriptions and fetches. The correction must distinguish deleted handles
  from live same-environment handles.
- An active session must remain usable while its environment mapping loads.
  Treating every missing mapping as deletion causes a startup regression.
- The E2E pull request must match the live checkout branch and repository. This
  prevents a fixture mismatch from changing the result for the wrong reason.
