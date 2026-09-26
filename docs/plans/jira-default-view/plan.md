---
created: 2026-09-24
status: implemented
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
system_design:
  - ../../specs/integrations/system-design/jira-default-view.md
legacy_specs: []
---

# Implementation Plan: Jira Default View

## Overview

Issue [#3921](https://github.com/kdlbs/kandev/issues/3921) reports that every `/jira` visit starts on **Assigned to me** even when a user always works from the same saved custom-JQL view. `useFilterState` initializes `activeViewId` to `builtin:assigned`, while `useSavedViews` hydrates saved views later without selecting one. Add a per-user default ID, resolve it before the first ticket search, and expose set/clear controls in the existing view picker. Implement the settings contract first, then selection and persistence, then the rendered controls and end-to-end proof.

## Scope

### In scope

- One user-owned default that may identify a built-in or custom Jira view.
- Exact saved-filter and custom-JQL restoration, missing-view fallback, and unchanged no-default behavior.
- Desktop and phone picker actions, translations, public documentation, and focused regression coverage.

### Out of scope

- Active-view URL parameters, bookmarks, and sharing.
- Last-selected-view persistence or defaults for other providers.
- Changing Jira workspace configuration or saved-view query semantics.

## Technical approach

1. Add scalar `jira_default_view_id` to the existing user-settings GET/PATCH path: `apps/backend/internal/user/models/models.go`, `dto/dto.go`, `controller/controller.go`, `service/service.go`, `store/sqlite.go`, and `internal/settingscatalog/defaults.go`. Regenerate `apps/web/lib/settings-discovery/contract.generated.json`. Add the web transport field in `apps/web/lib/types/http-user-settings.ts`. Empty string clears the choice; an omitted field preserves it.
2. Extend `apps/web/components/jira/my-jira/use-saved-views.ts` to hydrate the default and views together, expose readiness and an acknowledged set/clear operation, and clear the default in the same PATCH as default-view deletion. Add a pure resolver for an available default and the legacy **Assigned to me** fallback. Use `apps/web/app/jira/jira-page-client.tsx` to apply the resolved view once, protect manual interaction during hydration, restore custom JQL, and gate `useJiraSearch` until initial selection is known.
3. Add marker and sibling set/clear action to `apps/web/components/jira/my-jira/list-toolbar.tsx`. The selected view and default view are distinct states. Reuse the existing picker and deletion confirmation. Localize new copy in all Jira catalogs and document the user workflow in `docs/public/integrations.md`.

## ASCII UI preview

`UI-01` is the existing `/jira` saved-view picker. The primary row action selects a view; the star action changes the future default. The drawing shows control order and states, not pixel spacing or final icon styling.

Current desktop and phone picker (source: `ListToolbar`):

```text
Views: Assigned to me
  Built in
    [check] Assigned to me
            Unassigned
  Saved
            My open tickets             [Delete]
```

Proposed desktop picker, after **My open tickets** is marked default:

```text
Views: Assigned to me
  Built in
    [check] Assigned to me             [Set default]
            Unassigned                 [Set default]
  Saved
            My open tickets       [star: default] [Delete]
```

Proposed phone picker, using the current inset picker surface:

```text
Views: Assigned to me  [tap]
  Built in
    [check] Assigned to me       [star, 44px]
            Unassigned           [star, 44px]
  Saved
            My open tickets      [star, 44px] [Delete, 44px]
  (one vertical scroll region; actions stay inside the viewport)
```

The visible star identifies the default; its accessible name says **Set ... as default view** or **Clear ... as default view**. A default action leaves the current view unchanged. The phone surface keeps a visible touch target for each action and does not rely on hover. This preview covers `AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.5` and `.6`.

## Tests

| Acceptance criteria    | Focused evidence                                                                                                                                                   |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `.1`, `.7`             | Backend GET/PATCH and SQLite round-trip tests; `use-saved-views.test.ts` acknowledged writes, replacement, clear, and failure tests.                               |
| `.2`, `.3`, `.4`, `.5` | New pure resolver and `jira-page-client` state tests, including exact custom JQL, no-default project key, deleted view, and late hydration after manual selection. |
| `.6`                   | `list-toolbar.test.tsx` selection isolation, accessible names, and phone hit-target assertions.                                                                    |

## E2E tests

- `apps/web/e2e/tests/integrations/jira-default-view.spec.ts` (desktop): mark a custom JQL view as default, revisit `/jira`, inspect the query and results, delete it, and verify the next visit falls back to **Assigned to me**. Covers `.1` through `.4`.
- `apps/web/e2e/tests/integrations/mobile-jira-default-view.spec.ts` (`mobile-chrome`): set and clear a default in the phone picker, revisit the page, and verify touch reachability and no horizontal overflow. Covers `.1`, `.2`, `.5`, and `.6`.

## Work orders

- [x] [Task 01: Persist the Jira default preference](task-01-persist-jira-default.md)
- [x] [Task 02: Restore the default Jira view](task-02-restore-jira-default.md)
- [x] [Task 03: Add picker controls and prove the flow](task-03-jira-default-controls.md)

Work orders run sequentially in the primary session. Planning a dependency wave does not authorize delegation.

## Verification results

All three work orders are complete. The user settings contract persists a per-user default view ID. `/jira` restores an available built-in or custom view, including exact custom JQL, before its displayed ticket search, and retains a manual choice made during hydration. The picker provides localized set/clear actions on desktop and phone. Deleting the active default clears it and returns the current and next visit to **Assigned to me**. Status reconciliation runs only after authoritative metadata arrives and only for structured filters; a failed status lookup preserves saved statuses and custom JQL. The results remain loading until initial selection resolves. Saved-view list mutations serialize, new views publish only after an acknowledged settings write, and a later user selection takes precedence over a pending save or deletion. The desktop picker keeps its active default star visible without hover. Public Jira documentation explains how to choose or clear a default.

Focused backend tests pass. The six-file Jira web suite passes 47 tests. Web typecheck, ESLint, i18n, and E2E sleep checks pass. The rebuilt desktop and Pixel 5 Playwright scenarios each pass. The production web build and both public documentation validators pass.

## Risks

- Settings hydrate after the page mounts. Initial search must wait for the resolved target, and late responses must not overwrite a user action.
- Saved views and the default ID share one user-settings document. Deleting the default must clear both fields in one PATCH; failed writes must retain the prior state.
- A custom JQL may refer to projects unavailable in another workspace. The page should preserve the saved query and show the existing Jira search error rather than silently rewrite it.
- The picker is narrow on phones; star and delete targets must remain reachable without horizontal scrolling.

## CI retry follow-up

PR #3924 merged at 2026-09-25 14:00 UTC as `1b9c2de146d279be0702f3e6799832487a1fe819`. Its E2E retry report contained 11 tests that passed only after a retry. The follow-up remediation fixes Quick Chat's missing persisted model/config replay after a backend restart and correlates the taskless-routine test's `RoutineRun` and live Office run through their shared causation ID. It also fixes the mobile PR selector's overlap and close/reopen race, scopes recovery-proxy response drops to the intended session, waits for screenshot/file-tree resources, and checks unrounded mobile hit-target dimensions after finite menu animations.

On the latest `main`, the session-entry recovery, mobile PR re-request review, and mobile saved-view scenarios passed 9 runs with retries disabled. With the causation-ID correlation in place, taskless routine passed 10/10, mobile PR review passed 6/6, mobile saved views passed 6/6, mobile session-entry recovery passed 3/3, Quick Chat restart recovery passed 3/3, completed-workspace restoration passed 2/2, and preview feedback passed 3/3. Backend run-list DTO and session-model replay tests, web typecheck, focused ESLint, and `git diff --check` pass. The corrected PR head's remote CI remains the final shared-runner verification.

## PR #3936 CI retry remediation

The PR #3936 retry artifact reported nine retry-only E2E failures. Fixes now wait for cancellation and file-tree WebSocket responses, wait for the page and chat to finish loading after reload, wait for a persisted response before checking mobile rendering, select repositories by ID, navigate to the created terminal task directly, and allow subpixel rounding at the 44px touch-target boundary. Worktree cleanup now recovers the branch commit only when a checkout disappears during `git rev-parse`; a regression test reproduces the failed inspection race.

Focused no-retry repetitions pass for the changed desktop and mobile cases: 10 desktop runs and 8 mobile runs. The worktree, task-service, and backend-app Go packages pass; web typecheck, ESLint, the E2E sleep ratchet, and `git diff --check` pass. The updated PR head's CI and retry artifact remain the final shared-runner verification.

## PR #3936 follow-up after exact-head CI

The blob audit for PR head `97c0d15d65222d87ec2d7dd82fa300d850754288` found ten retry-only scenarios and a file-tree test that failed on all three attempts. The follow-up arms file-tree response waits before navigation, avoids clicking a Kanban card while its live updates can detach it, gives the Office run test an isolated agent, waits for the preview iframe's screenshot mode before dragging, and uses a deterministic agent prompt for the lifecycle test. It also restores focus to the surviving PR chip after unlink, reuses a just-completed workflow preview across immediate surface changes, waits for the selected prompt state in the passthrough test, and applies the shared subpixel-safe touch-size assertion.

No-retry local verification against the rebuilt E2E bundle passed: the five other desktop scenarios passed three times each; the office error scenario passed ten times with `CI=true`, `GITHUB_ACTIONS=true`, and IPv4-first DNS; the mobile menu and PR unlink scenarios passed three times each; passthrough prompt selection passed ten times; the large file tree passed three times; and the mobile workflow preview passed five times. The focused PR-chip and workflow-preview unit suites passed 59 tests. `pnpm run build:e2e`, web typecheck, focused ESLint, and `git diff --check` pass. The `fetch failed` in the original office error attempt was not reproducible in ten CI-environment runs. Exact-head PR checks and the explicit blob retry audit remain pending for the pushed follow-up.

## PR #3936 retry follow-up after e767f4d

The exact-head E2E artifact for `e767f4d698e0f88bbefdb6aa49b91ce14710088e` contained six retry-only scenarios. Their causes were mutable shared workflow data, a Kanban card detaching during live updates, a workspace tree read before its file fixture was ready, a taskless routine firing while its shared CEO was stopped, a worktree disappearing during Git status inspection, and fake-timer teardown leaving preview cleanup pending. Office-specific reset and settings calls also now use the existing backend transport-recovery wrapper. The same head's frontend unit job exposed preview-cache cleanup timers not being drained in fake-timer teardown. Kubernetes compatibility separately failed while downloading external tools after repeated HTTP 500 responses; fresh CI must confirm whether that infrastructure failure clears.

After those corrections, the five Chromium retry cases pass three repetitions each (15 passes), and the mobile changes-panel case passes three repetitions. The rebuilt backend and E2E web bundle, worktree/task-service/backend-app Go packages, workflow preview unit test (26 tests), web typecheck, focused ESLint, E2E sleep ratchet, and whitespace check pass. Exact-head CI and a zero-retry blob audit remain pending for the pushed fixup.

The PR #3936 run for `4059bcef607b7da9050de8cfd4c7e2a111999288` reported four retry-only tests and one compact-stepper test that failed all attempts. The latest fixup makes the compact fixture select its intended layout, waits for ready fixture files and a fresh tree response, associates the Docker slow-bootstrap task with its profile at creation, and retains the mobile comment action across menu close. Local retry-free repetitions pass for the stepper, symlink, rename, mobile comment, and Docker cases; the review toolbar unit suite passes 12 tests. Exact-head CI and the zero-retry blob audit are pending.

## PR #3936 follow-up after 42886d6

The exact-head CI for `42886d63340ab5823c8e1002d50f302e83292d93` exposed four retry-only scenarios and two rename failures. Their causes were unpushed fixture commits, file-tree reads before exact worktree materialization, one missed setup-helper migration, a workflow import race with a redundant navigation, and a PR watcher attached after the socket opened. The fixtures now publish to `main`, wait for ready files and a fresh tree response, use the updated rename setup shape, stay on the import page, and subscribe to the poller’s exact task-PR update before navigation. All seven selected E2E cases pass three repetitions without retries (21 total). Web typecheck, focused ESLint, E2E sleep ratchet, bundle build, and whitespace checks pass. The next exact-head CI run and blob audit will validate the pushed revision.
