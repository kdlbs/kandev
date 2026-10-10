---
id: "03-visible-recovery"
title: "Present and verify missing-record recovery"
status: done
wave: 3
depends_on:
  - "02-atomic-reconstruction"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.7
system_design:
  - ../../specs/platform/system-design/durable-agent-record-recovery.md
---

# Task 03: Present and verify missing-record recovery

## Summary

Show the durable missing-record notice before a complete continuation identity
exists. Prove that Retry repairs records without sending a prompt, then permits
only eligible explicit continuation on desktop and phone.

## In scope

- Extend the shared recovery view model with the persisted incomplete-identity
  notice. Fence requests by block identity before reconstruction and by the
  committed recovery identity afterward.
- Render the saved rejected instruction's blocked delivery state. Keep its text
  visible. Retry must not silently submit it or pre-check acknowledgment.
- Reuse existing Retry, Stop, progress status, and inline continuation controls.
  Preserve independent errors and late-response protection from PR #4380.
- Localize new reasons in all seven catalogs. Generate Traditional Chinese
  variants with the existing script and keep pseudo-locale coverage current.
- Add a disposable real journal/SQL E2E fixture for the incident shape. Do not
  implement the fixture by seeding only an error banner or recovery metadata.
- Extend the public session recovery guide after behavior passes. Its primary
  content type is a how-to guide. Explain record repair, explicit continuation,
  and remaining ownership blocks in user language.

## Out of scope

New navigation, overlays, a separate recovery dashboard, production repair,
automatic retries, and changes to PR #4380's acknowledgment requirement.

## Acceptance

1. Desktop and phone show the persisted blocked message and actionable recovery
   cause before and after reload. Progress and terminal results are accessible.
2. The real Retry route reconstructs one record with zero provider prompts.
   Duplicate Retry and backend restart preserve that result without duplicate history.
3. An eligible explicit continuation sends one new instruction in the original
   native conversation. Ambiguous evidence, incomplete process proof, and stale
   responses remain blocked without changing another session.

## ASCII UI preview

UI-01: Same card and states as the [combined preview](plan.md#ascii-ui-preview).

```text
Desktop:
  Message saved. Delivery is blocked.
  [Retry connection] [Stop]
  Status: Inspecting retained records...

Phone, eligible result:
  Records restored. Previous outcome unknown.
  Next instruction:
  [                                    ]
  [ ] Acknowledge uncertainty
  [Resume session]
  [Stop]
```

The chat owns scrolling. Phone actions stack with at least 44px targets.
Desktop controls retain the standard 28px size. Use safe-area clearance and
keep Resume reachable when the keyboard opens. No second scroll surface is added.
When proof is missing, show its cause instead of the continuation form.
This preview maps to AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5 and 008.6.

## Verification

Run from the repository root. Install workspace dependencies once when absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-agent-delivery-recovery.test.ts lib/services/session-recovery-service.test.ts hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-revision.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-record-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-record-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the two managed browser selections sequentially. Use a fresh build after
production changes. Confirm discovery of the intended tests and inspect a
rendered phone screenshot. Do not overlap full E2E suites.

Add `TestDeliveryRecordRecoveryFixture` alongside the new backend fixture and run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'TestDeliveryRecordRecoveryFixture' -count=1)
```

The browser tests must capture prompt counters before Retry. Assert the result
through canonical API state and rendered UI after reload. After explicit
continuation, assert one new provider acceptance and the same native ID.
Include real backend restart between retries. Include a blocked fixture without
process proof, plus session navigation while an old response is delayed.
Use the mock provider only for deterministic identity and dispatch evidence.
Real provider continuation remains a separately reported compatibility limit.

The missing-record fixture intentionally has incomplete process identity, so
its repaired session must remain blocked. The eligible explicit-continuation
path is covered by `layout/agent-runtime-replacement.spec.ts` and
`layout/mobile-agent-runtime-replacement.spec.ts`; both assert one accepted
instruction while retaining the native conversation identity.

## Files likely touched

- `apps/web/lib/session-agent-delivery-recovery.ts` and its test
- `apps/web/lib/services/session-recovery-service.ts` and its test
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts` and its test
- `apps/web/hooks/domains/session/use-session-recovery-revision.ts` and its test
- `apps/web/components/task/chat/session-recovery-context.tsx`
- `apps/web/components/task/recovery-actions.tsx`
- `apps/web/src/locales/` affected task catalogs and generated variants
- `apps/web/e2e/tests/session/durable-record-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-durable-record-recovery.spec.ts` (new)
- `apps/web/e2e/helpers/delivery-record-recovery.ts` (new)
- `apps/backend/internal/backendapp/e2e_delivery_record_recovery.go` (new)
- `apps/backend/internal/backendapp/e2e_delivery_record_recovery_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/e2e_delivery_record_fixture_test.go` (new, if the runtime fixture requires it)
- `docs/public/sessions-and-review.md`
- This package's verification results and status fields

