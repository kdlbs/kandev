---
created: 2026-09-17
status: complete
requirements:
  - REQ-OFFICE-TASKLESS-001
  - REQ-OFFICE-RUN-OBSERVATION-001
  - REQ-OFFICE-DASHBOARD-001
  - REQ-OFFICE-KILL-SWITCH-006
  - REQ-OFFICE-CONFIG-EXPORT-001
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
  - ../../specs/office/system-design/run-observation.md
  - ../../specs/office/system-design/workspace-topbar-actions.md
  - ../../specs/office/system-design/workspace-kill-switch-02.md
  - ../../specs/office/system-design/config-export.md
legacy_specs: []
---

# Implementation Plan: Office Mode Repairs

## Overview

Deliver genuine taskless coordinator execution and repair Office run/activity
identity, workspace action placement and configuration export. Implemented in the
primary session, sequentially, with the user-authorized plan package.

Office owns all four vertical contracts. Shared runtime, task ownership and
workspace authorization are dependencies, not alternate specification owners.
[ADR](../../decisions/2026-09-17-office-taskless-run-sessions.md) records why taskless
sessions are run-owned instead of synthetic tasks or nullable task sessions.

## Confirmed evidence

The read-only diagnostic bundle from port 38429 reported ready, no warnings,
version v0.94.0-201-g6d8e54ffd. Run `a99a415f-aacf-4eaa-a2ea-0bfd92c058db`
was dispatched at 14:40:26 UTC and rejected at 14:40:31 UTC on September 17:
`cannot launch taskless run`, agent CEO, reason routine_dispatch_cron, executor
local_pc. Another rejection occurred at 14:45:31 UTC. No model launch occurred
for the rejected run. The checkout at investigation was e8e8bcff6, distinct from
the live binary; source inspection independently confirmed both rejection gates.
The diagnostic temporary directory was removed; no live settings were changed.

Other confirmed causes:

- `dashboard/run_detail.go:buildInvocation` assigns agent.ID as adapter.
- RuntimePanel prints skill_id; ActivityRow prints actorId and targetId.
- OfficeShell mounts WorkspacePauseState under PageShell; paused, stale and
  unavailable branches own the conditional second row. Refresh reads pause
  state only.
- OFFICE_ROUTES omits the existing export page. Both export endpoints returned
  200 and a 34-entry ZIP passed integrity checks. Selection is ignored by download;
  frontend preview independently serializes YAML and omits the ZIP path prefix.

## Scope

In scope: taskless run lifecycle and exact attribution; readable run/activity
labels; topbar workspace actions; truthful selected-file export; desktop/mobile
coverage and relevant public documentation.

Out of scope: changing routine cadence or idle defaults, interactive taskless
chat, replaying historical failures, broad activity wording redesign, import/sync
mutation redesign, unrelated placeholder routes, and live-instance changes. PR
publication is handled as the delivery step for this implementation.

## Technical approach

1. Add Office run-session persistence and a typed shared-runtime owner/start seam.
   Task lifecycle invariants remain intact. No launch path is enabled until its
   admission, cancellation and reconciliation are wired.
2. Integrate concrete/routed scheduler launch, first prompt, exact lifecycle
   events, usage, continuation, stop and restart recovery as one functioning slice.
3. Add additive name/snapshot projections and update run/activity rendering.
4. Share one pause controller between topbar controls and conditional banners.
5. Register export route and use one server manifest for preview/selected ZIP.

New API/type/file names in the work orders are proposed outputs, not claims that
they already exist. Existing integration points were inspected in source.

## ASCII UI preview

The control order, identity hierarchy, phone composition and failure affordances
below are structural requirements. Copy and spacing are illustrative; use existing
localized strings and UI primitives where possible. Phone views use a single
content scroll owner and safe-area-aware controls. No document horizontal overflow.

