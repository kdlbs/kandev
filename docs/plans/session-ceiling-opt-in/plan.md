---
created: 2026-09-17
status: completed
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation Plan: Opt-in session ceiling

## Overview

Disable the instance ceiling by default and let administrators enable it in
Settings > Task Behavior. First deliver persisted configuration and live
admission updates. Then expose that working API through the shared Settings
form and verify desktop and phone flows. Finally add explicit queue-limit scope,
configuration links, and live WIP-update regression coverage. All three work
orders are sequential.

The [requirements](../../specs/agents/requirements/session-concurrency-ceiling.md)
and [design](../../specs/agents/system-design/session-concurrency-ceiling.md)
extend the existing agent-owned capability. Task queue ownership stays with
the task system. The user explicitly requested disabled defaults and a Settings
opt-in. The chosen implementation preserves explicit environment overrides and
uses the existing live Message Queue settings pattern.

## Scope

### In scope

- Disabled defaults on fresh and upgraded installs; no CPU-derived migration.
- Persisted enabled state and remembered maximum, with live save/apply.
- Existing environment override compatibility and visible locked state.
- Settings discovery, permissions, validation, all five locales, and mobile.
- Deferred launch recovery on disable/increase and disabled population failures.
- Workflow/global queue explanations and links to the correct Settings page.
- Immediate WIP application and reason changes when one gate releases another.
- Public documentation and focused admission/Settings regression checks.

### Out of scope

- Live changes or a restart of the user's current installation.
- Workflow WIP ordering changes, manual-start hard caps, or historical notice deletion.
- Release flags, YAML configuration, or per-workspace limits.
- PR creation, commits, or delegation without a separate user request.

## Technical approach

Task 01 adds `internal/system/sessioncapacity` over the existing settings store.
It resolves explicit environment > persisted setting > disabled, injects the
effective integer into `orchestrator.ServiceConfig`, and wires the new API in
`internal/system/system.go` and `internal/backendapp/main.go`. The API shape,
validation, and error codes are defined in the paired design.

Update `session_ceiling_config.go`, `session_ceiling.go`, and `service.go` so
the controller has no independent CPU/environment default. A mutex-protected
setter changes the existing controller without discarding reservations. Disabled
admission must succeed even if population lookup would fail. Keep the existing
single sweeper alive and signal it after disabling or increasing capacity.
Audit constructor tests that previously used environment variables directly.
Preserve explicit env setup in enabled ceiling integration/E2E tests.

Task 02 adds a Session capacity section to `task-behavior-settings.tsx`. Use
`useSettingsSaveContributor`, the existing save bar, `SettingsTarget`, discovery
catalog, typed settings API, and responsive controls. Save enabled state and
maximum together. A fixed suggested maximum of five has no admission effect
until enabled and saved. Display "No session limit" for the disabled state.

Task 03 extends task-owned queue presentation. `launch_queue.reason` identifies
global session-capacity waiting; WIP uses `queued_for_step_id` and its authorized
destination workflow. Do not combine those records or infer causes from prose.
Add internal links to `#setting-session-capacity` and the correct workspace's
workflow settings/card. Reuse `WorkflowStepUpdated` reconciliation for saved
WIP changes and assert that increasing or removing either limit wakes its queue.
The task-owned specs above define these outcomes; agent specs retain ownership
of global admission and settings.

Use [ADR 0018](../../decisions/0018-runtime-settings-overrides.md) for install
scope, admin authorization, and environment precedence. Use Message Queue's
live settings behavior rather than the restart-based feature-toggle registry.
This local extension does not require a separate ADR.

## ASCII UI preview

### UI-01: Task Behavior, Session capacity, desktop and phone

Entry: Settings > Task Behavior, after Message Queue. The field order is shared;
phone uses the full-width inline card in the existing Settings route.

```text
Desktop, default saved state
+-----------------------------------------------------------+
| Session capacity                                          |
| Applies to all workspaces.                                 |
| Limit automatic sessions                         [ OFF ]   |
| Current: No session limit                                  |
+-----------------------------------------------------------+

Desktop, enabled draft
+-----------------------------------------------------------+
| Session capacity                                          |
| Limit automatic sessions                          [ ON ]   |
| Maximum automatic sessions [ 5          ]                   |
| Current: No session limit (changes not saved)               |
| Manual starts can exceed this limit.                       |
| Workflow WIP limits are separate.                          |
+-----------------------------------------------------------+
                    [ Reset ] [ Save changes ]

Phone, enabled and saved
+-----------------------------------+
| < Settings       Task Behavior     |
| ... Message Queue ...              |
| Session capacity                  |
| Applies to all workspaces.         |
| Limit automatic sessions   [ ON ]  |
| Maximum automatic sessions        |
| [ 5                             ] |
| Current: 5 automatic sessions      |
| Manual starts can exceed this     |
| limit. Workflow WIP is separate.   |
+-----------------------------------+
```

