---
created: 2026-10-03
status: complete
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-stream-continuity.md
legacy_specs: []
---

# Fix plan: Workspace stream continuity

## Overview

Repair dropped workspace events after a workspace-only runtime becomes an agent runtime.
The same stream remains attached, but its callbacks currently retain an obsolete ACP startup generation.
Two sequential work orders cover the minimal correction and integrated delivery evidence.
Implementation was authorized from this reviewed package. This package does not authorize delegation,
commits, or deployment.

## Evidence and timing

Affected task: `4f49965f-becb-47bb-8972-d9ea13871cd5`.
Inspected checkout: `546bef5183c4086c089b362c36c3a0287747bf59`.
Deployed revision: `59687fafa4f151512e666dc8f762f5c2fb10cc89`.
Relevant workspace callback and startup-generation code matches between those revisions.
Local diagnostic evidence is `.kandev/diagnostics/changes-loading-timings.json`.
The following timeline uses UTC on 2026-10-03.

| Time or interval | Source evidence |
| --- | --- |
| 13:40:23.635 | Workspace-only runtime connects its stream |
| 13:40:25.484 | The same execution is promoted to an agent runtime |
| 13:40:26.534 | Workspace stream is already attached, so connection is skipped |
| 13:54:12.403 | Focus activates fast polling for the affected session |
| 1.077 seconds | Initial explicit Git refresh request duration |
| 1.124 to 1.573 seconds | Initial cumulative diff, PR files, and commit requests |
| 35.680 seconds | Layout restoration to the next observed populated Git status |
| 1.878 seconds | Later foreground refresh request to populated status delivery |

Fast polling worked. Subsequent slow-mode messages belonged to other sessions.
Git event publication stopped after promotion, while explicit reads still returned snapshots.
The 35.680-second interval measures time until observed recovery, not Git computation or an exact spinner duration.
The evidence does not establish that contention caused that interval.

A temporary `TestReproWorkspaceGitStatusAfterAgentPromotion` reproduced the callback loss without polling or Git computation.
It attached callbacks, advanced startup generation, reused the attached stream, and delivered another status.
The original callback discarded that status. Newly constructed control callbacks forwarded it.
The regression failed in 0.017 seconds and the temporary test was removed.

Diagnostic command, from `apps/backend`:

```bash
go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestReproWorkspaceGitStatusAfterAgentPromotion$' -count=1 -v
```

## Requirement reconciliation and assumptions

Platform already owns accepted publication, stale-source rejection, and focus-based polling in the
[workspace Git status requirement](../../specs/platform/requirements/workspace-git-status.md).
Criterion `.43` makes same-runtime promotion continuity explicit. Criteria `.2`, `.20`, `.27`, `.29`, and `.31` remain binding.
Tasks retains canonical environment identity and execution ownership. No duplicate Tasks requirement is necessary.

Confirmed scope: restore live delivery after promotion and preserve current stale-runtime protection.
Verified facts: startup generation changes while execution/client identity and the attached workspace stream remain the same.
No material product choice remains unresolved.
The [design supplement](../../specs/platform/system-design/workspace-stream-continuity.md) restores the existing lifetime distinction without a new ADR.

## Scope and technical approach

Change the common workspace forwarding closure in `streams.go` to use the existing captured-client lease independently of ACP startup.
Keep current execution rejection, source ordering, stream reuse, retry/drain behavior, and ACP callback fencing intact.
Cover every workspace callback channel, not only Git status.
Do not add an ad hoc Git-only bypass or reconnect the workspace stream at every startup.

Excluded: polling cadence changes, larger request budgets, contention tuning, tracker redesign, new instrumentation systems, and UI changes.
Existing recovery remains useful for transport loss. It cannot serve as the primary delivery path after promotion.

## Companion packages

- [Changes panel Git refresh](../changes-panel-git-refresh/plan.md) owns progressive producer/cache publication and correlated recovery.
- [Changes loading feedback](../changes-loading-feedback/plan.md) owns toolbar presentation and delayed automatic retry.
- [Exact dirty-path monitor refresh](../workspace-dirty-path-monitor/plan.md) owns filename parsing and monitor detection.
- [Completion callback lease](../completion-callback-lease/plan.md) owns ACP completion lease safety.

This package changes none of those mechanisms. Reconcile shared `streams.go` edits without reverting their source guards.

## Tests and E2E evidence

| Criteria | Evidence |
| --- | --- |
| `.43` | Original attached callbacks still forward all workspace channels after first and repeated ACP startup |
| `.27`, `.31` | Replacement/detachment leases, retired execution rejection, startup fencing, and shutdown/drain regressions |
| `.2`, `.20`, `.43` | Real workspace WebSocket delivery reaches the manager publisher after generation advances |
| `.29`, `.31`, `.43` | Desktop and phone Changes receive real post-promotion Git membership and settled detail while focus remains active |

Task 01 supplies the behavioral RED gate before production changes.
Task 02 verifies the resulting transport and UI behavior against fresh managed builds. Its browser
scenario starts a prepared CREATED session because completed-session resume creates a new runtime
execution and cannot prove same-execution promotion continuity. The completed workspace restoration
test remains in the desktop regression run.
Browser evidence must observe the actual stream, not a synthetic status response or direct store update.
Observe request counts so a later explicit/recovery read cannot mask dropped stream delivery.
Use bounded causal waits and barriers. Do not encode the production incident's elapsed duration as a test threshold.

## Mobile and UI assessment

Desktop entry remains task Changes. Phone entry remains bottom navigation to Changes, as in `mobile-changes-panel.spec.ts`.
Both use shared environment state and existing diff navigation.
There is no rendered composition, copy, touch, or layout change, so no ASCII UI preview is required.
Focused desktop and phone browser checks prove the same restored live-delivery outcome.

## Work orders

- [x] [Task 01: Correct workspace callback lifetime](task-01-callback-lifetime.md)
- [x] [Task 02: Prove promotion delivery in Changes](task-02-promotion-delivery.md)

Run these orders sequentially in the primary session. Task 02 depends on Task 01.

## Verification results

- Investigation reproduction: expected callback-loss failure, as recorded above.
- Catalog validation: passed (346 decisions and 1328 specifications).
- Specification linter tests: passed (36 tests).
- Full specification lint: passed.
- Local PR coverage preflight: covered, both work orders, no reference errors.
  The preflight included the proposed `streams.go` edit to exercise implementation coverage.
- Local Markdown links and `git diff --check`: passed.
- Runtime change preserves captured agentctl-client fencing while removing the obsolete ACP startup-generation check from common workspace callbacks.
- Targeted callback-lifetime and transport-to-publisher tests passed under the race detector. The full lifecycle race suite passed in 122.582 seconds after adding the transport fixture.
- Desktop prepared-session promotion and completed-workspace restoration passed with the host-mode managed E2E runner. The phone promotion and Changes suite passed all 10 tests. Final focused promotion reruns passed on both viewports.
- Web typecheck, targeted ESLint, and fresh E2E backend/frontend builds passed.
- Specification catalog validation, linter tests, full spec lint, Markdown link validation, and `git diff --check` passed.

## Documentation impact

Internal requirements, system design, and delivery records only.
This repair restores existing live updates without changing operator procedures, settings, wire formats, or UI terminology.
Public documentation requires no change for this package.

## Risks

- Removing the client lease with the startup lease can admit retired callbacks.
- A direct manager-handler test can bypass the failed stream wiring.
- Browser recovery reads can conceal the dropped-notification defect.
- Broad startup-lock changes can regress ACP attempt attribution or completion safety.
- The latency evidence supports this delivery defect, not a general contention or enrichment-performance claim.
