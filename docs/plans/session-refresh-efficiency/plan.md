---
created: 2026-09-29
status: in_progress
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-002
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-003
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-005
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-006
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Implementation Plan: Session Refresh Efficiency

## Overview

Reduce repeated work on an open task session without changing its recovery
behavior. First make the existing full-session GET conditional, then let the
frontend retain its current store object on an unchanged response. Share
environment-status reads between the toolbar and composer. The second trace
adds task-switch work in Tasks 04–08, described below. Execute sequentially
in the primary session; no delegation is authorized.

The 2026-09-29 trace covers 5.85 seconds of an open session. It contains six
77,279-byte decoded session responses, five following 34–38 ms main-thread
tasks, and two pairs of near-simultaneous environment-status requests. Equal
response lengths do not prove equal content. The 309 ms profiler initialization
task is not application work. This plan calls for a clean-browser repeat
measurement before claiming a performance gain.

## Ownership and assumptions

UI owns shared client reads and render publication on the open task page.
Platform retains session lifecycle, authorization, WebSocket subscription,
and recovery authority. The [session subscription recovery contract](../../specs/platform/requirements/session-subscription-recovery.md)
keeps the bounded state snapshot. No work order removes it. The existing
[task navigation responsiveness package](../task-navigation-responsiveness/plan.md)
owns shell/commit/diff sharing and progressive file-tree reads. Preserve those
completed work orders. New Tasks 04–08 cover different route, editor, feedback,
and initialization owners; they do not reopen completed file-tree work.

The intended change is data and state coordination inside existing desktop
and phone surfaces. Task 04 changes when destination content becomes available; its preview is
below. Control placement, copy, touch targets, and scroll owners remain.
Desktop and phone both require causal navigation and draft-restoration checks.

## Scope

### In scope

- Conditional full-session GET responses and a typed frontend 304 path.
- No store publication for unchanged reconciliation reads. Existing stale
  HTTP and WebSocket guards remain in force.
- One environment-status read owner per task and store, with active/background
  cadence, manual refresh, reset, and task-switch behavior preserved.
- Targeted unit, handler, desktop/phone E2E, and repeat trace evidence.
- Progressive client route hydration, editor lifecycle attribution and repair,
  shared PR feedback, agent discovery, and integration health reads.

### Out of scope

- A new session-status endpoint, event replay protocol, persistent cache, or
  changes to session lifecycle and permissions.
- Task-panel redesign, dependency replacement, general Zustand refactoring,
  and broad file-tree or transcript optimization.
- Treating the trace's node/listener counts or profiler startup task as a
  proven leak or application long task.

## Technical approach

1. In `apps/backend/internal/task/handlers/task_http_handlers.go`, serialize
   the authorized `GetTaskSessionResponse` after runtime enrichment. Use the
   stable pending-action snapshot projection so unchanged reads retain the
   same revision, then compute an opaque strong ETag over the exact JSON bytes
   and evaluate
   `If-None-Match` after the authorization/service path. Send `304` with no body
   only for an exact match. Otherwise preserve `200` JSON behavior. Keep
   caching private and revalidated. Handler tests cover runtime-only field
   changes, missing/invalid validators, and auth failure before comparison.
2. In `apps/web/lib/api/domains/session-api.ts`, add a dedicated conditional
   fetch result. In `session-state-reconciler.ts`, keep a validator per
   store/session owner. Treat `304` as no publication. Retain the existing
   hydration-epoch, stale-event, ref-count, and bounded-timer behavior for
   `200`. Do not change general `fetchJson` semantics for all endpoints.
3. Add a task-keyed environment read coordinator under
   `apps/web/hooks/domains/session/`. Let `use-task-environment.ts` and
   `use-executor-environment-availability.ts` subscribe to its raw response
   while preserving their local derived state and UI controls. Match the
   current 3-second active and 7-second background cadence and retire the
   owner after its last consumer.

No ADR is needed: this extends an existing GET with standard conditional HTTP
semantics and keeps the current session ownership boundary. The system design
records why this route was chosen over a new narrow endpoint.

## Tests

| Criterion | Focused evidence |
| --- | --- |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-001.1` | Handler ETag/304 tests and `use-session.test.ts` no-publication assertion |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-001.2` | Handler runtime-projection change test and deferred WebSocket/HTTP race test |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-001.3` | Missing ETag, 304 without cached full session, error/reconnect unit tests |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-002.1` | Two-consumer deferred-promise test with one request/timer |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-002.2` | Fake-timer cadence and late-task-response tests |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-002.3` | Manual refresh, 404, reset, and failure-preservation tests |

## E2E tests

