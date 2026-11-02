---
id: "05-audit-verification"
title: "Verify finalized audit artifacts against explicit trust"
status: pending
wave: 5
depends_on: 
  - "04-typed-reports"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-010
acceptance_criteria:
  - AC-AGENTS-SANDBOX-010.3
  - AC-AGENTS-SANDBOX-010.4
  - AC-AGENTS-SANDBOX-010.5
system_design:
  - ../../specs/agents/system-design/sandbox-evidence.md
---

# Task 05: Verify finalized audit artifacts against explicit trust

## Summary

Verify finalized artifacts with independent chain, finalization, ledger, and signer results. Persist evidence and trust provenance.

## In scope

- Operator trust-key configuration and authenticated verification action, bounded async work, provider verification, historical persistence, and UI.
- Known upstream signature fixtures plus generated disposable test keys; no private key in RPCs or public output.

## Out of scope

- No automatic signing-key provisioning, platform enrollment, remote audit ingest, or live attestation.

## Acceptance

- Unsigned, incomplete, valid-chain, wrong-key, modified-bundle, and trusted-signature results remain distinct.
- Replay with another session/artifact or trust revision cannot reuse a verified result; historical evidence survives restart.
- Desktop and phone expose the result and trust source without exposing secret material.

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

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test database DSN privately}"
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd ../kandev-plugin-nono-main && make validate)
(cd apps/backend && go test -race ./internal/agent/sandbox/... ./internal/agent/sandboxruntime/...)
make build-backend build-web
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-audit-verification.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-audit-verification.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `../kandev-plugin-nono-main/server/audit_verification.go (new)`
- `apps/backend/internal/agent/sandbox/report.go`
- `apps/backend/internal/agent/sandbox/report_http.go`
- `apps/web/components/task/sandbox-denial-report.tsx`
- `apps/web/e2e/tests/task/sandbox-audit-verification.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-sandbox-audit-verification.spec.ts (new)`

## Dependencies

04-typed-reports.

## Risks

A report-selected signer is not operator trust. Cryptographic validity does not attest the current runtime policy.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending.
