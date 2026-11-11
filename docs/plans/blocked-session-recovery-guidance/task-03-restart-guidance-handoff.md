---
id: "03-restart-guidance-handoff"
title: "Share Restart recovery guidance"
status: in_progress
wave: 3
depends_on:
  - "02-uncertain-delivery-actions"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-SESSION-TAB-RESTART-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-TASKS-SESSION-TAB-RESTART-001.2
  - AC-TASKS-SESSION-TAB-RESTART-001.3
  - AC-TASKS-SESSION-TAB-RESTART-001.4
  - AC-TASKS-SESSION-TAB-RESTART-001.5
  - AC-TASKS-SESSION-TAB-RESTART-001.6
system_design:
  - ../../specs/agents/system-design/session-recovery-guidance.md
  - ../../specs/tasks/system-design/session-tab-restart.md
---

# Task 03: Share Restart recovery guidance

## Summary

Carry Restart's typed launch failure to the captured session's existing recovery owner.
Retain current guidance across menu unmount and failed continuation without changing Restart's Stop-then-Resume sequence.

## In scope

- One store-scoped continuation-guidance model, shared by Restart and ordinary recovery.
- A request-local active recovery model while matching durable projection is delayed.
- Incident, operation, generation, navigation, and store-owner fencing.
- Desktop and phone menu-to-card regressions and public recovery instructions.
- A real-backend repeated-interruption scenario after issue 5256's repair is integrated.

## Out of scope

New lifecycle APIs, automatic continuation, a second durable error store, or changes to native retirement proof.
Do not implement issue 5256 inside this work order.

## Acceptance

1. After typed Restart failure, the captured session offers continuation after menu unmount. A failed same-incident continuation keeps its hint without automatic retry.
2. Stale replies, successor incidents, independent stores, and later user navigation retain their own state. Drafts, pending guards, selection pinning, and Stop failure ordering remain unchanged.
3. Desktop and phone pass routing and geometry checks. After issue 5256 integration, the real-backend scenario proves bootstrap and one distinct new prompt without replaying the interrupted submission.

## ASCII UI preview

