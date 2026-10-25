---
spec: docs/specs/kubernetes-executor/spec.md
created: 2026-09-01
status: complete
---

# Implementation Plan: Kubernetes Executor Task Status Preview Repair

## Overview

The Kanban and sidebar executor glyph is rendered by
`RemoteCloudTooltip`, but `useLiveRemoteStatus` returns before doing any work
unless its local `open` state is true. A newly mounted card therefore has no
`remote_checked_at`, `getCloudState` deliberately classifies it as `stale`, and
the glyph remains muted until the first hover opens the Tooltip. The focused
test currently reinforces that behavior by calling `openTooltip()` before it
expects `getKubernetesTaskSession` or the compatibility WebSocket request.

The same component renders its disclosure as an unstructured
`space-y-0.5` list inside an uncontrolled Tooltip and wraps the glyph in a
non-focusable `span`. By contrast, `PRTaskIcon` uses a controlled, focusable
trigger, `useChangeRequestTaskTooltipState`, a viewport-bounded 320 px surface,
and `ChangeRequestTaskStatusSummary` with an identity header and aligned
semantic rows. This explains both reported symptoms without involving the
Kubernetes backend, Kanban mapping, or missing task identities:
`KanbanCardBody` and `TaskItemContent` already pass the exact task, primary
session, executor ID, executor type, and fallback name.

The repair moves exact remote-task status into a small shared frontend resource
that eagerly hydrates valid rendered scopes, deduplicates simultaneous Kanban
and sidebar consumers, and keeps hover/focus as an explicit refresh. It then
gives the executor glyph the same visual and interaction quality bar as the PR
glyph, with a coarse-pointer Drawer rather than a hover-only path. Kubernetes
continues to use the exact session inventory endpoint; other remote executors
continue to use `task.session.status`.

---

## Root-Cause Evidence

- `apps/web/components/task/remote-cloud-tooltip.tsx` passes `open` into
  `useLiveRemoteStatus`; its effect exits on `!open`. `status` is consequently
  `null` on mount and `getCloudState(null)` is `stale`.
- `apps/web/components/task/remote-cloud-tooltip.test.tsx` opens the mocked
  Tooltip before asserting either status request. There is no eager-mount or
  duplicate-consumer regression.
- `apps/web/components/kanban-card-content.tsx` already supplies
  `primarySessionId`, `primaryExecutorId`, `primaryExecutorType`, and task ID.
  `apps/web/components/task/task-item.tsx` receives the same exact identities
  through `task-session-sidebar-item.ts`; prop projection is not the defect.
- The executor trigger is `span.cursor-default` and the content is plain text.
  `apps/web/components/github/pr-task-icon.tsx` is the requested in-product
  comparison: it supplies keyboard focus/Escape behavior and renders a
  `w-80 max-w-[calc(100vw-1rem)] p-3` structured summary.
- `apps/web/lib/types/http-kubernetes.ts#KubernetesSession` already exposes Pod
  name, phase/container state, restarts, workspace kind, creation time, and a
  sanitized failure reason. No backend or wire-shape change is needed.

---

## Frontend

### Eager, shared exact-status resource

- Create a focused resource under
  `apps/web/hooks/domains/session/remote-executor-status-resource.ts` and a
  `useRemoteExecutorStatus` hook beside it. Model a request with executor type,
  optional executor ID, task ID, and primary session ID; derive a collision-safe
  scope key from all four identities.
- Keep source selection exactly where it exists today:
  `getKubernetesTaskSession(executorId, taskId, sessionId)` for `k8s`, and
  `getWebSocketClient().request("task.session.status", ...)` for compatible
  remote executors. Project `KubernetesSession` into a display snapshot that
  retains `restarts` and `workspace_kind` instead of discarding those facts.
- Follow the external-resource pattern in
  `apps/web/hooks/domains/github/pr-commits-resource.ts`: stable snapshots,
  `useSyncExternalStore`, one promise per exact scope, listener notification,
  last-used tracking, and bounded LRU eviction. Successful results remain fresh
  for 90 seconds; failures remain visible but retry on the next mount or
  disclosure open rather than being suppressed for the full freshness window.
