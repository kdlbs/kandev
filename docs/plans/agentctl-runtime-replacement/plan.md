---
created: 2026-09-27
status: in_progress
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
legacy_specs: []
---

# Implementation plan: replace agentctl without restarting Kandev

## Overview

Extend PR #3598 with safe replacement of the standalone control runtime inside the healthy backend.
Baseline: `710d67fa3d33a1c95d5c280747cc729438d1ec21`, inspected on 2026-09-27 with a clean worktree.
The user explicitly requested implementation on 2026-09-27. Implement the work orders sequentially, preserve the design boundaries, and leave all changes uncommitted.

This is a new architectural increment, not a claim that #3962's Git diagnosis is proven.
The existing durable stream repair package is implemented. Preserve its fixes and evidence.
The platform owns the runtime connection; executor and session owners retain process and task authority.

## Scope

### In scope

- Bounded local runtime replacement, immutable connection epochs, consumer rebinding, and owned-child containment.
- Revisioned availability, admin retry, retained desktop/mobile state, and durable session reconciliation.
- Launched and adopted local servers, survival on/off, Windows/Linux/macOS evidence.

### Out of scope

- Arbitrary harness Git interception or a promised fix for the unconfirmed staging symptom.
- Automatic prompt resend, tool replay, conversation replacement, or full backend restart.
- Remote executor auto-restart policies and merging issue #3961 into this package.
- New feature flags, publication, commits, or persistent task creation.

## Contracts and decision

- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md): three requirements and twenty-one acceptance criteria.
- [System design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).
- [Durable delivery](../../specs/platform/system-design/durable-agent-delivery.md).
- [Earlier containment](../backend-failure-containment/plan.md) remains historical; mandatory restart behavior is explicitly superseded.

## Technical approach

Tasks 01-02 prepare the shared owner and migrate consumers while keeping startup behavior intact.
Task 03 supplies trustworthy loss/cleanup evidence. Task 04 coordinates replacement using those foundations.
Task 05 composes durable session authority. Task 06 completes API/UI integration and real crash evidence.
No intermediate milestone is releasable with partially rebound consumers.

Read the design's consumer inventory before implementation and expand it from current source.
A static count of auth-token references is insufficient; follow the clients those constructors distribute.
Do not rewrite shared config while requests run. Use immutable leases and validate their epoch before state mutation.

Runtime health and session certainty are separate. New work can proceed once the replacement is healthy,
while prior uncertain sessions remain fenced. Stop cannot claim cancellation without evidence.

## Integration with the current PR

This package can be implemented on PR #3598 before merge, as requested.
It broadens the PR materially. All package evidence must pass before treating earlier green CI as release evidence.
Keep the completed continuity, reconciliation, and stream-repair task results intact.
Link their release notes to this pending increment instead of resetting their completed tasks.

Issue #3961 is compatible but not a prerequisite. If it lands first, preserve its owner-safe port leases.
Never use port release, provisional creation cleanup, or stored numeric ports as runtime-death evidence.
Coordinate any remote executor follow-up around shared epoch/identity types without importing its restart policy.

## ASCII UI preview

UI-01: Global runtime alert above the route content; existing alert is the layout exemplar.

```text
Desktop recovering:
[Recovering agent runtime... Saved work is retained.]
[Existing route/chat remains usable]

Desktop exhausted (administrator):
[Agent runtime unavailable] [Retry agent runtime] [Restart Kandev*]
[Session: Outcome uncertain. Review before continuing.] [Stop]

Phone exhausted:
[Agent runtime unavailable]
[Saved work is retained.]
[Retry agent runtime]
[Restart Kandev*]
[Session: Outcome uncertain.]
[Stop]
```

The restart fallback appears only when authorized and supported; it is not the primary recovery action.
Non-admin users see status and administrator guidance without runtime mutation controls.
During recovery the retry action is unavailable; Stop remains session-scoped and reachable.
After runtime success the global alert clears; an unresolved session keeps its own notice.
Labels are illustrative localized copy. Requirements are hierarchy, action semantics, and preserved state.
Reuse the shared view model and in-flow alert, not a drawer or a second bottom bar.
Phone controls stack with 44px targets; desktop uses existing 28px controls.
Keep one route scroll owner, safe-area clearance, keyboard focus, and no horizontal overflow.
Use a polite status announcement while recovering and role=alert for persistent failure.
This covers AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.6 through 001.8 and 003.5.

## Tests

Each work order names proposed regressions, existing source owners, and exact commands.
Use deterministic barriers and fake clocks for concurrency and retry policy.
Use real SQL/journal stores for recovery and actual isolated processes for exit/containment tests.
Do not accept zero selected tests, skipped host tests, or cross-compilation as proof of runtime behavior.

## E2E tests

Task 06 owns Chromium and mobile-chrome crash/retry flows with real child replacement.
Run one local shard at a time. Assert backend boot continuity and no prompt resend, not just changed banner text.
Keep existing runtime-unavailable, durable recovery, survival, queue, and stream-isolation suites as regression inputs.
Native Windows and macOS process tests are release requirements; Linux-only validation leaves those gates incomplete.

## Work orders

- [x] [Task 01: Introduce a generation-fenced runtime owner](task-01-runtime-owner.md)
- [x] [Task 02: Move local runtime consumers to the shared owner](task-02-consumer-migration.md)
- [x] [Task 03: Detect runtime loss and contain only owned children](task-03-owned-exit.md)
- [x] [Task 04: Coordinate bounded replacement within the healthy backend](task-04-replacement-coordinator.md)
- [x] [Task 05: Recover durable session evidence after runtime replacement](task-05-session-reconciliation.md)
- [x] [Task 06: Expose authorized retry and prove desktop/mobile recovery](task-06-recovery-surface.md)

## Verification results

Implementation validation passed on 2026-09-27:

- W05 runtime reconciliation race suite passed across lifecycle, orchestrator, SQLite, task service, Office, and automation. SQL guard, store conformance, and backend lint passed.
- W06 backend race suite passed. Six focused web test files passed (36 tests); full web lint, TypeScript typecheck, i18n check, and i18n ratchet passed.
- Desktop Chromium E2E passed 4/4 and mobile Chrome E2E passed 3/3 using real child replacement and graceful-restart flows.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed after the results update. `git diff --check` and the modified-Go-file `gofmt -l` audit also passed.
- PostgreSQL conformance was unavailable because `KANDEV_TEST_POSTGRES_DSN` is not configured. Native Windows and macOS process-containment tests remain release gates.
- All implementation and package edits remain uncommitted. E2E runner teardown completed after the browser runs.

## Risks

- A missed client or old callback can authenticate with a retired credential or corrupt successor state.
- A live orphan with an uncertain identity must block its session, never be killed by port/PID guesswork.
- Runtime replacement can succeed while prior session outcomes remain uncertain; UI must represent both facts.
- Recovery records can outlive an interrupted backend. Candidate publication must survive that crash boundary.
- No feature flag is added. Do not ship a partial migration or treat historical checks as coverage for this increment.

## Handoff

Tasks 01-06 are implemented on the requested branch and their local Linux checks are recorded above.
Do not commit this work. Complete PostgreSQL and native Windows/macOS release gates before treating the increment as release-ready.
This package authorizes no commit, push, merge, or delegation by itself.

## Surviving-agent reattachment follow-up (2026-09-27)

The [reattachment package](../durable-agent-reattachment/plan.md) owns the five review items promised to @nova28.
Its work orders remain pending. Preserve this package's recorded results and outstanding release gates.
Live remote reattachment remains distinct from confirmed local runtime replacement.
