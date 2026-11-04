---
id: "01-terminal-settlement"
title: "Reconcile interrupted terminal settlement"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-PLATFORM-DELIVERY-RETENTION-003.1
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 01: Reconcile interrupted terminal settlement

## Summary

Allow native recovery to verify a known terminal from its exact retained SQL
effect without completing settlement or mutating submission history.
Keep the normal fail-closed behavior for any missing or conflicting proof.

## In scope

- Write a failing regression with a real SQLite repository and bbolt journal:
  project a terminal, settle its outcome but interrupt before final binding,
  retain an exact interrupted-unknown journal record, and invoke native resume.
  Leave workflow settlement pending to prove recovery does not manufacture it.
- Add a read-only verifier for an entirely absent final binding backed by an
  exact settled terminal effect and projected cursor. Reuse that verifier inside
  the existing locked block-resolution transaction, after journal retirement.
- Preserve the persisted-binding path and reject partial/conflicting bindings,
  missing or superseded effects, hash/owner mismatches and independent live work.
- Cover repeat recovery and a reopened journal, explicit and loaded-native entry
  points, and stale evidence between retirement and SQL resolution.

## Out of scope

- Fabricating terminal/workflow evidence, silently resending prompts, loosening
  security guards, schema migration, and changes to the user's installation.

## Acceptance

1. Exact provable interrupted terminal history is acknowledged and the same native session
   resumes without dispatching the interrupted prompt; repeat recovery is safe.
2. Missing or conflicting evidence and independent work remain blocked, with
   original records and unrelated blocks preserved.
3. Existing queue, workflow and retention settlement invariants remain intact.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'Test.*(NativeResume|AgentDeliverySettlement|TerminalSettlement)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/native_resume_terminal.go apps/backend/internal/task/repository/sqlite/native_resume_terminal.go
(cd apps/web && pnpm e2e:run --project chromium -- tests/session/durable-stream-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/session/mobile-durable-stream-recovery.spec.ts)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/native_resume_terminal.go`
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_pending_terminal_test.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_terminal.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_pending_terminal_test.go`

## Dependencies

None.

## Risks

The incident's actual mismatch is unknown. Missing or conflicting terminal
evidence must remain blocked; workflow and queue effects remain untouched.

## Parallelism

sequential

## Inputs

- `docs/specs/platform/requirements/durable-agent-delivery.md`
- `docs/specs/platform/requirements/backend-agent-delivery-retention.md`
- Both system designs in frontmatter.

## Results

Read-only evidence recovery and locked revalidation are implemented. Real
SQLite/bbolt regression covers all three outcomes, repeat acknowledgement,
reopened storage and loaded-native recovery. Mismatched journal/SQL identity,
missing effect, projection gaps and live inventory retain the fence. SQLite and
PostgreSQL stale-proof matrices reject changed/partial bindings, wrong effects,
pruning/settlement markers and superseded terminals. Targeted tests, race checks,
storage conformance, SQL guard and scoped lint passed. All five desktop and all five mobile recovery E2E scenarios passed.
