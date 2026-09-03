---
spec: docs/specs/kubernetes-executor/spec.md
created: 2026-08-31
status: complete
---

# Implementation Plan: Kubernetes Executor Status and Settings Repair

## Overview

The isolated seed instance reproduces two different answers for the same live
Pod: `task.session.status` returns `remote_status_error: "... Unauthorized"`,
while `GET /api/v1/kubernetes/executors/:id/sessions` returns that exact Pod as
`Running`. The generic path reaches `KubernetesExecutor.RefreshRemoteInstance`,
whose `inspectKubernetesRefresh` reads through the launch-time
`kubernetesSession.runtime`; the exact Kubernetes endpoint creates a client from
the current executor configuration. `RemoteCloudTooltip` then caches the first
error for the lifetime of the session row. Separately, the first UX repair
deliberately kept `KubernetesConnectionCard` on a dedicated executor route,
made the profile page link to it, and rendered task actions as a prominent
footer. Those choices directly conflict with the updated product contract.

The repair first makes Kubernetes lifecycle inspection credential-fresh. It
then moves list/card status to the same exact task/session Pod API used by the
task page, compacts the task disclosure controls, and composes shared connection
plus profile configuration on one profile-root settings page. The persisted
`executors` and `executor_profiles` resources remain separate.

---

## Backend

### Refresh active Kubernetes clients before status and reconnect inspection

- Update
  `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_refresh.go`
  so `KubernetesExecutor.RefreshRemoteInstance` does not treat the runtime
  client captured at launch as permanent credential authority. Build a fresh
  `kubernetesRuntimeClient` from the recorded connection metadata before the
  exact Pod/PVC inspection, validate the same recorded namespace/name/UID and
  ownership labels, and publish the refreshed runtime only after that
  inspection succeeds.
- Preserve the existing session, loopback forward, agentctl client, nonce, and
  token when the fresh inspection proves no process or transport refresh is
  needed. If a restart or dead-forward refresh is needed, stage the replacement
  using the fresh runtime so future exec and port-forward operations use the
  repaired credentials.
- Keep authorization failures causal and sanitized. A fresh client that is
  still forbidden or unauthorized must remain an error; the repair must not
  convert a real RBAC failure into a healthy status or fall back to unrelated
  cluster resources.
- Reuse `kubernetesExecutorConfigFromMetadata`, `clientFactory`,
  `kubernetesCleanupInventory`, `verifyRecordedPod`, and
  `verifyKubernetesRecordedPVC`. Do not add a second Kubernetes client builder
  or weaken exact-inventory checks.

### Remote status orchestration

- Keep `Manager.pollOneRemoteStatus` as the owner of opportunistic refresh then
  `GetRemoteStatus`. Add a manager-level regression only if needed to prove that
  a successful fresh inspection replaces a previously cached unauthorized
  result for `GetRemoteStatusBySessionID`.
- Do not change the `task.session.status` wire shape or expose Kubernetes API
  errors beyond the existing sanitized `remote_status_error` field.

---

## Frontend

### Sidebar and Kanban Kubernetes status

- Extend `RemoteCloudTooltip` in
  `apps/web/components/task/remote-cloud-tooltip.tsx` with the exact executor ID.
  For `executorType === "k8s"`, call
  `getKubernetesTaskSession(executorId, taskId, sessionId)` and project its Pod
  name, phase/container state, creation time, and sanitized failure reason into
  the existing tooltip view model. Other remote executors continue to use
  `task.session.status`.
- Refetch when a hover reopens instead of using `fetchedSessionId` as a
  permanent cache key. Deduplicate only the currently in-flight request and
  discard responses for an obsolete task/session scope.
- Thread `primaryExecutorId` through `buildSidebarItem`, `TaskSwitcherItem`,
  `TaskSwitcherRow`, and `TaskItem`; Kanban cards already carry the field.
  A genuine exact-Pod failure remains destructive, while a repaired credential
  read clears the old error and restores the healthy Pod glyph/tone.

### Compact task disclosure controls

