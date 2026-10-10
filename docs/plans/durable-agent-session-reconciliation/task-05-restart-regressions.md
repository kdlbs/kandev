---
id: "05-restart-regressions"
title: "Prove combined restart behavior"
status: complete
wave: 5
depends_on:
  - 04-recovery-guards
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 05: Prove combined restart behavior

## Summary

Add real restart evidence for the integrated adoption path and record the final delivery state.

## In scope

Add real restart evidence for the integrated adoption path and record the final delivery state.
- Extend the merged restart spec using disposable fixtures. Cover both worktree and local_pc executors.
- Use controlled mock-agent barriers and durable cursor evidence rather than sleep-based timing.
- Prove a nonzero projected/ACK cursor before restart, output during downtime, one completion, and a usable subsequent prompt.
- Prove quiet active-turn Stop and cancellation after adoption. Assert exact submission settlement and no resend.
- Add the mobile spec for restart, typed guard state, continuation eligibility, touch targets, and overflow.
- Prove survival-off plus durable-on behavior and genuine legacy adoption with an explicit old-peer fixture.
- Add journal-unavailable and owner-mismatch cases at integration level. Do not fake a legacy peer by deleting the client cache.
- Preserve instance/PID and native session identity for successful adoption. No initialize, native resume, new session, or replacement spawn is permitted.
- Use the docs-maintainer skill to update public executor/session docs. Update root/scoped AGENTS guidance and both companion plan links.
- Record actual results and dependency skips. Do not carry old test counts into the integrated result.

## Out of scope

New survival runtimes, new flags, automatic context continuation, and unrelated CI fixes.

## Acceptance

1. Worktree and local_pc restart tests prove replay, single completion, later prompting, and Stop without harness replacement.
2. Desktop/mobile recovery and survival-off/legacy/storage-error cases satisfy the existing contracts.
3. Every changed suite is covered, and documentation distinguishes automatic durability from optional process survival.

## Test cases

Required browser cases: partial-ACK restart, terminal during downtime, subsequent prompt, quiet Stop, and mobile guard/continuation precedence.
Required backend integration cases: survival disabled with v1, genuine legacy adopted peer, unknown outcome, and journal/owner failures.
The backend integration fixture must exercise real adoption and repository initialization, not only the policy function.

## Verification

Run from the repository root. Proposed test files must exist before these commands pass.

```bash
make build-web build-backend
make -C apps/backend build-agentctl build-mock-agent
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-survival-restart.spec.ts tests/session/pause-resume-recovery.spec.ts tests/session/session-resume-recovery.spec.ts tests/session/session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts tests/session/mobile-pause-resume-recovery.spec.ts tests/session/mobile-session-resume-recovery.spec.ts tests/session/mobile-session-stream-overload-isolation.spec.ts)
(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'TestDurableAdoption|Test.*Survival|Test.*Retracked|Test.*RecoveryGuard|TestReplayedTerminal' -count=1)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/session/agent-survival-restart.spec.ts`
- `apps/web/e2e/tests/session/mobile-agent-survival-restart.spec.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/internal/backendapp/agentctl_survival_test.go`
- `docs/public/executors.md`
- `docs/public/sessions-and-review.md`
- `AGENTS.md`
- `apps/backend/AGENTS.md`
- `docs/plans/durable-agent-sessions/plan.md`
- `docs/plans/agent-survival-across-restart/plan.md`

## Dependencies

Task 04 must pass its acceptance before this work begins.

## Inputs

- [Plan and baseline](plan.md).
- [Adoption contract](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
- Read the scoped AGENTS.md files and the tests next to every changed implementation.

## Risks

A passing slow-response restart is insufficient without evidence that the pre-restart cursor was nonzero.
Use the repository's existing PostgreSQL fixture for schema/SQL changes. Record its command and real outcome.
Run any additionally changed suites once in their documented package scope.

## Parallelism

`sequential`

## Results

Completed 2026-09-14. Added worktree and local executor restart coverage plus
the mobile survival case. The desktop integration command passed 7/7 tests.
The mobile command passed 7/7 tests on the final run, including restart,
delayed cancellation, failed resume, branch recovery, and stream overload
isolation. Focused desktop and mobile overload reruns passed after canonical
durable publications were routed through the existing coalescer.

The backend restart and lifecycle race checks, store conformance, SQL guard,
web/backend builds, specification validation, all 36 specification linter
tests, full specification lint, and whitespace checks passed. Public executor
and session documentation plus root, backend, and frontend agent guidance now
state that durable delivery is automatic for compatible retained storage and
is separate from process survival. PostgreSQL, Docker, SSH, Kind,
retained-executor replacement, and live harness matrix checks were not run
because their external dependencies were unavailable.
