---
spec: docs/specs/kubernetes-executor/spec.md
created: 2026-08-31
status: complete
---

# Implementation Plan: Kubernetes Executor UX Repair

## Overview

The shipped Kubernetes controls are functionally connected, but four concrete
frontend choices make them feel incomplete: `useLiveEnvironment.loadEnv` hides
every loading state after the first successful read and drops a refresh while
the same scope is polling; the task disclosure renders a dense ungrouped
definition list; `ProfileEditSections` puts cluster diagnostics after workload
and omits `KubernetesSessionsCard`; and the PodTemplate textarea relies on CSS
`field-sizing` behind a fixed `min-h-80` floor. The repair keeps the existing
Kubernetes API and persistence contracts, then fixes the task disclosure, makes
cluster operations the first profile section, and adds responsive regression
coverage.

| Before                                                                                                  | After                                                                                                                             |
| ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| A flat, tightly packed Pod definition list with weak action hierarchy                                   | A structured Pod summary with a clear identity header, grouped facts, and visually distinct Refresh and Executor settings actions |
| Manual Refresh has no visible state and can be discarded by polling                                     | Manual Refresh is visibly busy and joins the exact in-flight scoped read without duplicating it                                   |
| Profile pages begin with generic profile/workload fields; diagnostics are later and sessions are absent | Cluster status, test, executor-wide sessions, and Edit/View connection lead every Kubernetes profile page                         |
| Raw YAML keeps a 320 px floor and depends on CSS-only autosizing                                        | Raw YAML grows and shrinks from a compact floor with a WebKit-safe measurement fallback and no vertical inner scroll              |

---

## Confirmed Root Cause

- `apps/web/hooks/domains/session/use-task-environment.ts` stores only a
  `Set<string>` of in-flight scopes. `loadEnv` returns immediately for a
  duplicate scope, so a click cannot await the poll already in progress.
  `setLoading` also becomes true only for `active && !hasLoadedRef.current`, so
  the Refresh button never enters its spinner/disabled state after the first
  successful load.
- `apps/web/components/task/executor-environment-info.tsx` applies the same
  12 px definition-list treatment and monospace value styling to identity,
  state, dates, and counts. `executor-environment-disclosure.tsx` adds two small
  controls in a hairline footer, leaving no strong identity or action hierarchy.
- `apps/web/app/settings/executors/[profileId]/page.tsx` renders
  `ProfileDetailsCard` before executor-specific sections.
  `KubernetesProfileSections` orders Workload, Workspace, then Diagnostics and
  never mounts `useKubernetesSessions`; the session card exists only on
  `/settings/executors/k8s/:executorId` behind the header connection action.
- `apps/web/components/settings/kubernetes-workload-card.tsx` overrides the
  shared textarea with `min-h-80 resize-y`. Chromium's shared
  `field-sizing-content` happens to expand content, but the fixed floor prevents
  compact shrinkage and WebKit has no measurement fallback.

The smallest live reproduction is the seeded Kubernetes task/profile: opening
the desktop task control shows the cramped list; clicking Refresh sends the two
GETs without changing the control; and opening the profile directly shows
Profile Details and Workload before diagnostics with no Active sessions card.
At 393 px, the same page puts a full-width Cluster connection button above those
fields, but the health information is still below the fold or absent.

---

## Backend

No backend, API, model, database, RBAC, or lifecycle change is required. The
repair continues to use:

- `GET /api/v1/tasks/:id/environment/live` plus the exact filtered Kubernetes
  session read for task chrome;
- `POST /api/v1/kubernetes/test` with persisted executor connection config and
  the profile's current unsaved workload config; and
- `GET /api/v1/kubernetes/executors/:id/sessions` for the existing sanitized,
  executor-wide active-session inventory.

---

## Frontend

### Task disclosure refresh contract

- In `apps/web/hooks/domains/session/use-task-environment.ts`, replace the
  scope-only in-flight marker with an awaitable per-scope request entry. Keep
  automatic poll reads deduplicated, and expose a separate manual `refreshing`
  state so initial loading and an explicit user action have distinct semantics.
  A manual refresh that arrives during a poll awaits the existing promise; it
  does not start another environment/session pair.
- Preserve last-known Pod facts while refreshing and keep the existing
  request-scope generation check so a late response cannot publish under a new
  task/session.
