---
created: 2026-09-10
status: done
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
legacy_specs: []
---

# Implementation Plan: Threads Home Default

## Overview

Add Threads to the existing portable Startup Page choice, honor it on bare
startup and Home navigation, and keep device-local listing memory independent.
Implement the settings contract first, then routing, then the localized choice
and desktop/mobile browser proof. All work orders are sequential.

This task's workspace is
`/home/zeval/.kandev/tasks/choose-threads-as-de_94whggoq/kandev-2`.
The initial worktree was clean. No reviewed package for a fixed Threads Home
default was present; the prior
[task-listing plan](../task-listing-display-preferences/plan.md) is completed
historical work. This package was handed off before implementation, which the
user authorized with the subsequent "go for impl" request.

## Sources and reconciliation

- [Owning requirements](../../specs/ui/requirements/task-listing-display-preferences.md)
  retain `REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001`, extract the already
  shipped startup behavior into `002`, and define the extension in `003`.
  UI owns the reusable presentation preference, not task or workspace state.
- [System design](../../specs/ui/system-design/task-listing-display-preferences.md)
  moves technical material out of the old requirement's migrated prose and
  defines precedence without duplicating the Threads deck contract.
- `view-preference.ts` and its tests already accept Threads. The requirement
  and public task-listing documentation still listed only three modes; that
  mismatch is reconciled in this package, not treated as missing deck support.
- At reconciliation, `StartupPage`, `NormalizeStartupPage`, `applyStartupPage`,
  the settings radio card, and frontend parsing accepted only overview/last task.
- At reconciliation, `workspaceHomeHref` and `homeDestinationHref` duplicated
  Home policy; `nav-home` additionally used a workspace-less overview override.
- [ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md),
  [ADR 0023](../../decisions/0023-active-workspace-cookie.md), and
  [Office mode](../../decisions/2026-08-15-office-mode-follows-active-workspace.md)
  retain settings and workspace authority. No new ADR is needed: this is a
  bounded extension whose rationale is preserved in the owning design.

## Scope

### In scope

- Portable `startup_page: "threads"`, preserving the existing default and
  patch/revision semantics across storage, HTTP, WS, and boot hydration.
- Fixed Threads Home overriding remembered mode only for generic Home and bare
  startup. An explicit route remains explicit.
- Desktop/sidebar, phone/menu, settings-exit, workspace-picker, and Home-command
  wiring through the shared destination policy.
- Startup settings copy, discovery aliases, all locales, and browser evidence.
- Public task-listing documentation when implementation ships.

### Out of scope

- Parent dirty worktree, its demo data, swipe-indicator timing, and mobile
  topbar normalization. A phone Home href change is wiring only.
- New Kandev tasks/sessions, agent launch, subagent delegation, commit, push,
  PR, or a new worktree as an execution mechanism.
- New per-device or per-workspace Home stores, schema migrations, feature
  flags, or fixed defaults for every other listing mode.
- Changing task Back/Task overview links, task lifecycle, Threads saved-view
  selection, deck filtering, or resource budgets.

## Technical approach

### 1. Portable startup choice

Extend `internal/user/models` and `internal/user/service` to retain/validate
`threads`. Verify the existing DTO, JSON settings store, revision-guarded
service, event, and `mapUserSettingsState` boot paths. Extend frontend
`StartupPage` and the common settings mapper; the same field already flows
through Appearance draft/save and the settings store. Do not add a column or
new default field. See [Task 01](task-01-persist-threads-startup-choice.md).

### 2. Home intent and entry routing

Centralize Home href policy in `lib/navigation/workspace-home.ts`; adapt the
manifest and direct callers. Carry the saved startup choice through
`NavContext`/`useNavContext`. Home in a non-Office workspace resolves directly
to Threads when selected. Preserve existing overview links for existing
choices; `nav-home`'s old overview override is bypassed only for the new choice.

Bare root resolution belongs to `startup-page.ts` and `PageClient`, using
existing explicit-destination guards and one redirect per resolution. Expose
workspace-scoped bootstrap readiness in `useKanbanRouteBootstrap` so settings
and workspace data settle before the default applies. Fix the local listing
hook's synthetic `loaded: true` write to preserve real readiness. Empty/error
bootstrap paths must settle without requiring board snapshots or workflows.
See [Task 02](task-02-resolve-threads-home-navigation.md).

### 3. Selectable settings and integrated proof

Add the third radio to `StartupPageSettingsCard`; reuse the existing Appearance
contributor and shared Save/discard/error flow. Revise descriptions and search
aliases in all locales to distinguish last-used overview, startup-only last
task, and fixed Threads Home. Existing native settings rows and Threads deck
provide mobile composition. See
[Task 03](task-03-expose-threads-home-choice.md).

Update the how-to section **Find and organize tasks** in
`docs/public/tasks-and-workflows.md` with supported device modes, the three
startup choices, and Home versus explicit-route behavior. Publication belongs
to implementation, not the preceding design-only turn.