- Refactor
  `apps/web/components/task/executor-environment-disclosure.tsx` and
  `apps/web/components/task/executor-environment-info.tsx` so Kubernetes Refresh
  and Profile settings are quiet icon-only controls in the Pod summary header.
  Remove the sticky two-button footer and reduce the desktop popover width in
  `executor-settings-button.tsx` while retaining the existing summary content.
- Keep the existing single-flight `useTaskEnvironment.refresh` contract. The
  refresh icon must expose `aria-busy`, spin while awaiting the joined request,
  and remain clickable after the request settles.
- Resolve the settings destination with
  `executorProfileSettingsPath({ id: env.executor_id, type: env.executor_type },
  env.executor_profile_id)`. Keep a safe executor-level fallback only for
  malformed legacy rows with no profile identity.
- Preserve a 40 px desktop hit target and at least 44 px on coarse-pointer
  Drawer layouts, plus translated accessible names and tooltips.

### One Kubernetes profile settings hierarchy

- Replace the summary-and-link composition in
  `apps/web/components/settings/kubernetes-profile-cluster-section.tsx` with the
  inline `KubernetesConnectionCard`, followed by diagnostics and executor-wide
  sessions. Diagnostics serialize both the unsaved connection form and unsaved
  Kubernetes profile form.
- In `apps/web/app/settings/executors/[profileId]/page.tsx`, own connection form
  state alongside the existing profile form and register one Kubernetes page
  save contributor. It must:
  - validate both forms;
  - load active-session impact once and show one confirmation for the dirty
    connection/workload scopes;
  - save the executor connection before the profile when both are dirty;
  - update each baseline after its own successful call so a later partial
    failure leaves only the unsaved resource dirty; and
  - reset both forms and diagnostics from the shared floating save control.
- Reuse `useKubernetesExecutorResource`, `KubernetesConnectionCard`,
  `useKubernetesSessionImpact`, `serializeKubernetesExecutorConfig`, and the
  existing profile serializer. Extend
  `kubernetes-save-confirmation.ts` with combined copy rather than displaying
  two browser confirmations during one Save action.
- Keep members on the same page with disabled connection/profile controls,
  disabled diagnostics, and readable active sessions.

### Navigation and legacy recovery

- Make configured Kubernetes executor nodes in
  `settings-menu-branches.ts` expand to profiles instead of navigating to a
  second connection page. The exact profile rows remain the canonical settings
  destinations.
- Change the task popover gear to the exact profile path. Make
  `/settings/executors/k8s/:executorId` redirect to a deterministic profile when
  one exists. Retain the existing standalone page only for an orphan executor
  with no profiles so administrators can repair or delete it.
- Update `LegacyExecutorSettingsRoute` and route helpers/tests without changing
  non-Kubernetes executor routes.

---

## Tests