Add `apps/web/e2e/tests/session/session-refresh-efficiency.spec.ts` and its
`mobile-session-refresh-efficiency.spec.ts` counterpart. Use seeded tasks and
causal request/response waits. For `AC-UI-SESSION-REFRESH-EFFICIENCY-001.2`–`.4`,
hold a session response, publish a newer state, and prove the visible state
does not regress. Also confirm an unchanged conditional response carries no
body. For `AC-UI-SESSION-REFRESH-EFFICIENCY-002.1`–`.3`, observe both environment
consumers mounted, count one request per active tick, and prove status/actions
remain usable on desktop and phone. The mobile scenario uses the existing
`SessionMobileLayout` and touch drawer. It does not change their composition.

## Work orders

- [x] [Task 01: Conditional session response](task-01-conditional-session-response.md)
- [x] [Task 02: Skip unchanged session publication](task-02-session-reconciliation-client.md)
- [x] [Task 03: Share environment status polling](task-03-shared-environment-status.md)

## Verification results

Implementation and focused verification on 2026-09-29:

- Backend task handler and service tests pass. The E2E runner builds the Go
  backend and production web assets successfully.
- The focused frontend suite passes: 11 files and 73 tests. Web TypeScript
  typechecking passes.
- Desktop and phone task-switch E2E suites pass, three tests each. They cover
  progressive navigation with held enrichment, shared PR feedback and
  invalidation, and shared unconfigured integration health.
- Desktop and phone session-refresh E2E suites pass, three tests each. They
  cover bodyless 304 revalidation, stale-response ordering, and shared
  environment refresh behavior.
- A matched clean-browser before/after trace has not been captured. The tests
  establish behavior and request coordination, not a measured timing or memory
  improvement. Task 05 remains open because the captured 213 ms Tiptap sample
  has not been attributed to a specific editor owner.

For a future comparison, use the same seeded session, viewport, browser build,
and clean browser profile. Record steady poll count, 304 hit rate, decoded
session bytes, response-following main-thread work, and duplicate environment
requests. Do not infer response equality from payload length. If the 304 hit
rate is low, inspect the live DTO fields that change on each poll. Revise this
design before claiming the bandwidth objective.

## Risks

- A validator based only on persisted `updated_at` can miss runtime-only
  changes. Hash the final authorized representation.
- A 304 without a retained full response can hide a session transition.
  Clear validators with their owner and retry unconditionally in that case.
- Joining environment requests can preserve a stale result after a task switch
  unless response ownership is checked at publication.
- Browser extensions affected the source trace. Use a clean profile for the
  repeat measurement and treat timing as comparative evidence, not a fixed
  product guarantee.

## Second trace: task-switch extension

The [10.70-second capture report](trace-2026-09-29-task-switch.md) documents a
1.21-second click interaction, a 1.15-second main-thread task, continuing render
congestion, and overlapping PR requests with genuine multi-second header waits.
This is a higher-priority user-visible problem than steady session transfer.
The report separates confirmed code paths from unresolved editor ownership,
backend latency attribution, and memory retention.

Keep Tasks 01–03 and their current implementation evidence intact. At design
time, Tasks 04–08 were pending. The implementation checkpoint below records
their current outcomes. Technical dependencies remain recorded per order.

- [x] [Task 04: Reveal tasks before optional hydration](task-04-progressive-task-navigation.md)
- [ ] [Task 05: Bound editor and render work](task-05-task-switch-render-lifecycle.md) (still open; see its Results)
- [x] [Task 06: Share PR feedback](task-06-shared-pr-feedback.md)
- [x] [Task 07: Share agent initialization](task-07-shared-agent-initialization.md)
- [x] [Task 08: Share integration health](task-08-shared-integration-health.md)

## ASCII UI preview

UI-01: Selecting a task while optional enrichment is pending.

```text
Desktop                         Phone
+---------+------------------+  +-------------------------+
| Task    | Destination      |  | Destination header      |
| sidebar | header           |  | Available conversation  |
|         | Available chat   |  | or local loading state  |
|         | Local loading in |  |                         |
|         | pending panels   |  | Existing bottom nav     |
+---------+------------------+  +-------------------------+
```

Before: the route loading overlay covers the destination until enrichment
settles. After: authorized destination identity appears first; pending surfaces
use their existing loading/error states. The desktop sidebar and phone task
drawer remain usable. Current panels retain their scroll owners, and phone
controls retain touch targets and safe areas. Draft/model readiness still gates
send. This structure is required; spacing is illustrative. No new copy is planned.
Maps to `AC-UI-SESSION-REFRESH-EFFICIENCY-003.1`, `.3`, and `.4`.

## Added acceptance coverage and measurement