```text
UI-01: Agent Runs / Activity, populated and missing identity
Desktop
[Agents > CEO                         Refresh pause  Pause workspace]
[Run status | CEO | duration | cost]
[Adapter: Codex   Model: <actual model>]
[Skills: Planning  v...  hash...]
[Activity: CEO recorded a decision on KAN-14 Build report]

Phone
[< CEO Runs                  Workspace actions]
[Status / duration / cost]
[Adapter: Codex]
[Skills: Planning / version / hash]
[CEO recorded a decision]
[KAN-14 Build report]

Missing: [Skill unavailable (04957907)]
Before invocation: [Adapter: Not started]
```

```text
UI-02: Office shell, running / paused / unknown
Before (desktop, from source and screenshot)
[Breadcrumb                                    ]
[                         Refresh  Pause workspace]
[Page content]

After desktop
[Breadcrumb        Page actions | Refresh pause | Pause workspace]
[Paused / stale / unavailable banner, only when relevant]
[Page content, scrolling]

After phone
[Menu  Page title                  Workspace actions]
[Paused / stale / unavailable banner when relevant]
[Page content, scrolling]

Workspace actions opens inset bottom drawer:
[Workspace name]
[Refresh pause status]
[Pause workspace / Resume workspace]
[Close]
Pause then opens the existing reason + confirmation flow.
Partial stop: banner retains [Retry stop].
```

```text
UI-03: Preferences > Export
Desktop
[Preferences > Export                 Workspace actions]
[Workspace / selected count                    Download selected]
[File list + checkboxes | Selected file preview]

Phone: file list
[< Preferences   Export                Workspace actions]
[Workspace / selected count]
[[x] .kandev/kandev.yml                 View]
[[x] .kandev/agents/ceo.yml             View]
[Download selected]

Phone: selected file
[< Back to files    ceo.yml]
[Full-width content, contained code scroll]

Loading: [Loading export...]
No workspace: [Select a workspace]
Failed load: [Could not load export] [Retry]
Empty/none selected: [No files selected] [Download disabled]
Stale download: [Configuration changed] [Reload preview]
```


UI-01 maps to AC-OFFICE-RUN-OBSERVATION-001.1-.5; UI-02 to
AC-OFFICE-KILL-SWITCH-006.4-.14; UI-03 to AC-OFFICE-CONFIG-EXPORT-001.1-.4.

## Tests

Each work order lists exact commands and proposed regression test names. First
run the new regression red, then implement. Include existing neighboring tests
when they protect affected lifecycle contracts. The runtime task is intentionally
an internal compatibility expansion; the following task activates it only after
end-to-end lifecycle checks pass. Do not weaken tests by accepting a skipped
Postgres check as database compatibility evidence.

## E2E tests

- `office/taskless-routine-session.spec.ts`: real mock-agent initial prompt and
  completion, no task created, actual invocation/usage, second fresh session,
  pause during execution (taskless .1-.5, .8).
- `office/run-observation.spec.ts` and `office/mobile-run-observation.spec.ts`:
  direct run/activity entry, names, legacy/missing/long labels (observation .1-.5).
- Existing `office/workspace-kill-switch.spec.ts` and
  `office/mobile-workspace-kill-switch.spec.ts`: topbar and all pause states (.4-.14).
- `office/config-export.spec.ts` and `office/mobile-config-export.spec.ts`:
  navigation, exact selected ZIP contents, preview/back, retry and workspace
  switching (export .1-.4).

Use chromium for desktop and mobile-chrome for mobile. Build fresh backend/web
assets before each affected E2E run and use guarded repository runners with their
normal worker limits. Do not run overlapping full suites or touch the live instance.

## Work orders

- [x] [01: Run-owned runtime foundation](task-01-run-session-foundation.md)
- [x] [02: Taskless scheduler lifecycle](task-02-taskless-lifecycle.md)
- [x] [03: Run and activity names](task-03-run-observation.md)
- [x] [04: Workspace topbar actions](task-04-workspace-topbar.md)
- [x] [05: Configuration export](task-05-config-export.md)

## Verification results