- On first mount of a valid scope, call `load` without waiting for the Tooltip to
  open. Simultaneous Kanban/sidebar instances subscribe to and join that one
  request. A forced `refresh` used by hover or keyboard focus also joins an
  existing request and retains the previous snapshot while `loading` is true.
- Do not request with an empty task/session or with a Kubernetes scope lacking
  its executor ID. Scope changes subscribe to the new entry immediately, and a
  late response remains isolated under its old exact key. Preserve the existing
  `status` prop as an authoritative no-fetch compatibility path.
- Keep errors causal and sanitized. The icon must derive its tone from the
  resource snapshot and never promote an absent/malformed scope to `ready`.

### PR-quality responsive disclosure

- Refactor `RemoteCloudTooltip` into a thin responsive shell over a new
  `RemoteExecutorTaskStatusSummary` component. Keep the exported component name
  for its Kanban, task-sidebar, and mobile-topbar callers, but remove network
  state ownership from the presentation component.
- Extract the pointer/focus/Escape behavior in
  `use-change-request-task-tooltip-state.ts` into a domain-neutral
  `apps/web/components/task/use-task-icon-tooltip-state.ts`. Keep the existing
  change-request export as a compatibility wrapper so PR and registered-review
  icons do not change behavior while the executor icon reuses the same
  interaction contract.
- On fine pointers, use a controlled Tooltip, a focusable translated trigger,
  `sideOffset={6}`, and the same bounded `w-80`/viewport/padding surface as
  `PRTaskIcon`. The summary header uses the executor glyph and exact remote or
  Pod identity; aligned rows show state, Kubernetes restart count and workspace
  mode when available, created time, last check, and a distinct sanitized error
  state. State text and semantic icons accompany color.
- Preserve the small visual glyph in dense Kanban/sidebar chrome. Loading may
  animate a quiet status affordance but must not replace retained facts or shift
  task-row layout. Missing identity/result copy says unavailable; it must not
  synthesize `Ready`.
- On coarse pointers, use `useTouchDrawer` and the existing Drawer primitives to
  present the same shared summary. Give the trigger a 44 px effective hit area
  without increasing the visible glyph or row height, stop its activation from
  selecting the underlying task card/row, support Enter/Space, constrain the
  body with internal scrolling, and honor bottom safe-area padding.
- Add all new labels through `task` locale keys in English, pseudo, Portuguese,
  Simplified Chinese, and both Traditional Chinese catalogs. Generate the
  Traditional Chinese pair with `pnpm run i18n:zh-hant`; do not hardcode visible
  copy.

---

## Tests

- **What:** a valid Kubernetes scope requests exact status immediately on mount,
  before the Tooltip opens, and publishes its semantic icon tone.
  **File:**
  `apps/web/components/task/remote-cloud-tooltip.test.tsx` plus the new resource
  test.
  **How:** render without calling the Tooltip open helper; resolve a deferred
  `KubernetesSession`; assert exact arguments, loading transition, Pod fields,
  and the healthy/error class.
- **What:** two mounted consumers for the same exact scope make one request,
  hover/focus joins an eager in-flight request, a later open forces one refresh,
  failed reads are retryable, and obsolete scopes cannot publish into the active
  view.
  **File:**
  `apps/web/hooks/domains/session/remote-executor-status-resource.test.ts` and
  `remote-cloud-tooltip.test.tsx`.
  **How:** use deferred requests, fake time for the 90-second freshness boundary,
  subscription counts, scope changes, and explicit release/remount cases.
- **What:** non-Kubernetes executors remain on `task.session.status`, an external
  `status` prop performs no request, and invalid identities perform no request
  or false-ready projection.
  **File:** `remote-cloud-tooltip.test.tsx`.
- **What:** the fine-pointer executor trigger is focusable and translated,
  hover/focus/Escape follow the PR icon contract, and the summary has a bounded
  identity header plus aligned state/restarts/workspace/timestamp/error rows.
  **Files:** `remote-cloud-tooltip.test.tsx` and a focused new
  `remote-executor-task-status-summary.test.tsx`.
