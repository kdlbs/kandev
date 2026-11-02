---
id: "03-runtime-facts"
title: "Show separate process and launch evidence"
status: done
wave: 3
depends_on: 
  - "02-provider-protocol"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-009
acceptance_criteria:
  - AC-AGENTS-SANDBOX-009.1
  - AC-AGENTS-SANDBOX-009.2
  - AC-AGENTS-SANDBOX-009.3
  - AC-AGENTS-SANDBOX-009.4
system_design:
  - ../../specs/agents/system-design/sandbox-evidence.md
---

# Task 03: Show separate process and launch evidence

## Summary

Persist and render independent runtime facts, using core liveness and historical launch checks without weakening active-confinement validation.

## In scope

- Observation/persistence/API/store schema, source timestamps, owner invalidation, UI groups, and provider runtime metadata parsing.
- Neutral indicator behavior, localization, responsive state preservation, and legacy-row decoding.

## Out of scope

- No new protection claim, policy change, audit signing, or task lifecycle transitions.

## Acceptance

- Running and launch-passed facts are useful while confinement remains unknown when evidence is absent.
- Restart, disconnect, stale/sibling events, and provider loss preserve history without retaining a live claim.
- Desktop and phone render accessible independent facts and preserve focus/scroll behavior.

## UI preview

Native structured preview per the task instruction. [Full composition and states](plan.md#ui-previews).

### UI-01: Execution evidence

Entry: selected session's Execution details. AC-AGENTS-SANDBOX-009.1 through .4.

| Order | Desktop dialog | Phone full-height view |
| --- | --- | --- |
| Fixed header | Execution details / Close | Back / Execution details |
| First group | Process: running, checked time | Same fact and time, full-width row |
| Second group | Launch checks: passed, launch time | Expand named checks in place |
| Third group | Runtime: reported facts / limitations | Same facts with wrapped labels |
| Fourth group | Audit: live/partial/unsigned or verified | Open audit details as next view |
| Final group | Captured policy / next launch | Same sections in one scroller |

The neutral tab indicator remains when live confinement is unverified. Loading and unavailable facts retain their labels. Disconnect makes live process data stale while launch history remains. No success color implies stronger evidence.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test database DSN privately}"
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd apps/backend && go test -race ./internal/agent/sandbox/... ./internal/agent/sandboxruntime/... ./internal/task/models/...)
(cd ../kandev-plugin-nono-main && make validate)
(cd apps/web && pnpm exec vitest run lib/execution-status.test.ts lib/ws/handlers/session-execution-snapshot.test.ts)
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/execution-details.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-execution-details.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `apps/backend/internal/common/sandbox/observation.go`
- `apps/backend/internal/agent/sandboxruntime/evidence.go`
- `apps/backend/internal/agent/sandbox/attempt_observer.go`
- `apps/backend/internal/task/repository/`
- `apps/web/lib/types/execution.ts`
- `apps/web/components/task/session-execution-body.tsx`
- `apps/web/lib/execution-status.ts`
- `../kandev-plugin-nono-main/server/status.go`

## Dependencies

02-provider-protocol.

## Risks

A runtime event emitted once must not gain a new observation timestamp on each read. SQLite and PostgreSQL need equivalent persistence and invalidation checks. The optional facts use existing JSON columns, so this change requires no new DDL.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Implemented in the primary session on 2026-11-02. All Task 03 delivery checks passed.

- Added optional versioned facts to the existing observation JSON. SQLite and PostgreSQL round trips cover receipt binding, expiry, backend-owner replacement, failed refresh, and legacy observations. No DDL is required.
- Core process ownership remains useful during provider loss. `VerifyLive` retains the policy/provider checks required for approvals and active confinement. New launch receipts name preparation checks and bind policy, package, and runtime digests. Old attempts have no invented receipt.
- Nono major 2 reports conditional runtime metadata from the captured session's verified audit chain. Original timestamps remain unchanged. Missing events, optional fields, siblings, malformed values, and tampering have dedicated tests. The actual packaged default launch reports missing metadata without claiming failed confinement.
- Added localized Process, Launch checks, Runtime observations, and Audit groups in the existing desktop and phone view. Live facts expire on disconnect/deadline. Historical receipts and metadata survive. The shared clock schedules process expiry even when confinement is unknown.
- Passed current-package race tests for common sandbox, policy, runtime, and task models; SQLite/PostgreSQL execution tests; 62 browser-state tests; actual-runtime plugin validation and packaging; independent public SDK consumer build; typecheck; seven-language/pseudo copy checks; zero-issue scoped Go/frontend lint; docs validation; and whitespace checks.
- Rendered desktop and phone scenarios passed with retries disabled. They cover a non-active session, stale liveness, historical checks, keyboard/focus return, 393/700/767px phone layouts, and 767/768px fine-pointer transitions. Screenshots were inspected. The final run after the test fixture timestamp correction passed both projects with retries disabled.

Local test setup required a loopback-only bind and the existing Nix browser library paths. Neither changes the main instance or product configuration. Current-pair managed acceptance remains Task 10.