Required structure: switch before maximum, scope visible, effective state
separate from unsaved draft, shared Save changes/Reset actions. Hide the numeric
field while off. On phones, the existing `settings-scroll-container` is the
single scroll owner. The shared floating save bar appears only for dirty state
and must not cover the final controls; preserve its safe-area behavior. No new
modal, drawer, or nested scroller. Dimensions and spacing are illustrative.
Phone/coarse-pointer inputs, switch hit area, and actions are at least 44px;
ordinary fine-pointer controls are 28px. The Message Queue card/mobile spec is
the nearest shipped exemplar. This maps to AC-002.1, .2, .6, and .7 below.

### UI-02: Changed error and managed states, both viewports

```text
Enabled draft: Maximum [ 0 ]
Enter a whole number from 1 to 2147483647.
[ Reset ] [ Save changes (disabled) ]

Environment override:
Limit automatic sessions [ ON, read-only ]
Maximum [ 8, read-only ]
Current: 8 automatic sessions
Managed by KANDEV_MAX_CONCURRENT_SESSIONS.

Load failure: Could not load session capacity. [ Retry ]
Save failure: Could not save changes. (draft retained)
```

Environment value zero shows an off/read-only switch and no numeric field.
Members see an admin-only explanation. Loading disables edits; Retry appears on
load failure. All messages are localized. These states use the same inline card
and map to AC-002.4 through .6. The numeric bounds are illustrative UI copy
subject to locale formatting; the design defines their exact validation.

### UI-03: Queue reason and configuration link, desktop and phone

```text
Global session-capacity queue
+-----------------------------------------------------+
| Automatic launch: Queued                            |
| Global session limit (all workspaces)               |
| Destination: Codex / 5.6 Luna Max                    |
| 5 of 5 sessions in use. Checked 11s ago.             |
| Queued since 26s ago. Automatic retry enabled.       |
| [Configure global session limit]                    |
+-----------------------------------------------------+

Workflow WIP queue
+-----------------------------------------------------+
| Task queued: Workflow WIP limit                     |
| Workflow: Development / Step: Implement              |
| 2 of 2 tasks admitted.                              |
| [Configure workflow WIP limit]                      |
+-----------------------------------------------------+

Phone, same persistent region above task content
+-------------------------------------+
| Automatic launch: Queued            |
| Global session limit               |
| Applies to all workspaces.          |
| Destination: Codex / 5.6 Luna Max    |
| 5 of 5 sessions in use.             |
| Checked 11s ago.                    |
| Queued since 26s ago.               |
| Automatic retry enabled.           |
| [Configure global session limit]    |
+-------------------------------------+
```

Scope, units, target identity, and a visible internal link are required. Example
names/counts are illustrative. WIP uses its destination, even when the task is
in a feeder; unavailable identity/counts must not be guessed. Errors keep their
actual reason; counts remain labelled stale when necessary. Phone links wrap
and have 44px targets without adding a scroll owner. Configuration navigation
and Back must not execute work. After a save expands capacity, show Retry pending
until dispatch confirms; a WIP-to-global queue change names the new current cause.

These previews map to AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.8-.9 and
AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.7-.9. Task 03 owns the rendered checks.

## Tests

Here AC-001.N abbreviates `AC-AGENTS-SESSION-CEILING-001.N`; AC-002.N uses the
same prefix with `002.N`. Proposed test names are implementation targets.

| Test file / test | Acceptance evidence |
| --- | --- |
| `system/sessioncapacity/resolver_test.go`: `TestResolveSessionCapacity` | AC-001.7, AC-002.1, .4: missing setting, valid/invalid env, saved off/on, zero override |
| `system/sessioncapacity/service_test.go`: `TestSessionCapacitySaveAndReload`, `TestSessionCapacityUpdateFailure` | AC-002.2, .5: remember maximum, partial patch, save-before-apply, no mutation on failure |
| `system/sessioncapacity/handler_test.go`; `system/system_routes_test.go` | AC-002.4, .5: actual admin route wiring, member rejection, validation and lock responses |
| `backendapp/session_capacity_settings_test.go`: `TestSessionCapacityStartup` | AC-001.7, AC-002.2: shared store wiring, restart restoration, missing key upgrade, load error before workers |
| `orchestrator/session_ceiling_config_test.go`, `session_ceiling_wiring_test.go` | AC-001.7: zero constructor default, injected configuration, no CPU dependency |
| `orchestrator/session_ceiling_settings_test.go`: `TestDisabledCeilingAdmission`, `TestSessionCapacityLiveChange`, `TestSessionCapacityChangeReplaysDeferredLaunch` | AC-001.8, .9, AC-002.3: failing lister, no override notice, concurrent updates, held reservation, increase/disable replay |
| `components/settings/system/session-capacity-settings.test.tsx`; `lib/api/domains/settings-api.test.ts` | AC-002.1-.7: draft/reset/save, partial update, reload, errors, member/env lock, edit during save |
| `lib/settings-discovery/catalog.test.ts` and target coverage tests | AC-002.6: searchable section and valid target |

Retain existing ceiling admission, recovery, and queued-session-ownership tests
for AC-001.1-.6. Tests requiring a bounded ceiling must configure it explicitly.

