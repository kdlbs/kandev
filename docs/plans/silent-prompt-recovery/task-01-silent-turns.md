---
id: "01-silent-turns"
title: "Preserve silent accepted turns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.1
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.2
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.3
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.4
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.5
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.6
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.8
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.9
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.12
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.13
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.14
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.15
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 01: Preserve silent accepted turns

## Summary

Separate acceptance from progress. Keep quiet turns running with the existing
cancel action, and retain uncertainty when cancellation cannot confirm outcome.

## In scope

- Add a generation-safe acceptance snapshot using `markPromptDispatched` and
  authenticated adopted-submission evidence. Preserve output/effect flags.
- Replace inactivity-only launch failure and forced teardown with advisory
  handling. Keep trusted startup-error cleanup and explicit task Stop.
- Exercise `agent.cancel` through real cancellation ownership and durable
  settlement. Reuse its bounded waits; repair only behavior that violates
  the referenced contract. Do not use `CancelTurn`'s internal success mapping
  as evidence that a durable submission has settled.
- Capture the bounded first-stall snapshot defined in the design. Test valid
  multibyte truncation within the byte cap and omission of malformed UTF-8.
- Update desktop/phone E2E, public recovery guidance in
  `docs/public/agents-and-profiles.md`, and localization only if new copy is
  necessary. Preserve unrelated completed plan results.

## Out of scope

Resume wait, new settings, native-log collection, automatic retry, or runtime
version changes.

## Acceptance

1. Accepted or unconfirmed silence produces one neutral notice and no forced
   stop; metadata does not postpone it. Late acceptance/output and successor
   generations cannot trigger an old terminal action or alter replay evidence.
2. Cancel targets the captured turn. Confirmed settlement permits ordinary
   admission; uncertain delivery stays blocked with no replay, including after
   reload. Duplicate/stale cancellation cannot affect a successor.
3. Desktop and phone complete these flows. The first-stall record contains
   bounded observed metadata, never prompt text, arguments, credentials, or
   raw provider logs.

## UI preview