## Tests

The following additions and extensions implement the acceptance coverage.
Each work order records its RED/GREEN results.

| Acceptance criteria | Evidence |
| --- | --- |
| `001.3` to `001.6`, `001.12` | `view-preference.test.ts` and `use-task-listing-view.test.tsx`: Threads, invalid/missing/legacy values, blocked writes, phone Pipeline fallback, startup choice unchanged by view changes |
| `002.1`, `002.5`, `003.3` | Backend `TestApplyStartupPage`, `TestScanUserSettingsStartupPage`, DTO/event/boot startup tests; new repository startup persistence tests for SQLite/PostgreSQL and separate users; frontend mapper/WS startup cases |
| `002.2` to `002.4` | `startup-page.test.ts`, `page-client.test.tsx`: workspace-local recent task, no matching task, explicit overview/task/workflow guards, no recent-task resume for Threads |
| `003.4`, `003.6` | New `lib/navigation/workspace-home.test.ts` Home matrix; existing navigation resolver, sidebar workspace-navigation, Home affordance, and app-destinations hook tests |
| `003.4`, `003.5`, `003.7` | `page-client.test.tsx`, new `src/kanban-route-startup.test.tsx`, and `view-navigation.test.ts`: competing remembered value, deferred bootstrap, stale workspace completion, explicit routes, empty workspace/deck, failed bootstrap recovery |
| `003.1`, `003.2` | New `appearance-settings-state.test.ts` startup draft/patch/rebase cases; existing general-settings/save-provider regressions and browser radio/save/discard/error flow |
| `003.8` | Phone E2E: labelled radio row at least 44px high, tap Save, Home to native deck, explicit route reload, no horizontal overflow |

The unchanged workflow-filter and rich-row criteria (`001.1`, `001.2`,
`001.7` to `001.11`) retain the completed prior plan's evidence. Existing
task-listing desktop/mobile specs run as regression neighbours because this
change touches their shared view navigation.

## E2E tests

Use isolated worker fixtures only. Capture settings before patches and restore
them in `afterEach`, acquiring `testPage` before changing persistent defaults.
Use causal save-response waits and final UI assertions; no fixed sleeps or
larger locator timeouts. Rebuild through the managed runner. Confirm each
project discovers its intended tests; desktop and mobile run separately.

Extend `e2e/tests/settings/startup-page.spec.ts` (`chromium`):

- **saves Threads as the Home default independently of the last listing**:
  choose and save Threads, reload Settings, use List, assert `/tasks` survives
  reload, then use Home and reload bare root to reach Threads in the same
  workspace. Clearing only the listing key cannot disable fixed Threads.
  Covers `003.1` to `003.5` and `003.7`.
- **keeps Threads draft changes pending until saved**: discard the selection;
  then fail one settings save, retain the prior backend choice and retryable
  draft, and retry successfully. Covers `003.2`.
- **keeps every Home entry in the selected workspace**: brand/Home row,
  settings exit, Home command, and workspace picker target the selected
  workspace. Include two non-Office workspaces with distinct task titles, then
  test the empty Threads state. Covers `003.4`, `003.6`, `003.7`.
- **preserves explicit destinations with a Threads Home default**: select
  Kanban/Pipeline/List, reload, use a workflow URL and explicit task/session
  route, and follow a focused Threads URL. Verify path, workspace, and focus
  remain intact; browser Back/Forward does not force Home. Covers `003.5`.
- Retain both existing last-task tests for `002.1` to `002.4`. Add a real
  second browser context or fresh browser session using the same user backend
  with no listing key to prove `003.3`; do not run the test fixture's settings
  reset in that second context.

Extend `e2e/tests/settings/mobile-startup-page.spec.ts` (`mobile-chrome`):

- **saves Threads from phone settings and returns Home after using List**:
  enter Settings through phone navigation, tap the labelled Threads row, Save,
  reload, switch to List through the drawer, and tap the mobile menu Home row.
  Verify workspace, native deck, and a subsequent reload. Covers `003.1` to
  `003.4`, `003.6`, `003.8`.
- **keeps explicit phone destinations with Threads selected**: List reload,
  task/Task overview links, and focused Threads link remain explicit; missing
  local memory still permits Threads on Home. Assert row hit area, Save
  visibility, safe-area clearance, and no horizontal overflow. Covers `003.5`,
  `003.7`, `003.8`; retain the existing last-task scenario.

Extend the Home-focused cases in `e2e/tests/office/sidebar-navigation.spec.ts`
and `mobile-office-navigation.spec.ts` using `startup_page: "threads"` to
prove Office Home still works from shared Settings surfaces (`003.6`). These
are normal `chromium` and `mobile-chrome` navigation tests, not the provider
`routing` project. Do not exercise provider-routing scenarios for this change.

## Work orders

