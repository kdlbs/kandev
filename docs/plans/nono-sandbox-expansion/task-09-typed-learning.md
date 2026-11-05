---
id: "09-typed-learning"
title: "Extend explicit policy learning to supported exact operations"
status: pending
wave: 9
depends_on: 
  - "08-endpoint-approvals"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-010
  - REQ-AGENTS-SANDBOX-011
acceptance_criteria:
  - AC-AGENTS-SANDBOX-010.2
  - AC-AGENTS-SANDBOX-010.5
  - AC-AGENTS-SANDBOX-011.1
  - AC-AGENTS-SANDBOX-011.3
  - AC-AGENTS-SANDBOX-011.6
  - AC-AGENTS-SANDBOX-011.7
system_design:
  - ../../specs/agents/system-design/sandbox-evidence.md
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
---

# Task 09: Extend explicit policy learning to supported exact operations

## Summary

Turn selected exact reports into reviewed typed additions without confusing static permissions and approval eligibility.

## In scope

- Typed proposal/diff/persistence, source and trust provenance, ceiling/task-guard checks, operator publication, separate adoption.
- Settings migration review and desktop/phone learning view.

## Out of scope

- No automatic learning or adoption, guessed redacted targets, or reclassification of old permission records.

## Acceptance

- Exact supported records produce minimal proposals; incomplete targets and stale ceilings refuse publication.
- Publication survives restart with provenance and changes neither current attempts nor selected defaults.
- Both viewports show static-versus-ask differences and explicit persistence scope.

## UI preview

Native structured preview per the task instruction. [Full composition and states](plan.md#ui-previews).

### UI-02: Reports and audit

Entry: Execution details / Reports. AC-AGENTS-SANDBOX-010.1 through .5.

| Order | Desktop dialog | Phone full-height next view |
| --- | --- | --- |
| Fixed header | Reports / Close | Back / Reports |
| Summary | Coverage by kind; last successful read | Stacked coverage and age labels |
| Filter | Kind selector and decision filter | Touch picker, same shared filter |
| Body | Event rows with exact scope | Wrapping event cards, visible details action |
| Review | Selected exact records / Review additions | Same action in fixed safe-area footer |
| Verification | Final record / Verify audit / result details | Next view with one scroller and Verify audit |

Unsigned, incomplete, invalid signature, untrusted signer, and valid signature are separate states. A failed refresh preserves old rows with a stale banner. Incomplete/redacted identities explain why learning is disabled.

The learning review also shows two explicit choices: Add static access or Allow asking. Neither is pre-published.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test database DSN privately}"
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd apps/backend && go test -race ./internal/common/sandbox/... ./internal/agent/sandbox/... ./internal/agent/sandboxruntime/...)
(cd ../kandev-plugin-nono-main && make validate)
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-learning.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-learning.spec.ts --project=mobile-chrome)
(cd apps/web && pnpm e2e:run e2e/tests/settings/sandbox-executor-settings.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/settings/mobile-sandbox-executor-settings.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `apps/backend/internal/agent/sandbox/learning_diff.go`
- `apps/backend/internal/agent/sandbox/learning_publish.go`
- `apps/backend/internal/agent/sandbox/task_admission.go`
- `../kandev-plugin-nono-main/server/proposal.go`
- `apps/web/components/task/sandbox-learning-review.tsx`
- `apps/web/components/settings/sandbox-policy-create.tsx`

## Dependencies

08-endpoint-approvals.

## Risks

A suggestion to ask for approval is not a static grant. Legacy ceilings must not become new expansion envelopes without review.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending.