- In `executor-environment-disclosure.tsx`, bind the Refresh control to
  `refreshing`, including an immediate disabled state, spinner, and accessible
  busy label. Background polling alone must not animate the manual action.

### Task disclosure visual hierarchy

- Refine the Kubernetes branch of
  `apps/web/components/task/executor-environment-info.tsx` into a Pod-specific
  summary: Pod glyph and executor label, live status chip, exact Pod identity
  with a usable copy target, then grouped phase/container, restart/workspace,
  created, and failure facts. Restrict monospace typography to technical
  identities and raw enum values where it improves scanning.
- In `apps/web/components/task/executor-settings-button.tsx`, retain the
  fine-pointer hover/focus surface and coarse-pointer Drawer, but give the
  content a deliberate width, padding, border/shadow, bounded desktop height,
  and consistent mobile scroll ownership. Do not add Pod restart/delete/reset
  controls.
- In `executor-environment-disclosure.tsx`, make Refresh a clear secondary
  action and Executor settings the primary destination. The same content and
  controls render in the touch Drawer with 44 px action targets.

### First-class profile cluster status

- Add
  `apps/web/components/settings/kubernetes-profile-cluster-section.tsx` as the
  profile-level composition owner. It receives the shared `Executor`, current
  `KubernetesProfileConfigForm`, and permission state; renders the executor
  identity/namespace as cluster context; runs `useKubernetesDiagnostics` with
  `executor.config` plus `serializeKubernetesProfileConfig(form)`; and mounts
  `useKubernetesSessions(executor.id)` immediately.
- Compose the existing `KubernetesDiagnosticsCard` and
  `KubernetesSessionsCard` inside that leading section. State explicitly in
  visible copy that sessions are executor-wide. Administrators see Edit cluster
  connection; members see View cluster connection, the disabled test with its
  existing explanation, and readable sanitized sessions.
- In `apps/web/app/settings/executors/[profileId]/page.tsx`, render that section
  after the page header/read-only notice and before the profile editing
  fieldset. Remove the Kubernetes connection action from the generic page
  header so the contextual action has one canonical location.
- Reduce `KubernetesProfileSections` to the mutable Workload and Workspace
  cards; it no longer owns a second diagnostic hook/result.

### PodTemplate content sizing

- In `apps/web/components/settings/kubernetes-workload-card.tsx`, measure the
  controlled textarea after each YAML value change: reset its inline height,
  then set the measured `scrollHeight`. Force fixed field sizing for consistent
  cross-browser behavior, keep a compact minimum, disable manual vertical
  resize, and hide vertical overflow while retaining `wrap="off"` plus
  contained horizontal scrolling.
- Resize after initial hydration, typing, programmatic profile replacement, and
  settings discard so the field can shrink as well as grow. Measure once per
  committed value; do not add an observer or per-frame loop.

### Localization

- Add only the cluster-section and Edit/View connection copy to
  `apps/web/src/locales/*/executors.json`. Reuse existing task, Kubernetes fact,
  Refresh, diagnostics, and session strings where possible. Generate
  Traditional Chinese and pseudo catalogs with the repository scripts.

---

## Tests

- **What:** explicit refresh becomes visibly busy, awaits an existing scoped
  poll request, publishes the refreshed Pod row, and does not duplicate the
  HTTP pair. **File:**
  `apps/web/hooks/domains/session/use-task-environment.test.tsx` and
  `apps/web/components/task/executor-settings-button.test.tsx`. **How:** deferred
  promises for the environment and exact Kubernetes session reads, including a
  click while the poll promise is unresolved.
- **What:** Kubernetes renders the new identity/status/fact hierarchy and
  action priority while Docker/SSH fallback behavior and the coarse-pointer
  Drawer remain intact. **File:**
  `apps/web/components/task/executor-environment-info.test.tsx` and
  `apps/web/components/task/executor-settings-button.test.tsx`. **How:** focused
  component assertions for semantic headings, copy target, fact grouping,
  refresh busy state, settings link, and 44 px mobile classes.
- **What:** a Kubernetes profile mounts diagnostics and executor-wide sessions
  before mutable profile cards, tests the current unsaved profile config, and
  exposes the permission-appropriate connection action. **File:** new
  `apps/web/components/settings/kubernetes-profile-cluster-section.test.tsx`
  plus
  `apps/web/components/settings/profile-edit/profile-edit-page-chrome.test.tsx`.
  **How:** mocked diagnostics/session APIs and router assertions for admin and
  member states.