- [x] [Task 01: Persist the Threads startup choice](task-01-persist-threads-startup-choice.md) (wave 1, done)
- [x] [Task 02: Resolve Threads Home navigation](task-02-resolve-threads-home-navigation.md) (wave 2, depends on 01, done)
- [x] [Task 03: Expose the Threads Home choice](task-03-expose-threads-home-choice.md) (wave 3, depends on 02, done)

## Verification results

PR fixup scope also includes preserving disabled-Office phone Home routing,
repairing the workspace-picker test fixture, direct fallback resolver coverage,
and aligning specification index labels and backend verification build tags.
Task 02 records the remediation sequence and completed local validation.
The later full frontend CI run exposed three more incomplete navigation test
fixtures. Their test-only repair and expanded desktop/phone Home assertions
are also recorded in Task 02; no additional product contract change is needed.
The current-main merge retained both UI design index entries and passed the
122-test navigation suite, full web lint/typecheck/i18n, tagged startup/boot
tests, and 21 focused desktop/phone browser scenarios. Broad frontend and
current-head remote CI/review results are tracked in the PR delivery evidence.
Later E2E CI exposed a startup-readiness regression when switching to All
Workflows. Task 02 owns recording the hydrated fast-path completion, regression
coverage for live filters and route changes, and focused browser verification.
The existing listing-availability contract is clarified before this repair.
The landed parent polish moved phone Home into the shared menu. Preserve its
header and swipe implementation unchanged, retain the Threads choice through
the shared navigation context, and update this package's phone entry-point
tests and current design. The mobile parked-session flake is a separate
test-profile cleanup defect, with no product contract change.
After this integration, 109 focused frontend tests and 73 desktop/phone E2E
tests passed, including every failed CI filter case and the profile cleanup
regression. Full web lint/typecheck/i18n and specification/harness/public-docs
validators passed. Tasks 02 and 03 record exact commands; current-head remote
CI and review remain pending until the remediation push is verified.

- `python3 scripts/lint-spec-files.test.py`: passed, 30 tests.
- `python3 scripts/lint-spec-files.py --all`: passed for all specification files.
- `git diff --check -- docs/specs docs/plans`: passed.
- Task 01: startup enum/persistence, boot mapping, SQL guard, and SQLite and
  PostgreSQL fresh/replay/upgrade conformance passed. Frontend mapping and WS:
  87 tests passed.
- Task 02: 148 routing/navigation tests passed, including delayed and cancelled
  bootstrap, Office, empty/error fallback, and explicit destinations.
- Implementation-checkpoint frontend run: 272 tests passed across 21 files, including
  blocked/malformed local storage and settings draft/rebase/save regressions.
- Task 03: 19 browser tests passed with strict WS checks and no retries:
  desktop startup/listing 8, phone startup/listing 5, desktop Office Home 3,
  and phone Office navigation 3. Native phone screenshots were inspected;
  temporary artifact paths are recorded in Task 03.
- Typecheck, focused ESLint, all locale checks, and the production build passed.
- Public docs validation passed for 46 pages and its validator test passed.
- PR review remediation: the E2E settings response now declares the optional
  workspace and workflow IDs used by cleanup. A targeted compiler check
  reproduced both unknown-to-string errors before the fix and passed afterward;
  two API-helper tests and focused ESLint passed. The helper change is test-only.
- PR routing remediation: 88 focused frontend tests and seven phone E2E tests
  passed against a rebuilt web bundle, with retries disabled. The new
  disabled-Office regression failed on the old href before the fix. Typecheck,
  focused ESLint, specification and public-docs validation passed. Backend
  startup, boot mapping, and SQLite conformance passed with `-tags fts5 -race`;
  PostgreSQL was not rerun for this frontend/documentation-only remediation.
- Requirements are active, system design is current, and all three work orders
  are done. The task-owned disposable PostgreSQL fixture was removed after
  successful persistence and upgrade checks.

Each work order records its exact implementation commands and results;
execution was authorized by the user's subsequent "go for impl" request.
Public documentation changes were completed in Task 03. At the implementation
checkpoint, no commit, push, PR, agent delegation, or parent-worktree edit had
been performed. The user subsequently authorized PR publication and fixup.

## Risks

- A single forgotten Home consumer or the palette override would make the
  preference appear inconsistent. Integration coverage must click those
  actual entry points, not only test the resolver.
- `home=overview` already means remembered overview and suppresses recent-task
  resume. Treating it as generic Home would break explicit view selection.
- Deferred settings hydration could navigate to remembered List before the
  portable Threads choice arrives. Empty workspaces must not deadlock the
  readiness gate, and Office must resolve before task-listing defaults.
- This is a per-user choice applied to the current workspace, not a new
  per-workspace default. Settings copy must make the distinction from local
  recent-task and listing memory clear.
- Old binaries normalize unknown startup values to overview; normal release
  upgrade preserves existing values, but downgrades do not understand Threads.
- PostgreSQL coverage requires a disposable test database via
  `KANDEV_TEST_POSTGRES_DSN`; record a skip as missing evidence, not success.