- **What:** coarse-pointer activation opens a Drawer with the same facts, a
  44 px effective target, internal scrolling, safe-area treatment, keyboard
  activation, and no parent task-card selection.
  **Files:** the same component tests plus a focused TaskItem/Kanban integration
  test where propagation matters.
- **What:** the existing PR and registered change-request task glyphs retain
  their pointer, focus, Escape, color, and summary behavior after extracting the
  shared tooltip-state hook.
  **Files:** existing `pr-task-icon` and registered-change-request icon tests.

---

## E2E Tests

- Amend
  `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts` so Kanban and
  task-sidebar scenarios assert the exact Kubernetes session request and healthy
  or failed icon tone before any hover. Then hover/focus and assert the
  PR-style identity header, aligned Pod rows, and a causal refreshed response.
- Amend
  `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts` with a
  coarse-pointer task-list/card scenario: tap the compact glyph, assert the
  bottom Drawer contains the same exact Pod facts, dismiss it, and verify the
  task was not selected as a side effect. Keep the existing task-page executor
  Drawer scenario intact.
- Cover both a successful Pod and a sanitized unauthorized/unavailable response
  without printing credentials or raw Kubernetes response bodies.

---

## Public Documentation

- Update the task-status sections in `docs/public/k8s.md` and
  `docs/public/executors.md` to state that Kanban/sidebar executor glyphs hydrate
  status on render, show a structured exact-session summary on pointer/focus,
  and use a touch Drawer on coarse-pointer devices.
- Keep `k8s.md` primarily a how-to/reference page and `executors.md` primarily an
  executor reference. Do not add a new page or change `docs/public/meta.json`.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in the primary conversation. The resource,
component shell, and E2E selectors share contracts, so no task is marked
parallel-safe.

Wave 1:
- [x] [Task 01: Hydrate shared task executor status](task-01-hydrate-shared-status.md)

Wave 2:
- [x] [Task 02: Polish the responsive executor preview](task-02-polish-responsive-preview.md)

Wave 3:
- [x] [Task 03: Verify responsive flows and update docs](task-03-responsive-e2e-docs.md)

---

## Verification Results

- Focused Vitest: 5 files and 49 tests passed, including eager hydration,
  exact-scope sharing, retryable failures, LRU eviction, responsive disclosure,
  mobile projection, and unchanged PR/change-request indicators.
- Frontend `typecheck` and full zero-warning `lint` passed.
- Desktop managed Chromium E2E passed 1/1 first attempt with `--retries=0`;
  mobile Chrome passed 1/1 first attempt with `--retries=0` after a fresh build.
- Localization checks passed for all six catalogs; the i18n reference audit
  found 7,233 referenced keys and the new-copy ratchet was clean.
- Public-doc validation passed 61 tests and validated 41 published pages.
- The E2E sleep ratchet, focused Prettier checks, and `git diff --check` passed.

---

## Risks And Constraints

- Eager per-card reads can create a request burst on large Kanban boards. The
  exact-scope promise cache, 90-second successful-result freshness, bounded LRU,
  and no-request invalid-scope guard are required acceptance behavior, not later
  optimization.
- The same task can render in Kanban, a sidebar, and mobile task switchers.
  Shared status must be keyed by executor type/ID plus task/session, not stored
  as local component state or keyed by session alone.
- A task can switch primary sessions while an old request is unresolved. Old
  results may remain cached for their old key but cannot recolor or populate the
  current indicator.
- The task row itself is interactive. The coarse-pointer status trigger must not
  cause task selection, drag start, or nested-button markup regressions.
- Error color alone is insufficient. Every status remains available as text and
  a semantic icon, and loading preserves the last known result.
- The worktree contains the prior Kubernetes executor implementation and repair
  changes. Implementation must preserve those overlapping edits and avoid
  broad formatting or cleanup.

## Out Of Scope

- Backend changes, database migrations, task-status-summary schema changes, or
  a new batch endpoint.
- Periodic polling while a tab remains idle. This repair covers eager mount,
  foreground reuse, bounded freshness, and explicit pointer/focus refresh.
- Changing Kubernetes authorization, exact inventory validation, executor
  lifecycle, settings hierarchy, or cleanup controls.
- Redesigning PR/MR/change-request status content beyond the behavior-preserving
  shared trigger-state extraction.
