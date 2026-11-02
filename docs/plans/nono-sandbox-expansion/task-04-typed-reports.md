---
id: "04-typed-reports"
title: "Expose network and command audit reports"
status: done
wave: 4
depends_on: 
  - "03-runtime-facts"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-010
acceptance_criteria:
  - AC-AGENTS-SANDBOX-010.1
  - AC-AGENTS-SANDBOX-010.2
  - AC-AGENTS-SANDBOX-010.5
system_design:
  - ../../specs/agents/system-design/sandbox-evidence.md
---

# Task 04: Expose network and command audit reports

## Summary

Parse the available event classes and show independent coverage/integrity/finalization with exact typed targets.

## In scope

- Nono audit variants, bounded chunk collection, source provenance, live/final handling, report persistence and UI.
- Do not infer exact targets from redacted output; preserve last valid report on failure.

## Out of scope

- No new grants, no policy publication, no signature trust claim.

## Acceptance

- Real runtime records appear under the correct kinds; missing classes remain unsupported or unavailable.
- Corruption, replacement, chunk gaps, partial final lines, oversized reports, and refresh failure preserve trustworthy previous evidence.
- Desktop and phone can filter, inspect long targets, and see why a record cannot be learned.

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
(cd apps/web && pnpm e2e:run e2e/tests/task/sandbox-reports.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run e2e/tests/task/mobile-sandbox-reports.spec.ts --project=mobile-chrome)
```

Also run `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)`. Add targeted Vitest coverage for changed business logic. Use the existing Traditional Chinese generation command. Install workspace dependencies from `apps/` if this worktree lacks them.

## Files likely touched

- `../kandev-plugin-nono-main/server/reports.go`
- `../kandev-plugin-nono-main/server/reports_test.go`
- `apps/backend/internal/agent/sandboxruntime/report_source.go`
- `apps/backend/internal/agent/sandbox/report_validation.go`
- `apps/web/components/task/sandbox-denial-report.tsx`

## Dependencies

03-runtime-facts.

## Risks

Nono proxy events can be finalized later than live capability events. A valid chain is not a complete observation of all denied activity.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

In progress. The first contract/storage milestone adds:

- Separate report-schema negotiation, session/source-bound chunks, and closed reason codes in the standalone SDK. Existing field numbers and major 1 behavior remain unchanged.
- A core-owned frozen archive reader with 32-file/32 MiB collection limits, 256 KiB chunks, deterministic source ordering, append-prefix and file-birth checks, and private cleanup. Finalization metadata uses the generic major 2 `audit_final` declaration.
- Host validation for partial inspection-only identities, exact source binding, empty existing files, and the unchanged complete-operation checks used by approvals and policy grants.

RED/GREEN receipts: `/tmp/nono-audit-source-schema-red.log`, `/tmp/nono-report-capability-red.log`, `/tmp/nono-audit-reason-schema-red.log`, `/tmp/nono-audit-archive-red.log`, `/tmp/nono-audit-alias-red.log`, `/tmp/nono-final-artifact-declaration-red.log`, `/tmp/nono-chunk-binding-red.log`, and `/tmp/nono-partial-audit-red.log`. Current common-sandbox, full packaged-plugin/fixture, SDK, and targeted plugin race tests pass. The existing actual Nono package launch/ownership scenario also passes. Generated protobuf files are deterministic, scoped Go lint reports zero issues, and specification/public-doc checks pass. The archive implementation requires Linux statx birth identity and fails closed when that support is unavailable.

The Nono streaming reader now handles recorded filesystem, TCP, command, and HTTP-endpoint decisions. It validates source/chunk/session bindings, event chains, and final session metadata. Unknown variants fail explicitly. Redacted hosts remain absent, raw command arguments and endpoint query values are excluded, and pending command invocation outcomes are not presented as completed work. Every class retains partial coverage.

The host freezes the complete bounded input before dispatch, uses the negotiated report schema, persists typed data in the unchanged report envelope, and keeps the original successful-read time after failures. SQLite and PostgreSQL round trips, restart, stale version rejection, and major 1 refusal pass. A failed final read changes live collection to interrupted without changing the earlier report. Exact filesystem denials retain existing learning, including read/write access mapped to the existing writable grant. Other kinds remain inspection-only until their policy work orders.

The shared report view includes kind/decision filters, long-target details, independent coverage/chain/finalization labels, stale and missing-history states, and explicit learning blockers. Reports and policy review use one dialog shell. Phone navigation and the review footer stay fixed around one scroller. Legacy denial hiding, publication, and focus return remain supported. All locale catalogs are updated. Final desktop/phone delivery checks pass. Normal commit hooks and push are next.

Current receipts: dedicated plugin `make validate verify-package` passes with the real pinned 0.81.0 binary. Full host policy/runtime race suites pass with SQLite/PostgreSQL and the packaged plugin. Common sandbox and full plugin race suites pass. The 21 focused frontend tests, typecheck, localization, and zero-issue scoped Go/frontend lint pass. Desktop/phone report and learning flows pass; final captures additionally check 768 px fine-pointer layout and fixed-header geometry. A cropped element screenshot was a capture side effect. Viewport-clipped captures preserve the correct header. Task 05 signature verification, broader approval classes, and current-pair managed acceptance remain separate gates.


Final delivery receipts (2026-11-02): desktop reports/learning pass 3 tests in
23.7 seconds (`/tmp/kandev-run.e2e.dz9DvU7O.log`); phone reports/learning pass
3 tests in 20.4 seconds (`/tmp/kandev-run.e2e.c79BZQ0H.log`). They include
393/700/767 px phone layouts, 768 px fine-pointer layout, fixed-header geometry,
first-read failure without invented history, stale evidence, filtering, exact
selection, hiding/restoring, and publication without an implicit binding write.
Final desktop and phone screenshots were inspected. The report and learning
surfaces use one shell and one scroller. SQL guard and full store conformance
passed with PostgreSQL enabled. Typecheck, all locale checks, the new-code
translation ratchet, public docs, catalog validation, and specification lint
passed. A final typed read/write denial regression passes in Go and TypeScript;
the packaged Nono approval/report/cleanup test passes after that fix.