Inspect PR #4380's current fixture wiring before selecting its exact integration
point. Keep the new fixture under existing E2E-only admission and cleanup rules.
Search `README.md`, `docs/screenshots.md`, and related recovery guides for stale
claims. Change them only when the implementation changes those claims.

## Dependencies

Tasks 01 and 02, plus PR #4380's recovery controls and interruption request fencing.

## Risks

An unbound block cannot use a fabricated submission ID as a request key.
Do not retain stale acknowledgment, instruction, or success state across
different recovery episodes. Fixture-only UI assertions can hide a broken import.

## Parallelism

`sequential`

## Inputs

- [Design: presentation and compatibility](../../specs/platform/system-design/durable-agent-record-recovery.md#presentation-and-compatibility).
- PR #4380's `recovery-actions.tsx`, recovery services, and mobile runtime tests.
- Existing `durable-stream-recovery.spec.ts` and `mobile-durable-stream-recovery.spec.ts`.
- Mobile UI language, control sizing, E2E fixture, and public-documentation guidance.

## Results

Implemented the recovery notice, blocked saved-message state, request fencing,
accessible retry progress, and localized ambiguity/incomplete-identity reasons.
Retry reconstructs one canonical association without a provider prompt; the
saved user message remains visible and blocked. An incomplete process identity
stays blocked after reload and backend restart. Duplicate retry preserves one
row and one user message. Ambiguous journal evidence remains blocked. A delayed
response cannot update the next selected session.

The disposable SQL/journal fixture drove these checks through the real
`session.recover` route on desktop and phone. The eligible explicit-continuation
companion checks passed on both device projects and verified one new accepted
instruction with the same native conversation ID. The phone controls remain
reachable after the longer persisted notice and meet the 44px target minimum.
The rendered phone state was inspected. Real-provider continuation remains
outside this mock-provider verification.

Validation passed: 7 focused Vitest files / 102 tests; `pnpm run typecheck`,
`pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`; the
production-mode pseudo-locale build; lifecycle fixture and focused DTO/message
handler Go tests; both recovery browser specs; both eligible-continuation
companion browser specs; public documentation validation (63 validator tests
and 47 pages); and specification validation/lint (371 decisions and 1520
specifications). `git diff --check` passed.

### Review remediation

Final browser verification passed three tests on Chromium and three on
mobile Chrome. Both projects covered missing-record reconstruction, duplicate
Retry, reload, backend restart, ambiguous evidence, delayed responses after
navigation, and a live recovery block without reloading the chat. The runtime
replacement companion verified one new instruction in the original native
conversation. All eight desktop/phone captures were inspected.

The final focused frontend run passed 132 tests in eight files. Typecheck and
focused ESLint passed. Live session events carry recovery blocks and retain
nanosecond timestamp precision; an older snapshot cannot restore a cleared
block. An omitted block projection preserves the browser's existing state.

Commands: `CAPTURE_PR_ASSETS=1 GOMAXPROCS=2 scripts/run-quiet e2e --summary --
pnpm --dir apps/web e2e:run --host --project chromium
 e2e/tests/session/durable-record-recovery.spec.ts
 e2e/tests/layout/agent-runtime-replacement.spec.ts`, then the equivalent
`mobile-chrome` command with both `mobile-` specs. Each managed command rebuilt
the backend and frontend. No real-provider compatibility claim is made.

### PR review remediation

A lost continuation reply no longer discards the original request when the
same interruption advances its revision. The browser retains its instruction
and idempotency key for server-side lookup; another interruption cannot reuse
it. The regression failed before the fix and passed afterward.

The final service, batch-hook, and continuation-component run passed ten tests
in three files with `pnpm exec vitest run lib/services/interrupted-session-recovery.test.ts hooks/domains/session/use-interrupted-recovery-batch.test.ts components/task/chat/interrupted-session-continuation.test.tsx`.
Typecheck and focused ESLint passed. Both managed browser commands recorded
above were rerun and passed all six tests. The remediation changes recovery
behavior, not rendered markup, so the published viewport captures remain valid.
Remote CI and review disposition remain delivery work.

### Main conflict and CI remediation

Integrated the published native-resume retirement contract from main. Retirement
keeps the idle-dispatch guard and exact current owner checks, permits same-generation
acknowledgement only for interrupted-unknown work, and retains unfinished evidence
under successor retirement. Published acceptance IDs remain stable; branch-only
criteria now use 006.15 through 006.18.

Canonical interrupted-prompt blocks without a runtime recovery record retain the
existing Resume action. Runtime recovery records retain state-only Retry and
explicit continuation. Updated disconnect browser assertions to check the specific
Retry result rather than an obsolete error. The incoming process test fixture now
provides the logger required by delivery-pressure updates.

Post-integration verification passed:

- Four backend packages with race detection: journal, process, lifecycle, and
  orchestrator. Coverage includes both native Resume and explicit continuation,
  owner fencing, read-lock replacement, and unfinished-evidence retirement.
- Four frontend test files, 54 tests; web typecheck and focused ESLint.
- Sixteen focused browser checks: seven desktop recovery/order checks, five phone
  recovery checks, two desktop cancellation checks, and two phone cancellation
  and failed-resume checks. Every check passed on its first attempt.
- Full backend changed-scope lint against the integrated main revision (zero
  issues). The first run exceeded its time budget; the warmed-cache rerun passed.
- Documentation/specification validators and whitespace/conflict scans.

Exact-head remote CI and review remain externally pending until after push.

### Terminal projection follow-up

The final integration audit reproduced a missing terminal projection with no
runtime recovery metadata, and a stale session timestamp after terminal SQL block
resolution. Regression tests failed before the correction. Terminal settlement
now advances the session fence transactionally and publishes the cleared block
array independently of runtime metadata. Open canonical blocks keep the composer
in recovery mode without requiring a separate error message. Cancellation browser
coverage now sends a new instruction afterward to verify fresh admission.

The stricter native Resume regression also reproduced the missing block update.
Explicit resolution now publishes the committed state before releasing queue
work. The regression checks the session-state event and its non-null empty block
array, separately from the queue-status event.

Follow-up verification passed:

- Focused orchestrator recovery and settlement tests with race detection.
- SQLite and real PostgreSQL terminal revision tests, plus full store conformance
  under race detection with both databases.
- SQL guard and full backend changed-scope lint (zero issues).
- Four frontend files with 54 tests, then six composer tests after extraction;
  web typecheck and focused ESLint with zero warnings.
- Three rebuilt desktop and four phone browser checks on their first attempts.
  Coverage includes native Resume followed by a new instruction, cancellation
  followed by a new instruction, and durable reattachment.

Remote checks remain pending for the next pushed head.