- **What:** PodTemplate YAML grows and shrinks on initial, typed, and reset
  values without vertical overflow. **File:** new
  `apps/web/components/settings/kubernetes-workload-card.test.tsx`. **How:**
  controlled rerenders with mocked `scrollHeight`, asserting inline height,
  fixed field sizing, and horizontal-only overflow classes.

---

## E2E Tests

- **Scenario:** GIVEN a running Kubernetes task, WHEN the desktop disclosure
  opens and Refresh is selected, THEN the structured Pod summary remains
  visible, Refresh reports busy until the causal read settles, the returned Pod
  facts update, and Executor settings remains the primary destination. **File:**
  new `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts`.
  **What to verify:** intercept the exact task/session status read with a
  deferred response; assert busy state and no duplicate request before release.
- **Scenario:** GIVEN the same task at the Pixel 5 coarse-pointer viewport,
  WHEN the executor control is tapped, THEN the Drawer exposes the same hierarchy
  and refresh behavior with 44 px controls and no horizontal overflow. **File:**
  `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`.
- **Scenario:** GIVEN a Kubernetes profile opened directly from the settings
  tree, WHEN the page renders, THEN cluster diagnostics and executor-wide Active
  sessions precede Profile Details/Workload, Edit cluster connection routes to
  the canonical executor page, and diagnostics submits the current profile
  draft. **File:**
  `apps/web/e2e/tests/settings/kubernetes-executor.spec.ts`.
- **Scenario:** GIVEN short and long PodTemplate YAML on a phone, WHEN the value
  is edited, THEN the textarea shrinks/grows, has no vertical inner scrollbar,
  its long lines remain horizontally contained, all cluster controls are
  touch-visible, and the document does not overflow. **File:**
  `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`.

---

## Verification Results

- Focused Vitest: 8 files and 38 tests passed across the task disclosure,
  request deduplication, icon mapping, leading profile section, YAML sizing,
  and existing profile/settings contracts.
- Playwright with the managed isolated runner and retries disabled: desktop
  Chromium discovered and passed 3/3 tests in 21.6 seconds; Pixel 5
  `mobile-chrome` discovered and passed 4/4 tests in 22.4 seconds. The runs used
  random worker ports and did not use a real Kubernetes cluster.
- `pnpm run build:e2e` completed in 8.10 seconds. Focused ESLint, TypeScript,
  `i18n:check`, `i18n:ratchet`, `e2e:sleep-ratchet`, and `git diff --check`
  passed. All six locale catalogs remain synchronized.
- Public Kubernetes/executor docs now describe the Pod control, first-class
  profile cluster status, current-draft diagnostics, and content-sized YAML.
  The public-doc suite passed 61/61 tests and validated all 41 published pages.
- The runners left no current-run backend, Vite, Playwright, or worker-port
  listener. The pre-existing isolated seed instance on ports 47646/48646 and
  the main Kandev instance on `:9998` were preserved and untouched.

---

## Implementation Order

1. [x] [task-01-task-disclosure](task-01-task-disclosure.md)
2. [x] [task-02-profile-status-hierarchy](task-02-profile-status-hierarchy.md)
3. [x] [task-03-responsive-e2e](task-03-responsive-e2e.md)

All tasks are sequential by default. Task 02 depends on the settled disclosure
interaction language, and Task 03 validates the integrated desktop/mobile
surfaces. The list does not authorize subagent delegation.

## Risks And Considerations

- Polling and explicit refresh share one request scope. The awaitable dedupe
  must not reintroduce the tight polling loop that `hasLoadedRef` currently
  prevents, and scope changes must still invalidate late publications.
- The profile session list is intentionally executor-wide, not profile-scoped.
  Its visible description must prevent users from assuming every row was
  launched from the open profile.
- A profile test uses current unsaved workload values but persisted executor
  connection values. The section must not imply that editing the profile also
  edits kubeconfig, context, namespace, or auth mode.
- The PodTemplate limit is 256 KiB. Full content height can make the document
  long, but that is the requested single-scroll-owner behavior; the resize path
  should perform one layout measurement per value rather than continuous work.
- Desktop remains hover/focus capable and mobile remains a coarse-pointer
  Drawer. Styling changes must preserve keyboard access, focus, escape/outside
  dismissal, and the existing exact Pod identity trust boundary.
