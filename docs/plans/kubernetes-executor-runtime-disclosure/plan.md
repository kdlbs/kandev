---
spec: docs/specs/kubernetes-executor/spec.md
created: 2026-08-25
status: complete
---

# Implementation Plan: Kubernetes Task Runtime Disclosure

## Overview

The task-header disclosure currently treats Kubernetes as an untyped remote
environment: `EXECUTOR_ICON_MAP.k8s` selects `IconCloudComputing`,
`GET /api/v1/tasks/:id/environment/live` returns no Kubernetes block, and
`buildFields` has no `k8s` branch. The live seeded Kind task therefore renders
`ready` from `TaskEnvironment.status` beside “No resource details available”
even though the existing Kubernetes session API can verify the exact Pod and
return its live state. The repair first adds an exact task/session filter to
that existing API, then makes one shared desktop/touch disclosure consume it
and replaces every Kubernetes cloud glyph with a Pod-shaped cube.

No new persistence, cluster mutation, or lifecycle operation is introduced.
The existing session Stop/Resume and terminal Archive/Delete paths remain the
only Pod/PVC lifecycle controls.

---

## Backend

### Exact task/session status selection

- Extend `apps/backend/internal/kubernetes/sessions.go` with a small typed
  `SessionFilter` accepted by `Handler.listSessions`. Apply `task_id` and
  optional `session_id` to `executors_running` rows before `sessionRow` performs
  task authorization or an exact Pod GET. Preserve the existing unfiltered
  settings-page behavior.
- Update `apps/backend/internal/kubernetes/handlers.go` so
  `httpListSessions` reads `task_id`/`session_id` query parameters and
  `wsListSessions` accepts the same optional fields. Reject `session_id`
  without `task_id`; a well-formed mismatch returns an empty list without a
  Kubernetes API call.
- Keep `sessionRow`, `validateSessionInventory`, and `matchesSessionIdentity`
  as the sole trust boundary. The filter narrows which authoritative rows are
  evaluated; it must not permit a caller-supplied Pod name, namespace, or UID.

No database migration or new route is required.

---

## Frontend

### Kubernetes status client and polling state

- Add `getKubernetesTaskSession` to
  `apps/web/lib/api/domains/kubernetes-api.ts`. It calls the existing session
  list route with encoded task/session filters, normalizes the response through
  `normalizeKubernetesSessions`, and returns the exact row or `null`.
- Extend `apps/web/hooks/domains/session/use-task-environment.ts` to accept the
  active `sessionId`, load the filtered Kubernetes row after the environment
  identifies `executor_type: "k8s"`, and expose `kubernetes`, a sanitized
  status error, and an explicit `refresh` action. Guard the chained responses
  by the current task/session generation so a late result cannot overwrite a
  newly selected session.
- Extend `resolveExecutorEnvironmentStatus` in
  `apps/web/components/task/executor-environment-status.ts` with an explicit
  Kubernetes input. `container_state`/`pod_phase` and `failure_reason` take
  precedence over the recorded environment state. A completed filtered lookup
  with no row or a lookup failure is unavailable/error, never `ready`.

### Pod details and controls

- Extend `apps/web/components/task/executor-environment-info.tsx` so the `k8s`
  branch renders Pod name, phase/container state, restart count, workspace
  mode, created time, and sanitized failure reason. Reuse the existing
  `executors:*` Kubernetes labels/status/workspace translations.
- Extract the disclosure body/actions from
  `apps/web/components/task/executor-settings-button.tsx` into a focused shared
  component if needed to keep both files below the web subtree limits. The
  action row contains Refresh and a link built with
  `kubernetesExecutorSettingsPath(env.executor_id)`. The generic Reset
  environment action stays available to its existing executor types but is not
  rendered for Kubernetes, where it cannot own exact Pod/PVC cleanup.
- On fine pointers, preserve the compact task-topbar trigger and desktop
  popover. On coarse pointers, use `useTouchDrawer` and a shadcn `Drawer` with
  the same status/details/actions, an accessible title/description, safe-area
  padding, internal overflow, and a visible named trigger with a 44 px hit
  target.
- In `apps/web/components/task/mobile/session-mobile-top-bar.tsx`, render that
  Kubernetes disclosure button instead of the tooltip-only
  `RemoteCloudTooltip`. Other remote-executor indicators remain unchanged.

### Pod icon

- Change the central `k8s` mapping and status mapping in
  `apps/web/lib/executor-icons.ts` from `IconCloudComputing`/`IconCloudOff` to
  `IconCube`/`IconCubeOff`, retaining the Kubernetes test ID.
- Make the Kubernetes create header, executor header, and profile connection
  action consume the same central mapping instead of importing
  `IconCloudComputing` directly:
  `apps/web/app/settings/executors/new/[type]/kubernetes-create-page.tsx`,
  `apps/web/app/settings/executors/k8s/[executorId]/page.tsx`, and
  `apps/web/components/settings/profile-edit/profile-connection-settings-action.tsx`.
