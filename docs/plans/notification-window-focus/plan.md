---
created: 2026-10-18
status: implemented
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/local-notification-attention.md
legacy_specs: []
---

# Implementation Plan: Notification Window Focus

## Overview

Restore Local attention when the current task's window remains visible but loses
focus. One sequential work order extends real-handler tests, demonstrates RED,
adds the focus conjunct to the shared suppression predicate, and verifies GREEN.
The existing [Platform requirement](../../specs/platform/requirements/notifications.md)
owns this outcome; the [focused design](../../specs/platform/system-design/local-notification-attention.md)
defines its client boundary. Existing criteria .1-.10 remain unchanged; .11
adds the explicit attention rule.

## Scope

In scope: current document focus in the Local task attention gate, deterministic
regressions in the existing handler test, and bounded delivery compatibility
controls. Production ownership is limited to
`apps/web/lib/ws/handlers/notifications.ts`; test ownership is limited to its
existing `notifications.test.ts` sibling. The four design artifacts are this
plan, its work order, the existing requirement, and the new focused design.

Out of scope: focus listeners/caches, persistent settings, OS integration,
notification activation/navigation, retry, backend/provider changes,
route/activeTask lifecycle, notification settings races, new event semantics,
markup, translated copy, dependencies, broad E2E, and full web suites.

## Technical approach

Keep `registerNotificationsHandlers`, `registerNotificationHandler`, and all
post-admission paths intact. Add the current `document.hasFocus()` condition
inside `shouldSuppressNotification`. Retain shared Office handling and the
separate update handler. Do not couple this predicate to session Git recovery.

| Provider/path | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Local browser | Web Notification API; existing semantic payload | Apply attention gate, then current permission checks | Real handler with stub constructor; denied/unavailable API keeps sound admission only |
| Local desktop | Native invoke; event type plus envelope/occurrence ID | Same attention gate; native dedup remains authoritative | Real handler with fake Tauri invoke; no browser duplicate; missing task/occurrence retains browser fallback |
| Office inbox | Same shared task gate; current Office identity | Same attention rule when task-scoped; unscoped keeps fallback | Focused Office controls and existing no-task fallback |
| Local update | Separate in-app bridge | Existing routing | Existing update registration control |
| System/Apprise/failure paths | Existing independent transports | Unchanged; outside this predicate | No new support claim or transport fallback |

## Tests and acceptance mapping

All changes go in `apps/web/lib/ws/handlers/notifications.test.ts`. Create a
`notification window attention` describe group with these behavior names;
parameterize each across the two semantic events and browser/native targets:

- `delivers for the visible unfocused current task` (.11; causal RED).
- `suppresses for the visible focused current task` (.11; control).
- `delivers for a hidden current task` (.11; control with focus explicitly true
  to isolate the visibility condition).
- `delivers for a visible focused other task` (.11; control).
- `samples focus for each distinct occurrence` (.11; focused to unfocused to
  focused after registration, distinct IDs; causal RED at unfocused delivery).

Use the same registered real handler across the focus transition. Explicitly
stub visibility and `hasFocus()` independently. Assert both the sound hook and
actual selected dispatch boundary; never reproduce the predicate in a test
helper or import ROOT's private qualification source. Restore all document and
Tauri overrides. Update the old viewing-task suppression test to explicitly set
focus and visibility so it does not depend on happy-dom defaults.

Keep existing registration, copy, native identity/dedup delegation, missing
identity fallback, browser denied/unavailable, Office no-task, and update bridge
controls green (.2-.4 compatibility). Add bounded task-scoped Office suppression
and unfocused-delivery controls because the helper is shared. Sound preference
logic remains in the unchanged sound helper; tests observe gate admission, not
audible playback or an OS toast.

## E2E and mobile applicability

The real registered handler is the complete changed dispatch boundary. The
browser constructor/native invoke are mocked external endpoints. No fresh
browser or OS-toast proof is required for this bounded predicate. Mobile-parity
assessment: no rendered, navigation, touch, scrolling, or viewport change;
the deterministic handler matrix covers shared browser logic. No new Playwright
scenario or ASCII UI preview is warranted. Docs-maintainer assessment: internal
specs/plans updated; public desktop/WebSocket contracts remain accurate, with no
public API, setting, copy, or screenshot change.

## Work orders

- [x] [Task 01: Notify when the task window is unfocused](task-01-notify-unfocused-window.md)

No work-order dependencies. The single work order was implemented sequentially
in the same primary session after the reviewed implementation release.

## Verification strategy

Exact commands and expected RED/GREEN evidence are in
[Task 01](task-01-notify-unfocused-window.md#verification). Use Node 24, one frozen
workspace installation only when needed after implementation authorization,
Vitest 4 with `--maxWorkers=1`, scoped eslint and canonical `pnpm run typecheck`, spec
validation, documentation coverage preflight, and normal commit hooks. No
`--minWorkers`, full web suite, broad E2E, or extra generic review gate.

## Verification results

Design validation completed on 2026-10-18:

- `python3 scripts/list-docs.py validate`: passed (386 decisions, 1645 specs).
- `python3 scripts/list-docs.py specs --system platform --kind system-design
  --format paths`: discovers the new focused design beside the unchanged
  settings atomicity design.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local link/source-symbol check: 18 Markdown targets resolve across the four
  artifacts; referenced handler functions and the single work order exist.
- Repository `validateCoverage` preflight from Task 01: `covered`, no errors,
  against the package plus the planned production path. Used installed Node
  24.21.0 directly because this shell has no `node` on PATH. This is reference
  validation, not an implementation or hosted CI result.
- `git diff --check` and the plan-scoped diff check: passed. Plan-scoped status
  includes the untracked work-order directory, as expected before handoff.
- `git diff --exit-code HEAD -- apps/web/lib/ws/handlers/notifications.ts
  apps/web/lib/ws/handlers/notifications.test.ts`: passed; both files unchanged.

The design phase ran no dependency installation, product tests, eslint/typecheck,
hooks, browser/OS proof, commit, or push. Implementation is now authorized. The
reviewed verification amendment uses canonical `pnpm run typecheck` once after
final affected GREEN and scoped eslint, replacing the temporary TypeScript config.

Implementation completed on 2026-10-18. The final real-handler file passes all
39 tests, scoped eslint passes with zero warnings, and canonical typecheck
passes with the explicitly authorized task-local 8 GiB heap. The filtered RED
run proved 10 missing dispatches with 14 passing controls before production
changed; filtered GREEN passed all 24 attention cases. The original default
4 GiB typecheck failed with V8 heap OOM (exit 134); the first 8 GiB attempt
reported owned fixture-only TypeScript diagnostics (exit 2). Those fixture types
were corrected and all affected checks then passed. Historical failures remain
recorded in [Task 01 results](task-01-notify-unfocused-window.md#results).
Normal hook receipts and current-head hosted CI/review evidence are recorded in
the external task plan during delivery.

## Risks

- Environment-default document focus can hide the regression; fixtures must
  explicitly set and restore both inputs.
- The shared Office helper must retain event identity and taskless fallback.
- Native/browser stubs prove dispatch only. Permission, desktop origin checks,
  native dedup, OS delivery, and autoplay remain their existing contracts.
- A suppressed occurrence is not replayed on later blur; only new occurrences
  use the new attention state.