- **What:** a launch-time runtime client returns Unauthorized, a fresh client
  built from the same recorded connection metadata can inspect the exact Pod,
  and refresh/status becomes healthy without replacing the running agentctl
  transport.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_restart_test.go`.
  **How:** fake runtime factory returning stale then current resource clients;
  assert exact inventory verification, runtime publication, no new Pod, and no
  unnecessary forward/nonce rotation.
- **What:** a fresh client that remains unauthorized or returns a foreign Pod
  stays an error and never publishes the new runtime.
  **File:** the same lifecycle test file.
  **How:** table-driven negative cases.
- **What:** Kubernetes sidebar/Kanban status uses the exact session API and
  refetches after reopening, clearing a prior unauthorized result; other remote
  executors retain the WS request.
  **File:** `apps/web/components/task/remote-cloud-tooltip.test.tsx` plus mapping
  tests for sidebar/Kanban props.
  **How:** deferred API/WS mocks with close/reopen and stale-response cases.
- **What:** the task disclosure has compact icon controls, visible joined refresh
  progress, no action footer, and an exact profile-root link on desktop/mobile.
  **File:** `executor-settings-button.test.tsx` and
  `use-task-environment.test.tsx`.
  **How:** Testing Library interaction tests.
- **What:** the Kubernetes profile root renders editable/read-only shared
  connection fields, diagnostics, and sessions before profile sections; one
  save confirmation coordinates connection plus workload and represents a
  partial failure correctly.
  **File:** `kubernetes-profile-cluster-section.test.tsx`, a focused new combined
  save-contributor test, and `apps/web/src/settings-routes.test.tsx`.
  **How:** hook/component tests with ordered deferred mutations.
- **What:** Kubernetes tree/legacy routes resolve to profile roots while
  non-Kubernetes routing and orphan-executor recovery remain unchanged.
  **File:** `settings-menu-branches.test.ts`,
  `legacy-executor-settings-route.test.tsx`, and
  `executor-settings-routes` tests.

---

## E2E Tests

- **Scenario:** GIVEN a Kubernetes task in sidebar and Kanban chrome, WHEN the
  first hover response is unauthorized and a later exact Pod read succeeds,
  THEN reopening shows healthy Pod state without the red cached error.
  **File:**
  `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts` using a
  narrowly scoped WebSocket/API response rewrite helper.
- **Scenario:** GIVEN a running Kubernetes task, WHEN the desktop hover or mobile
  Drawer opens and Refresh/settings are selected, THEN the compact controls are
  accessible, Refresh visibly awaits the causal read, and settings opens
  `/settings/executors/:profileId`.
  **Files:** desktop and mobile Kubernetes task-environment specs.
- **Scenario:** GIVEN an administrator or member opens a Kubernetes profile,
  WHEN the page renders, THEN connection configuration, diagnostics, and active
  sessions are first-class sections on that one page; admin save/reset and
  member read-only behavior work without navigating to a connection subpage.
  **Files:** `kubernetes-executor.spec.ts` and
  `mobile-kubernetes-executor.spec.ts`.
- **Scenario:** GIVEN a legacy Kubernetes connection URL for an executor with a
  profile, WHEN it loads, THEN it redirects to that profile root. An executor
  without profiles still mounts the recovery editor.
  **File:** the desktop settings E2E spec.

---

## Public Documentation

- Update `docs/public/k8s.md` and `docs/public/executors.md` to describe the
  inline shared connection editor, one profile settings hierarchy, compact task
  controls, and profile-root navigation.
- Keep the warning that executor connection values are shared across profiles
  and that saved connection changes can affect active-session reconnects.

---

## Verification Results

Implementation is complete. Task-level records contain the RED/GREEN evidence
and focused command results. The final integrated gates passed with 14 focused
frontend files and 97 tests, 4/4 desktop Playwright scenarios, 4/4 mobile Chrome
Playwright scenarios, TypeScript typecheck, the E2E sleep ratchet, 61 public-doc
validator tests, 41 validated published pages, and a clean `git diff --check`.
Managed E2E processes and current-run temporary directories were torn down; the
main Kandev instance and the separately seeded test instance remained intact.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in the primary conversation. These tasks
share lifecycle and settings contracts, so none is marked parallel-safe.

Wave 1:
- [x] [Task 01: Refresh Kubernetes status credentials](task-01-refresh-status-credentials.md)

Wave 2:
- [x] [Task 02: Repair task chrome status and controls](task-02-task-chrome-status-controls.md)

Wave 3:
- [x] [Task 03: Unify Kubernetes profile settings](task-03-unify-profile-settings.md)

Wave 4:
- [x] [Task 04: Verify responsive flows and update docs](task-04-responsive-e2e-docs.md)

---

## Risks And Constraints

- A fresh credential client may point at a different cluster if an
  administrator replaces the kubeconfig target in place. Exact namespace,
  Pod name, UID, and all ownership labels remain mandatory before publication;
  mismatch fails closed.
- Executor connection and profile persistence are not transactional. The UI
  must accurately retain partial-save state rather than claiming all changes
  were rolled back.
- One executor can own several profiles. Inline copy and confirmation must make
  the shared blast radius explicit without duplicating a connection subpage.
- The orphan-executor recovery route remains intentionally exceptional so
  removing the normal subpage does not strand an executor with zero profiles.

## Out Of Scope

- Database migrations or merging `executors` with `executor_profiles`.
- New Kubernetes lifecycle controls, Pod deletion/restart actions, or changes to
  exact resource-ownership policy.
- Exposing kubeconfig contents, tokens, or unsanitized Kubernetes API errors.