- Add only genuinely new copy to all six locale catalogs and regenerate the
  Traditional Chinese and pseudo catalogs with the repository scripts.

---

## Tests

- **What:** filtered status reads touch only the requested authoritative
  task/session row, still enforce task authorization and Pod UID/label checks,
  and reject a session-only filter.
  **File:** `apps/backend/internal/kubernetes/sessions_test.go` and
  `apps/backend/internal/kubernetes/handlers_test.go`.
  **How:** fake repository/client reactors that fail if an unrelated Pod GET or
  authorization occurs.
- **What:** the client encodes the executor/task/session IDs and returns zero or
  one normalized Kubernetes session.
  **File:** `apps/web/lib/api/domains/kubernetes-api.test.ts`.
  **How:** mocked HTTP client assertions for matching, missing, and malformed
  response rows.
- **What:** Kubernetes running/pending/failed/missing/error states override the
  recorded `TaskEnvironment.status`, while Docker behavior is unchanged.
  **File:** `apps/web/components/task/executor-environment-status.test.ts`.
  **How:** table-driven pure-function tests, including the reported
  `environment.ready + no Kubernetes row` regression.
- **What:** Pod fields replace “No resource details available,” Refresh and
  Executor settings are visible, Kubernetes does not show Reset environment,
  and the cube icon is used in normal/error states.
  **File:** `apps/web/components/task/executor-environment-info.test.tsx`,
  `apps/web/components/task/executor-settings-button.test.tsx`, and
  `apps/web/lib/executor-icons.test.ts`.
  **How:** component tests with a running Kubernetes row and a failed row.
- **What:** a coarse-pointer Kubernetes trigger opens a Drawer containing the
  same details/actions and is reachable by accessible name.
  **File:** `apps/web/components/task/executor-settings-button.test.tsx` and a
  focused mobile-topbar component test beside
  `apps/web/components/task/mobile/session-mobile-top-bar.tsx`.
  **How:** mock `useTouchDrawer()` to `true`, tap the trigger, and assert Drawer
  semantics plus action target sizes.

---

## E2E Tests

- **Scenario:** GIVEN a real running Kind-backed task, WHEN the desktop
  executor disclosure opens, THEN it shows the exact Pod name, live Running
  state, zero restarts, workspace mode, Pod glyph, Refresh, and canonical
  executor-settings link without the generic empty-resource text.
  **File:** `apps/web/e2e/tests/kubernetes/kubernetes-executor.spec.ts`.
  **What to verify:** extend the existing kubeconfig launch case after it has
  the exact created Pod identity; do not add a thirteenth Kind lifecycle case.
- **Scenario:** GIVEN a Kubernetes task at the `mobile-chrome` coarse-pointer
  viewport, WHEN the executor control is tapped, THEN a Drawer exposes the same
  Pod status/actions with no hover dependency or document overflow.
  **File:**
  `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`.
  **What to verify:** use isolated backend fixture data and intercept only the
  filtered read-only session-status response; no real cluster is required.

---

## Verification Results

- Backend exact-filter tests and the full `internal/kubernetes` package passed.
- Frontend focused verification passed 7 files and 36 tests; full lint,
  typecheck, locale generation, `i18n:check`, and `i18n:ratchet` passed.
- The fresh mobile E2E passed 1/1 and the focused real Kind E2E passed 1/1,
  both on their first attempt with retries disabled. Task 03 records timings and
  teardown evidence.
- Public-document validation and `git diff --check` passed after the task-header
  disclosure documentation was synchronized.

---

## Implementation Order

1. [x] [task-01-exact-session-status](task-01-exact-session-status.md)
2. [x] [task-02-frontend-runtime-disclosure](task-02-frontend-runtime-disclosure.md)
3. [x] [task-03-desktop-mobile-e2e](task-03-desktop-mobile-e2e.md)

All tasks are sequential. The frontend consumes the backend filter contract,
and E2E follows the completed UI. No subagent delegation is implied.

## Risks And Considerations

- The settings session list can cover many Pods; task chrome must always use
  the new pre-Pod-lookup filter and must not poll the unfiltered list every
  three seconds.
- A selected task/session can change while HTTP is in flight. Generation
  guarding is required to avoid showing the prior Pod under the next task.
- A missing row is not evidence that a Pod is absent; it can also mean retained
  inventory is unavailable. The UI should say unavailable and preserve the
  sanitized error instead of inventing a cluster state.
- The existing generic Reset environment endpoint does not own Kubernetes
  inventory cleanup. Hiding it for Kubernetes prevents misleading destructive
  UI while leaving the accepted lifecycle authority unchanged.
- The seeded Tailscale/Kind lab is user-requested and remains live during this
  design checkpoint. E2E teardown may remove only its own fixture cluster, not
  that lab or the main Kandev process on port 9998.