UI-02 and UI-03 from the [combined preview](plan.md#ascii-ui-preview).
UI-02 maps to Tasks 001.2-.5 and Agents 006.5, .9, .16-.17.

```text
Desktop: tab context menu > Restart session > menu closes
Phone:   Sessions > visible row actions > Restart session

Request feedback: Session restart failed

Captured session's recovery region:
+------------------------------------+
| Recovery requires saved history    |
| New native conversation. The old   |
| prompt outcome remains unknown.    |
| [Continue from saved history     ] |
| [Other eligible actions          ] |
+------------------------------------+

Pending: equivalent actions disabled, accessible status
Failed:  failure stays visible, explicit action remains
```

Desktop uses UI-01's wrapping row. Phone uses its stacked full-width controls.
The selected session's recovery region owns the action, not a new toast-only interaction or additional overlay.
A later explicit user selection keeps focus and selection.

## Regression tests

First write a hook/provider regression named `Restart publishes continuation guidance after its menu unmounts`.
The current implementation must fail because it only updates the toast.
Mount separate Restart and recovery consumers against the same real application-store owner.
Close the menu consumer before rejecting the captured launch request with a typed hint.
Assert that the remaining active owner exposes the action without sending it.

Add `session-recovery-guidance.test.ts` for foreign sessions, changed blocks, invalid generations, operation ordering, and separate application stores.
Cover a canonical projection arriving before its matching failure response.
A presentation-only revision must not discard that same-incident hint.
Cover an untyped continuation failure that retains valid same-incident guidance.
Cover matching success, deletion, superseding incidents, and old-success rejection.
Update recovery-hook mocks to provide the real captured owner where required.
Run all affected recovery-hook suites, not only the new test.

Extend the existing Restart desktop/phone suites and history-continuation helper.
Preserve draft/attachment and navigation-race coverage.
Assert no automatic continuation, exactly one explicit dispatch, menu unmount survival, and phone hitboxes.

### Real-backend release scenario

Issue 5256 is a hard dependency for this part of acceptance.
Keep this work order pending or in progress until the following check succeeds.

Use the existing `e2e-delivery-fixture` boundary and disposable `kandev-e2e-` runtime.
Extend its lifecycle test fixture with the required repeated-interruption state.
Retain a generation-1 silent-restore resolution, a generation-2 interrupted submission, and stale generation-1 presentation.
Cover both the unknown-prompt block and generic unresolved-work shape, including multiple independent open records.
Do not manufacture successful terminal evidence or edit a live user's database.

Add `blocked-session-continuation.spec.ts` and its `mobile-` wrapper using a shared helper.
Do not intercept successful recovery responses in this scenario.
Inspect actual request outcome, committed generation identity, saved transcript, open-block state, and mock prompt trace.
Require successful continuation initialization and admission of one distinct new instruction.
Assert that no original interrupted payload is replayed and that unrelated recovery gates survive.
If the companion repair does not provide this contract, record the failure and leave this acceptance incomplete.

## Verification

Run from the repository root after Task 02.
The managed browser commands rebuild and run sequentially.

```bash
(cd apps/web && pnpm exec vitest run lib/session/session-recovery-guidance.test.ts hooks/domains/session/use-session-restart.test.ts hooks/domains/session/use-session-recovery components/task/chat/session-recovery-context.test.tsx components/task/chat/session-recovery-card-history.test.tsx components/task/chat/session-stopped-banner.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-tab-restart.spec.ts tests/session/history-continuation-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-tab-restart.spec.ts tests/session/mobile-history-continuation-recovery.spec.ts)
make -C apps/backend build-e2e-delivery-fixture
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/e2e_delivery_record_fixture_test.go apps/backend/internal/agent/runtime/lifecycle/e2e_blocked_session_continuation_fixture_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/session/blocked-session-continuation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-blocked-session-continuation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The last two browser commands remain blocked until issue 5256's backend repair is present.
Record those results separately from intercepted routing scenarios.
Record the integrated companion commit and actual scenario counts.

## Files likely touched

- `apps/web/lib/session/session-recovery-guidance.ts` and `.test.ts` (new)
- `apps/web/hooks/domains/session/use-session-restart.ts` and `.test.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/hooks/domains/session/session-recovery-operation-fence.ts`
- Existing affected `use-session-recovery-*.test.ts` hook suites
- `apps/web/components/task/chat/session-recovery-context.tsx` and `.test.tsx`
- `apps/web/e2e/helpers/history-continuation-recovery.ts`
- `apps/web/e2e/tests/session/session-tab-restart.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-tab-restart.spec.ts`
- `apps/web/e2e/tests/session/session-tab-restart-helpers.ts`
- `apps/web/e2e/helpers/blocked-session-continuation.ts` (new)
- `apps/web/e2e/tests/session/blocked-session-continuation.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-blocked-session-continuation.spec.ts` (new)
- `apps/backend/internal/agent/runtime/lifecycle/e2e_delivery_record_fixture_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/e2e_blocked_session_continuation_fixture_test.go` (new bounded fixture helpers)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`, if feedback needs additional copy
- `docs/public/sessions-and-review.md`
- This package's `plan.md` and work-order results

Keep fixture-only preparation within the existing isolated test boundary.
The real-backend browser cases exercise the fixture through its compiled test binary.
A direct fixture run without the required disposable runtime is skipped and does not count as verification.

## Dependencies

Task 02 supplies consistent recovery rendering.
Issue 5256 integration is required for actual recovery acceptance, not for the hook or intercepted browser tests.

## Risks

A session-keyed hint without incident identity can authorize the wrong successor.
A page-global map can leak guidance between application-store owners.
A successful intercepted reply can conceal the reported bootstrap mismatch.

## Parallelism

`sequential`

## Inputs

- [Guidance lifetime and surface](../../specs/agents/system-design/session-recovery-guidance.md#request-guidance-lifetime).
- [Restart action and failure ownership](../../specs/tasks/system-design/session-tab-restart.md).
- Existing `session-restart-state.ts`, operation fencing, provider tests, and delivery-record fixture.
- [Issue 5256](https://github.com/kdlbs/kandev/issues/5256) and its integrated repair evidence.

## Results

Implementation completed 2026-11-11; real-backend acceptance remains pending the issue 5256 repair.

- Added store-scoped, incident-bound guidance shared by Restart and ordinary recovery. Matching failed continuation retains the explicit action; success, deletion, successor incidents, and stale operations are fenced.
- Matching same-generation recovery and resolution projections now retire and fence guidance; an authoritatively RUNNING session also cannot be replaced by a stale synthetic card. For non-running sessions, an omitted block projection remains distinct from a block observed and later removed.
- Added hook/provider tests for cross-consumer handoff, canonical projection ordering, stable identity across presentation revisions, failure retention, deletion, stale results, and store isolation.
- Focused recovery Vitest run: passed, 14 files and 97 tests. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- Restart desktop E2E: the new menu-unmount handoff passed; the suite had 5 passes and 1 failure. The existing real-backend interrupted-turn case did not render its recovery card after `unknown_prompt_outcome`; this is tracked by open [issue 5256](https://github.com/kdlbs/kandev/issues/5256).
- Restart phone E2E: passed, 3/3, including the new 320px handoff and geometry case.
- Desktop and phone uncertain-delivery continuation E2Es: passed, 2/2 each after the shared-provider integration.
- `make -C apps/backend build`: passed. Targeted orchestrator handler recovery tests passed; targeted core orchestrator recovery tests passed.
- Public-doc tests passed, 63/63; all 55 public docs validated. `python3 scripts/list-docs.py validate` validated 448 decisions and 1982 specifications; `python3 scripts/lint-spec-files.py --all` passed.
- The real-backend fixture acceptance was not run: the work order requires issue 5256 to be integrated first. No successful real recovery is claimed.

Review follow-up, 2026-11-11:

- Added provider coverage for external native recovery and for an authoritatively usable session whose recovery projection had not arrived when the hint was recorded. Both cases restore the composer, remove the hint, and reject a late failure.
- Added hook coverage for successful same-generation recovery whose projection arrives before the response; the request-key change no longer preserves the hint.
- Focused recovery Vitest run passed: 15 files, 100 tests. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- `pnpm run build:vite` and `make -C apps/backend build` passed. Targeted orchestrator and handler tests passed. `golangci-lint run ./internal/orchestrator ./internal/orchestrator/handlers --new-from-rev=HEAD --timeout=10m --concurrency=2` passed with zero issues.
- Issue 5256 remains a hard dependency for Task 03 real-backend acceptance. That acceptance remains incomplete and no successful real recovery is claimed.

PR fixup follow-up, 2026-11-11:

- Resolved guidance now compares recovery evidence against the identity captured when the operation began. A resolved delivery phase retires guidance only when the matching submission has a newer projection revision; an observed block that disappears also retires the hint. An omitted block projection remains distinct from a block observed and later removed.
- Relevant open blocks are tracked for every supported history-continuation cause, regardless of consumer. Delivery phase and submission metadata stay associated only with `agent_delivery` blocks.
- Added regressions for prior `recovered`, `settled`, and `restored` metadata followed by a later failure and a newer resolution projection, and for an observed interactive block being replaced or removed. Existing provider and hook regressions cover external native recovery, restoration of the composer, same-generation recovery projected before the successful response, and rejection of a late failure.
- Focused recovery Vitest run passed: 13 files, 138 tests. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:ratchet`, and `pnpm run build:vite` passed. The changed behavior is session identity and projection reconciliation only; it does not change layout, touch behavior, or viewport interaction, so the provider and hook tests cover the phone and desktop outcome without a new mobile E2E case.
- Task 03 remains in progress. Issue 5256 remains a separate hard dependency; real-backend acceptance is incomplete, and no successful real recovery is claimed.

- Retained recovery actions are now scoped to the provider's current task and session. A regression carries `continue_from_history` from one session into a different `STARTING` session and confirms the second session receives its own `resume` state.
- Focused provider and recovery-card tests passed: 3 files, 41 tests. `pnpm run typecheck`, `pnpm run lint`, and `pnpm run build:vite` passed.