| Criteria | Evidence owned by work order |
| --- | --- |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-003.1`–`.4` | Task 04: deferred route tests, same-scope identity joining, desktop/phone navigation while optional requests remain held |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-004.1`–`.3` | Task 05: attributed editor mount/render/cleanup tests, repeated-switch typing and draft restoration |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-005.1`–`.3` | Task 06: three-consumer feedback request, invalidation follow-up, workspace/auth isolation and slow-PR navigation |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-006.1` | Task 07: concurrent discovery, route join, profile-version race and visible current model |
| `AC-UI-SESSION-REFRESH-EFFICIENCY-006.2`–`.3` | Task 08: shared 204, cadence/cleanup, disabled state, credential invalidation and scope switching |

The shared browser files are `task/task-switch-efficiency.spec.ts` and
`task/mobile-task-switch-efficiency.spec.ts`, in Chromium and mobile-chrome.
Hold responses causally; assert destination availability before releasing them.
Include cold/warm switches, tasks with no session, non-primary selection,
missing/forbidden task recovery, live message arrival, and A-B-A draft restoration.
No fixed sleeps or machine-dependent latency gates are required in CI.

For each order, repeat a matched production capture without extensions and
record click-to-next-paint, destination header/transcript/composer readiness,
long-task count/max/sum, editor construction/destruction counts, actual mounted
nodes, request count and header waits. Repeat with normal plugins enabled.
Use at least five warm switches and report median and worst values. Desired
profiling targets are click feedback below 200 ms, no switch task above 200 ms,
and at least 50% less main-thread work for the matched interaction. These are
provisional experiment targets, not established product guarantees or flaky CI
thresholds. A result that misses them requires explanation and remaining-owner
attribution before the sluggishness is reported as resolved.

Preserve the existing render-isolation, large-tree geometry, and navigation
regressions. The renderer counters warrant a bounded repeated-switch retention
check; a leak claim requires retained-owner evidence after collection.

## Design extension verification

Documentation checks passed: specification catalog validation, specification
lint, whitespace checks, and synthetic PR-documentation coverage for all eight
work orders. The coverage preflight uses a representative implementation path.
At this design-package checkpoint, product implementation and performance
comparison were still pending. Existing modified test files were preserved.

## Implementation checkpoint (2026-09-29)

- [x] Tasks 01–04: Implemented; targeted backend/web tests pass, and desktop/phone session-refresh and progressive-navigation E2Es pass.
- [ ] Task 05: Remains in progress. Repeated composer creation/destruction was observed, but the capture does not establish that it owns the 213 ms Tiptap CPU sample. No speculative lifecycle rewrite was made. A component-attributed profile and matched A-B-A trace are still required.
- [x] Task 06: Shared PR feedback resource and invalidation are implemented and verified. Single-request backend/upstream latency attribution remains open because these browser tests use the mock GitHub backend.
- [x] Task 07: Shared agent discovery and route joining are implemented. The browser test verifies a single outstanding list read while held and the destination's current model after release; the existing available-agent freshness pass may issue its own later follow-up.
- [x] Task 08: Shared integration health reads are implemented and verified for settled no-config results, disabled consumers, scope changes, invalidation, and teardown.
- [x] Final focused frontend tests, TypeScript check, task handler/service tests, production builds, and desktop/phone E2Es passed.

## Code-review remediation (2026-09-29)

A second code-only review found five correctness regressions in the uncommitted
implementation. All five are now covered by focused regressions and fixed:

- Task 04 omits unfetched turns from the navigation shell. Real-store tests cover
  successful history enrichment, optional failure followed by normal retry,
  and preservation of live turns during later hydration.
- Task 01 keeps snapshot representation state, completed observation order, and
  workspace event deduplication separate. Tests cover a full-session snapshot
  before the message event, a newer unchanged event before an older changed
  event, and a newer unchanged snapshot before an older changed read.
- Task 04 binds task identity reads to the original immutable store owner token
  and checks that token before shell and enrichment publication. A real-store,
  deferred response test changes workspace context and rejects the old result.
- Task 07 does not create or subscribe to agent discovery while settings data is
  disabled, and releases its listener when disabled.

Verification after remediation: the full backend task-service test package,
`make -C apps/backend build`, the four targeted web test files (44 tests), web
TypeScript typecheck, production Vite build, changed-file ESLint with no errors,
desktop and phone task-switch E2Es (three tests each), and `git diff --check`
pass. ESLint reports five existing size/complexity warnings on touched
route/session files. The Task 05 editor-owner attribution, matched clean-browser
performance/memory capture, and non-mock single-request latency measurement
remain open. No performance gain or memory-leak conclusion is claimed.
