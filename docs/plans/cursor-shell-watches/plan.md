---
created: 2026-11-24
status: done
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-011
system_design:
  - ../../specs/agents/system-design/cursor-shell-watches.md
legacy_specs: []
---

# Fix plan for Cursor ACP shell watches

## Overview

Project Cursor ACP shell watches into the shared Background Work viewer. Start
with the raw notification regression. Then add adapter recognition, launch-turn
attribution, and projection checks in one sequential work order.

GitHub confirmed `carlosflorencio` as the assignee on 2026-11-24. [Issue #5485](https://github.com/kdlbs/kandev/issues/5485)

## Confirmed evidence

The six captured notifications replayed through `handleACPUpdate` on
`4b7c8fb6c50`, the default branch head at investigation time. The temporary
test enabled Background Work explicitly and set the native session binding.
The issue's standard test helper creates an adapter with Background Work off.
For the permanent replay, set `AgentBackgroundWorkEnabled: true` in the adapter
configuration.

The adapter emitted two identical status sequences, `pending`, `in_progress`,
and `complete`. All six events had generic payloads. No event carried a
Background Work observation. `TestCursorShellWatchRepro` failed at that
assertion. The test and capture ran through a temporary Go overlay.

`newCursorACPDialect` has no shell watch recognizer. The existing Claude Monitor
path requires `_meta.claudeCode.toolName`. Cursor supplies its watch identity
on the initial tool call and sends a sparse terminal update. Background Work
cannot derive the watch from those generic transcript events.

## Scope

### In scope

- Cursor ACP registration and updates for the captured shell watch shape.
- One Monitor for each watch in the existing desktop and phone viewer.
- Running count, watch details, and retained History with the original owner.
- A public documentation note in **Inspect background work** at implementation.

### Out of scope

- Cursor foreground handoff and same-turn steering.
- Shell output, shell controls, shell outcomes, and historical backfill.
- Provider discovery and new desktop or phone components.

## Technical approach

Extend the [requirements](../../specs/agents/requirements/cursor-shell-watches.md)
and [design](../../specs/agents/system-design/cursor-shell-watches.md) in the
Agents system. The shared Background Work contract already covers persistence,
History, and inspection. A companion pair defines the new Cursor support.

Add a pure parser in `dialect_cursor.go`. The existing notification worker
keeps one map keyed by watch call ID for the bound native session. Each watch
uses one ID and one label through progress and terminal updates.

Emit the existing `EventTypeBackgroundWorkUpdated` with the existing
inspection-only capabilities. Resolve the saved launch generation with
`promptTurnIDForGeneration`. The shared projection stores the observations and
drives the running count, details, and History.

| Provider and transport | Evidence used | Result | Ordinary tool path |
| --- | --- | --- | --- |
| Cursor ACP | Bound `watch-` call, valid shell references, and matched lifecycle updates | Events-only Monitor | Invalid launch, unmatched update, or disabled flag |
| Claude ACP | Existing Monitor metadata and matched lifecycle evidence | Existing Monitor projection | Cursor-shaped frames without Claude evidence |
| Other ACP dialects | Their existing supported observations | Their existing inspection capabilities | Cursor-shaped frames follow each dialect's normal rules |

## ASCII UI preview

**UI-01 Cursor watch inspection.** The composer is the entry point for an
active watch. The independent Background Work entry opens History after the
last watch finishes. Existing Monitor labels, tabs, and detail controls apply.
The title and shell path are sample data. The existing detail view scrolls.

```text
Current
  Transcript: Watching: Wait for build
  Composer: [Queue]

Desktop, active
  [1 Background Job] -> popover: Monitor / Wait for build (/tmp/build.sh) [Open]
  Composer: [Queue]       Open -> central detail tab
  Detail: Wait for build (/tmp/build.sh) | Output unavailable
  Completed -> + panel menu -> Background Work -> History (1)

Phone, active
  [1 Background Job] -> summary drawer: Monitor / Wait for build... [Open]
  [Queue]                 Open -> full-height detail [Back]
  Detail: Wait for build (/tmp/build.sh) | Output unavailable
  Completed -> Panels -> Background Work -> History (1)
```

The preview covers `AC-AGENTS-BACKGROUND-WORK-011.1`, `.2`, `.4`, and `.5`.
The Go replay and projection checks establish the Cursor behavior. The existing
Playwright checks exercise the shared desktop and phone paths.

## Tests

- `011.1` and `011.2` use `TestCursorShellWatchLifecycle` and
  `TestCursorShellWatchProjection` for two interleaved watches. The Go tests
  exercise raw ACP parsing, one terminal transition, and retained History.
- `011.1` and `011.2` use `TestBackgroundWorkObservationOriginTurn` for captured
  turn ownership when a successor is current. `TestCursorShellWatchIdentity`
  tests a different session, native generation, and replacement adapter.
- `011.3` uses `TestCursorShellWatchRejectsUnsupportedFrames` for ordinary,
  malformed, disabled, and foreign-provider calls.
- `011.4` uses the Go tests for absent shell results and action capability. The
  existing `background-work.test.tsx` verifies title display and fallback text.
- `011.5` uses `TestCursorShellWatchProjection` and the existing
  `TestPromptTask_NonClaudeFramesStayBusy` for the coarse admission rule.

## E2E tests

For `011.1`, `011.2`, and `011.4`, use the shared Monitor lifecycle tests below.
Their Claude fixture proves the running count and History after reload on both
surfaces. The Cursor ACP replay tests supply provider-specific evidence.

- `apps/web/e2e/tests/task/claude-monitor-lifecycle.spec.ts` in `chromium`.
- `apps/web/e2e/tests/task/mobile-claude-monitor-lifecycle.spec.ts` in `mobile-chrome`.

## Work orders

- [x] [Task 01 Track Cursor shell watches in Background Work](task-01-project-cursor-shell-watches.md) — done

## Verification results

The raw replay reproduced the defect on `4b7c8fb6c50` and now emits one running
and one terminal observation per watch. ACP failure, cancelled-prompt, and
native-reset regressions verify terminal precedence, prompt ownership, reset
fence delivery, and call-ID reuse. The ACP, lifecycle, and orchestrator package
suites passed with the race detector enabled. The backend build and
changed-package lint passed. The shared viewer suite passed all 27 tests.
The PR review regression reproduced the buffer-cap loss with 64 setup events
and 71 watch retirements, then passed with every retirement retained and setup
events still capped. The full lifecycle package passed with the race detector;
the backend build passed, and changed-file Go lint reported zero issues.
Desktop and mobile Monitor lifecycle E2E each passed one test, with the host
backend and web assets built by the E2E runner. Changed-file Go lint passed
with an extended timeout after package loading exceeded the wrapper's default.
After rebasing onto `8aab9447b35f`, the ACP, lifecycle, and orchestrator race
suites and backend build passed again; the package-level Go lint also reported
zero issues on that base.
Public docs validation passed all 63 tests and validated 56 published pages;
spec validation passed for 464 decisions and 2,110 specifications, and all
spec-file lint checks passed. `git diff --check` passed.

## Risks

- Cursor can change its provider-specific payload. The recognizer requires the
  captured shape and leaves unfamiliar shapes on the ordinary tool path.
- The capture covers one native session. The precise Cursor launch version
  and live reconnect behavior are unverified.
- A terminal frame can lack every display field. Persist the captured launch
  association through matched progress and completion.