Design and implementation checks passed on September 17, 2026:

- `python3 scripts/list-docs.py validate`: 286 decisions and 994 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Work-order reference/link check: all five orders resolve their requirement,
  acceptance and design references.
- `git diff --check -- docs/specs docs/decisions docs/plans/office-mode-repairs`: passed.
- `git status --short -- docs/plans/office-mode-repairs`: package present with
  all five completed work orders and recorded results.

Implementation evidence:

- `GOCACHE=/tmp/kandev-go-cache go test ./internal/office/dashboard ./internal/office/config ./internal/office/repository/sqlite ./internal/office/service ./internal/office/pause ./internal/office/scheduler ./internal/backendapp -count=1`: passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/office/service ./internal/office/pause -run 'Taskless|RunSession|Pause' -race -count=1`: passed.
- Focused frontend regressions: 5 files and 25 tests passed for activity labels, runtime labels, run headers, pause controls and export API.
- `pnpm run typecheck && pnpm run i18n:check`: passed; all translated catalogs remain complete and the non-JSX copy gate is clean.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `node scripts/validate-public-docs.mjs`: passed.
- `GOCACHE=/tmp/kandev-go-cache make -C apps/backend build` and `pnpm --filter @kandev/web build:vite`: passed. The E2E runner's first host build required the cache override because the sandbox denies the default Go cache path.
- New desktop/mobile E2E specs parse successfully with Playwright project listings. The taskless routine spec passed in the Docker-backed runner, including a real mock-agent invocation with no task row. The corrected run-observation, export and mobile specs could not complete in this environment: the Docker rerun was stopped during a fresh glibc-matched backend image build, and the native host runner was blocked before browser execution by Chromium's MachPort sandbox permission error. The remaining browser checks are therefore recorded as environment-blocked rather than passing evidence.
- PostgreSQL migration tests were skipped because `KANDEV_TEST_POSTGRES_DSN` is unset; this is not PostgreSQL compatibility evidence.

## Risks

- Runtime ownership/admission and startup are cross-cutting. An empty task ID
  bypass is not an acceptable implementation of taskless support.
- Pause/recovery must stop every live taskless execution, including a failed
  predecessor beside a live successor; cancellation cannot rely on queue state.
- Recorded adapter evidence may not exist for historical runs. Show unknown,
  not a current-profile guess presented as history.
- Preview manifests can stale; revision checking deliberately returns a visible
  conflict rather than exporting different content from that reviewed.
- Phone drawer focus and full-height export preview need rendered checks.
- PostgreSQL proof requires an isolated configured test database. Missing DSN is
  an explicit verification blocker for the affected migration work.

## Review remediation (2026-09-18)

The four local review findings were corrected in the primary session:
run-owned runtime registration and scratch workspaces, normal ACP turn completion,
workspace/run cost attribution, and mobile confirmation lifetime. Regression
coverage now uses the production runtime inventory writer and serialized usage
frames. The taskless E2E fixture assigns its routine and requires two finished
agent runs with distinct session IDs. Mobile confirmation remains outside the
closing actions drawer, preserving the existing UI-04 composition and controls.

Delivery uses an independent checkout because this workspace's Git metadata is
outside the writable sandbox. Source changes remain in this workspace too.

Remediation verification:

- Office service, scheduler, pause, runtime, wakeup, costs and runs repository suites passed.
- Runtime/Office run-owner, taskless and run-session regression tests passed with `-race`.
- Backend composition checks (`Taskless|Routine.*Session|Office.*Scope`) passed.
- `taskless-routine-session.spec.ts` passed against a rebuilt backend with two real mock-agent completions.
- Mobile pause/resume component regressions, web typecheck and focused ESLint passed.
- Go lint reported zero issues; documentation catalog and specification lint passed.
- Native mobile Playwright was blocked by macOS Mach-port permissions. The rebuilt Docker mobile-chrome run of `mobile-workspace-kill-switch.spec.ts` passed, including pause and resume after drawer unmount.