## E2E tests

Task 03 adds queue-limit navigation and WIP-to-global transition scenarios in
`tests/workflow/queue-limit-navigation.spec.ts` and
`mobile-queue-limit-navigation.spec.ts`. Its work order maps the task-owned ACs
to focused tests and exact commands. These complement the settings flows below.

Add `tests/system/session-capacity-settings.spec.ts` for `chromium` and
`tests/system/mobile-session-capacity-settings.spec.ts` for `mobile-chrome`.
Use the existing worker-isolated fixtures and a shared helper that captures and
restores settings even after an assertion fails. Acquire page fixtures before
changing baseline state. Never change the developer's instance.

- Desktop: find through Settings search, observe off, enable with a maximum of
  one, Reset, enable and save, reload, change maximum, disable and reload.
  Observe the actual effective value after each save (AC-002.1, .2, .6, .7).
- Admission flow: occupy one slot with a controllable mock session. An automatic
  workflow launch waits when the saved ceiling is one. Disable through Settings
  and observe that exact selected session resume once. Manual launches still
  work at capacity. Default-disabled automatic starts do not queue solely because
  another session runs (AC-001.1, .2, .8, .9, AC-002.3).
- Phone: enter through home navigation > Settings > Task Behavior, enable,
  edit, save, reload, disable, and Reset a draft. Assert 44px input/switch/action
  hitboxes, the single scroll owner, final-control visibility above the save bar,
  and no document horizontal overflow (AC-002.6).
- Both: inline validation and managed/read-only state remain understandable.
  Backend tests prove authorization; mocked UI responses can isolate error and
  lock presentation without replacing the real-save happy path (AC-002.4, .5).
- Run the existing desktop/phone queued-session-ownership specs with their
  explicit ceiling setup to detect recovery regressions. Do not depend on the
  removed CPU default to arrange a queue.

Record rendered phone evidence during the focused run. Use causal HTTP/WS waits
and existing API polling for backend state, not sleeps. The managed runner
builds current backend and frontend assets before each selected project run.

## Work orders

- [x] [Task 01: Persist and apply opt-in session capacity](task-01-capacity-settings.md)
- [x] [Task 02: Expose session capacity in Settings](task-02-settings-surface.md)
- [x] [Task 03: Explain queue limits and link configuration](task-03-queue-limit-navigation.md)

Task 02 depends on Task 01's working settings API. Task 03 depends on Task 02's
registered settings target and covers both queue types. Execute sequentially;
final integration and verification remain in the primary session.

## Related delivery records

The completed [original ceiling plan](../session-concurrency-ceiling/plan.md)
records the previous environment-only behavior. Its results remain historical.
The [queued-session-ownership plan](../queued-session-ownership/plan.md) remains
the recovery/inspection compatibility input. Its explicit environment limit
continues to enable the ceiling for tests. Neither package needs its recorded
results rewritten as evidence for this change.

## Documentation impact

`docs/public/tasks-and-workflows.md` now documents the Settings path, disabled
default, live-save behavior, manual bypass, and environment override. The page
keeps workflow WIP and global session capacity as separate concepts. Searches
found no stale wording in `README.md` or `docs/screenshots.md`. The configuration
inventory wording was updated with the startup-only environment override.

## Verification results

Design-package validation on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed; 287 decisions and 1000 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- `git status --short -- docs/specs docs/plans`: confirmed all three work orders and
  the new manifest are present, alongside the paired specs and companion notes.

After the queue-banner refinement, catalog validation, full specification lint,
work-order ID/path checks, and whitespace checks passed again. Workflow live
application is covered by the `WorkflowStepUpdated` regression test and managed
desktop/phone scenarios.

Implementation validation on 2026-09-18:

- Backend package tests passed for backend wiring, system settings, session
  capacity, orchestrator, task service, workflow handlers, and MCP handlers.
- `go test -race ./internal/orchestrator ./internal/system/sessioncapacity -count=1`
  passed; `make lint`, `make build`, and `go run ./cmd/settings-catalog --check`
  passed.
- Focused frontend tests passed: 68 tests across 9 files. Web typecheck, full
  ESLint, i18n validation, and the production Vite build passed.
- Managed Chromium passed 4 new queue/settings tests, and managed mobile
  Chromium passed 2 new Settings/queue tests. Existing queued-session ownership
  desktop and mobile tests passed after their scoped wording assertions were
  updated.
- Public documentation validation passed for 62 validator tests and 46 pages;
  specification catalog validation and full specification lint passed.
- No live instance settings were changed, and no commit, push, or PR was made.

## Risks

- Applying a new limit must not replace the controller and lose reservations.
- A disabled ceiling must bypass the current population-read failure branch.
- An upgrade must not save the old derived limit as a user preference.
- A disabled sweeper would strand existing deferred work; keep it running.
- Settings storage and startup wiring must use the same resolved value as the
  admission controller. Do not report a saved limit while running another one.
- Explicit environment overrides remain authoritative. Removing one requires
  restart; UI saves do not.