[UI-01 and UI-02 in the plan](plan.md#ui-preview) apply.
The shared transcript row orders “Still waiting for the agent.” before the
inline **Cancel turn** action. Phone touch height is at least 44px.
Busy cancellation disables duplicate actions; uncertainty shows existing
recovery controls. No new scroll region, card, or animation.
Agents criteria .4-.6, .12-.14 and Platform .2 own these states.

## Verification

Run from the repository root. Add tests first and record their failing result.
Use fake time for the five-minute threshold.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Stall|AcceptedSilentPrompt|Prompt.*Evidence|Cancel|Cancellation|ResumedTurn|ResumeAttempt' -count=1)
(cd apps/backend && go test -race ./internal/agentctl/server/instance ./internal/agentctl/journal -run 'Cancel|Interrupt|Submission' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx components/task/chat/messages/action-message-actions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/session/stall-notice-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-stall-notice-recovery.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the actual changed Go paths.
If TS/TSX changes, run eslint on those paths from `apps/web`.
If translations change, run `pnpm run i18n:zh-hant` and `pnpm run i18n:ratchet`
there. Record the exact expanded commands in Results.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/{types,session,manager_events}.go`
- Lifecycle acceptance/adoption helpers and `stall_activity_test.go`
- `apps/backend/internal/orchestrator/event_handlers_stall.go` and its tests
- Lifecycle `manager_interaction_dispatch_cancel.go` and durable cancel tests
- Orchestrator cancellation and uncertain-delivery tests
- `apps/web/components/task/chat/messages/action-message-actions.tsx` only if
  existing busy handling or geometry fails the regression
- Both stall-notice E2E files, mock-agent scenario support, and existing
  test-only API fixtures
- `docs/public/agents-and-profiles.md`

## Dependencies

None. Read the linked design and decision before writing the regression.

## Risks

Do not count `session_info`, old native IDs, or HTTP dispatch intent as acceptance.
Do not introduce a reverse lock order between prompt lifecycle and activity.
Test-only clock controls must remain unavailable in production.

## Parallelism

sequential

## Inputs

- [Agent requirements](../../specs/agents/requirements/agent-stall-recovery.md)
- [Agent design](../../specs/agents/system-design/agent-stall-recovery.md)
- [Recovery design](../../specs/platform/system-design/interrupted-prompt-recovery.md)
- Existing `stall_activity_test.go`, `event_handlers_stall_test.go`,
  `manager_delivery_cancel_test.go`, and mobile stall E2E.

## Results

Implemented and verified.

- Failing regressions reproduced the terminal no-output action, acceptance
  erasing early output, inherited predecessor activity, missing bounded
  diagnostics, lost restored acceptance, and an old completion replacing a
  restored generation. Each received a focused fix.
- Lifecycle and orchestrator stall/cancellation tests pass under `-race`.
  Restored-generation and agentctl cancellation checks also pass under `-race`.
- The inline Cancel action now uses shared pending state after reload.
  Both chat action suites pass (57 tests). Targeted eslint and i18n checks pass.
- Typecheck passes with `NODE_OPTIONS=--max-old-space-size=8192`; the default
  4 GB Node heap was insufficient for the current repository.
- Final browser checks after integrating current main passed on desktop and
  phone: five cases on each viewport cover both work orders. This includes
  resumed output, confirmed cancellation, uncertain cancellation, and Resume.
  The shorter timer requires the E2E runtime profile. Desktop and phone
  screenshots were inspected; narrow fine-pointer and touch geometry retain
  the intended control sizes.
- The uncertainty fixture loses the runtime during cancellation and rejects
  native resume. Delayed provider acknowledgement alone can still produce a
  confirmed durable cancellation, and runtime replacement can restore native
  continuity; neither is proof that the session must stay blocked.
- Changed-package Go lint passes with `GOMAXPROCS=4 GOMEMLIMIT=6GiB`.
  The default two-worker, 3 GiB run exhausted its five-minute analysis budget.
- Browser commands use a temporary environment wrapper to remove inherited
  internal config-file settings and provide installed Nix Chromium libraries.
- Public quiet-turn recovery instructions are updated.
- Review follow-up documents the shared output/effect evidence and snapshot
  lock order. `go test -race ./internal/agent/runtime/lifecycle
  -run TestStallDiagnosticBoundsAndExcludesPrivatePayloads -count=1` passes,
  including the explicit effect-evidence assertion.

Design review correction: the verification filter includes
`TestWaitForPromptDone_AcceptedSilentPromptIsAdvisory`. A direct regex check
confirmed that match; documentation catalog, specification lint, and whitespace
checks passed at that checkpoint. The executable regression now passes.

## Browser fixture timing correction (2026-10-27)

A retained SDK integration CI trace proves that the real notice appeared and
output cleared it before the fixture finished navigation/readiness. The prompt
was accepted at 02:29:17.944 UTC; the watchdog reported no output at 18.954;
the notice was visible at 27.951; agent-rendered progress was visible with no
notice at 28.621; the test began expecting a notice at 28.783. The fixed ten-second
silent interval was therefore an invalid observation window.

The first browser scenario now uses two release gates. A narrow mockTurnGate
option omits the initial announcement; existing callers retain it by default.
The first gate preserves silence until the original notice and touch-target
checks finish. Releasing it emits progress. The second gate keeps the turn alive
through the original notice-cleared, RUNNING, reload, and overflow checks.
Both gates release in finally, including setup failures; the normal path stops
the session first. Watchdog thresholds, assertion budgets, and product code are
unchanged. The unextended helper failed the progress assertion because its
announcements contaminated the required silent/progress text. Desktop recovery
coverage passes with the silent option; phone and default-caller checks follow.

Validation result: five desktop recovery cases, four phone recovery cases,
and the existing default-announcement preview-feedback consumer all passed
with retries disabled. The default consumer explicitly observes its original
start announcement. Independent review found no actionable issue. The retained
CI trace proves the fixed-delay race separately from the controlled silent-mode
RED. Product watchdog and recovery logic are unchanged.
