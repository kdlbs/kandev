---
id: "05-audit-verification"
title: "Verify finalized audit artifacts against explicit trust"
status: in_progress
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

Pending implementation. Pinned 0.81.0 API investigation confirms that
`nono audit verify --json --public-key-file <file> <session>` reports chain,
ledger, and attestation results separately. JSON mode can exit successfully
while result fields report a mismatch. A verifier must check the individual
fields, exact session/root/count, expected key, and captured artifact binding.
The upstream keyed verifier uses ECDSA P-256 and the audit-session DSSE
predicate. Disposable `file://` test keys are supported without changing a
system keystore. Production signing-key provisioning remains out of scope.

The existing chunk RPC has optional trust but no explicit verification request.
Add a negotiated major 2 verification request field so ordinary report reads
never start verification. Final private artifact retention must precede runtime
cleanup. Preserve its binding separately from an earlier normalized report if
the last read fails. Cleanup must not depend on a successful provider RPC.
These are implementation details within the approved verification boundary.

The first implementation checkpoint adds the explicit SDK action and observed
signer field, host response binding checks, and private artifact storage/read
primitives. Focused RED/GREEN tests cover another session or signer, untrusted
and interrupted results, post-cleanup reads, changed content, symlinks, extra
files, changed manifests, and idempotent storage. Runtime retention wiring,
operator trust and job persistence, native provider verification, and the
desktop/phone action remain in progress. This checkpoint does not complete
Task 05 or permit a trusted-audit claim.

The dedicated provider checkpoint is committed and pushed at
`8b7b3ef6a50eb222cdebc583ed891caa9e9720a5`, using public SDK foundation
`0a098c9031d62c95c305dc6149224a96fba6dce5`. It invokes the digest-checked
packaged 0.81.0 executable against private reconstructed artifacts. Separate
results cover chain, ledger, signature, and expected operator key. Signed,
untrusted, wrong-key, missing-signature, interrupted, modified-signature,
changed-action/trust, redirected-bundle, and duplicate-JSON tests pass.
Ordinary report reads do not execute verification. `make validate
verify-package` and the independent SDK consumer build pass. The pinned
upstream tests generate their signature fixtures at runtime. The checked-in
public fixture was produced by the pinned runtime with a disposable key and
records its producer, public key, and artifact digests. No private key is
committed.

Runtime retention now passes the actual packaged Nono approval/private-boundary
journey, including a second run that disables the provider before final cleanup.
The stopped runtime is removed, the old report remains stale, and its separate
final artifact receipt loads the captured metadata and bytes. Scoped session
cleanup removes the archive. SQLite receipt checks reject another attempt
binding, invalid digests, future timestamps, and non-final receipts. Full
SQLite/PostgreSQL policy/runtime/common race suites pass. Operator trust/job storage, host
verification dispatch, the desktop/phone action, and final acceptance remain
unfinished.

Operator trust storage now passes its scoped verification. Scope is derived from the
authorized captured attempt. SQLite/PostgreSQL tests cover an unconfigured
scope, public-key parsing without a Nono-specific algorithm restriction,
private-key rejection, token/read-only rejection, stale writes, explicit
revocation, immutable history, and restart. Fresh schema, replay, conflict, transaction, and timestamp conformance pass on
both engines. Previous-stable upgrades and replay pass on both engines. The
owning-store descriptor includes both trust tables. The previous-stable manifest
already records the entire sandbox store as absent, so no historical fixture
or manifest exception was changed. SQL guard and scoped lint pass. No trust HTTP route or verification job is
exposed by this storage checkpoint.

Recovery retention now passes focused real-runtime checks. New typed-report
attempts capture their bounded audit declarations. Empty declarations remain
omitted from old snapshots and hashes. Recovery retains bytes without provider
RPCs, stores an unavailable historical report with a separate artifact receipt,
and retries removal when a stopped attempt still owns runtime files. A failed
status cannot authorize removal while the supervisor or captured descendants
are alive. The stopped-retry test preserves sibling allocations and the work
directory. Full SQLite/PostgreSQL policy/runtime/common race suites and scoped
Go lint pass after these changes.

Verification jobs must still cancel and join before session archive removal.
Backend restart must interrupt pending jobs without automatically verifying
again. Job persistence, the operator API, host dispatch, and the desktop/phone
action remain required. Tasks 06-10 remain pending.

Verification-job storage is now being checked. Focused SQLite/PostgreSQL tests
cover operator-only admission, expected report/artifact/trust guards, request
idempotence, one active job per attempt, epoch/version-fenced claims and results,
session/artifact/signer/trust replay rejection, restart interruption, corrupt-row
refusal, input-key ownership, and historical preservation after key revocation.
The job table is in the owning startup descriptor. Previous-stable upgrade and
replay checks pass on both engines without changing historical fixtures. Full
policy, SQL guard, and owning-store checks pass. Runtime dispatch, cancellation
and joining, HTTP routes, facts projection, and desktop/phone UI are not yet
connected. No verifier starts from these store reads or schema initialization.
