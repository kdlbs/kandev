---
id: "02-recovery-proof"
title: "Prove OpenCode recovery and update guidance"
status: done
wave: 2
depends_on:
  - "01-opencode-capacity-continuation"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.1
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.2
  - AC-PLATFORM-TURN-CONTINUITY-002.3
  - AC-PLATFORM-TURN-CONTINUITY-002.4
  - AC-PLATFORM-TURN-CONTINUITY-002.5
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.5
  - AC-PLATFORM-TURN-CONTINUITY-003.6
  - AC-PLATFORM-TURN-CONTINUITY-003.7
  - AC-PLATFORM-TURN-CONTINUITY-003.8
  - AC-PLATFORM-TURN-CONTINUITY-003.9
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 02: Prove OpenCode recovery and update guidance

## Summary

Prove that a terminal OpenCode capacity refusal enters automatic recovery in real task Chat.
Use production-shaped ACP frames in isolated fixtures and verify desktop and phone outcomes.
Update public guidance after the behavior passes.

## In scope

- Add an isolated OpenCode capacity scenario with native retry metadata, completed tool work, and the exact terminal error.
  Select the production OpenCode dialect through trusted test construction.
  Do not use the mock-only capability to bypass Task 01's provider support.
- Record process, connection, execution, generation, native session, prompt acceptance, and completed-effect counts.
  Keep trace files inside disposable test directories.
- Create the desktop/mobile specs and shared helper assigned by the plan.
  Cover active native retry, terminal backoff, success, repeated failure without new tools,
  Cancel, reload, another viewer, mixed unsafe work, and truthful final feedback.
- Use the existing isolated retry clock to test waits and expiry without long real-time delays.
  Reuse existing delayed-capacity restart/exhaustion regressions rather than duplicate their entire matrix.
- Verify the existing phone notice, Cancel hitbox, wrapping, composer, and transcript behavior.
  No presentation or locale changes are planned.
- Update `docs/public/tasks-and-workflows.md`, under Model capacity.
  Name tested OpenCode capacity support and explain native retries followed by bounded host continuation.
  Describe provider wait hints without exposing implementation details.
  Retain the confirmed-results, uncertain-work, cancellation, and restart limits.
- Record final commands and results in this work order and the plan.
  Resolve the October 28 draft amendment notes only after both work orders pass.

## Out of scope

UI redesign, new retry controls, translation changes, broad E2E runs,
live task mutation, model switching, and production scheduling overrides.

## Acceptance

1. Desktop and phone recover through the OpenCode classifier and dialect with one notice, one runtime, and no repeated original request or completed effect.
2. Cancel, supersession, unsafe work, expiry, and duplicate viewers preserve dispatch ownership and truthful durable feedback.
3. Focused suites, documentation checks, and recorded native compatibility evidence satisfy the complete package before its draft amendments are promoted.

## Verification

Run from the repository root. Install workspace dependencies once before the first pnpm command in a fresh worktree.
Run browser projects sequentially with the managed runner.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent ./internal/agent/runtime/lifecycle -run 'OpenCodeCapacity|Capacity|Retained' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/session/opencode-capacity-retry.spec.ts tests/session/delayed-capacity-retries.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-opencode-capacity-retry.spec.ts tests/session/mobile-delayed-capacity-retries.spec.ts)
(cd apps/web && pnpm exec eslint e2e/helpers/opencode-capacity-retry.ts e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/session/mobile-opencode-capacity-retry.spec.ts --max-warnings 0)
(cd apps/web && pnpm exec prettier --check e2e/helpers/opencode-capacity-retry.ts e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/session/mobile-opencode-capacity-retry.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/opencode-capacity-retry
```

The managed browser runner builds fresh assets and owns cleanup.
Do not use all-worker overrides or overlap browser suites.
Run changed-Go lint and the [documentation coverage preflight](plan.md#documentation-coverage-preflight).
Record actual scenario counts and any omitted evidence.

## Files likely touched

- `apps/backend/cmd/mock-agent/{opencode_capacity.go,opencode_capacity_test.go}`.
- `apps/backend/cmd/mock-agent/capacity_continuity.go` where scenario dispatch needs extension.
- Existing isolated lifecycle/fixture construction needed to select the production OpenCode dialect.
- `apps/web/e2e/helpers/opencode-capacity-retry.ts`.
- `apps/web/e2e/helpers/transient-turn-runtime-continuity.ts` only for shared fixture reuse.
- `apps/web/e2e/tests/session/{opencode-capacity-retry.spec.ts,mobile-opencode-capacity-retry.spec.ts}`.
- `docs/public/tasks-and-workflows.md`.
- This package and the four referenced specification amendments for lifecycle/results only.

## Dependencies

Task 01 and its native compatibility evidence.
Read web, mock-agent, mobile-parity, E2E, and public-docs guidance before implementation.

## Risks

- A mock-only capability can hide missing production OpenCode support.
- Browser fake time cannot prove host backoff timing.
- A repeated boot label cannot prove unchanged runtime identity.
- A generic error heading cannot prove that capacity recovery replaced the incorrect recovery controls.
- Temporary profiles, files, processes, and backend configuration must be cleaned up after failed assertions too.

## Parallelism

`sequential`

## Inputs

- [Plan scenarios and compatibility matrix](plan.md).
- Platform design: OpenCode support, durable projection, desktop and phone.
- Existing `capacity_continuity.go`, `delayed-capacity-retries.ts`,
  `transient-turn-runtime-continuity.ts`, and desktop/mobile delayed-capacity specs.
- Existing `TransientRetryNotice` and `session-mobile-layout.tsx` as the shipped phone presentation.

## Results

Delivered and locally verified.

- `cmd/mock-agent/opencode_capacity.go` adds
  `/opencode-capacity-{after-tools,completed-tools,retry,cancel,exhaust}[:N]`.
  It emits advisory native retry metadata, one completed edit for the completed
  shape, an unresolved tool for the unsafe shape, and the exact terminal
  admission refusal without a `kandevMock` marker. Continuation prompts reuse
  the stored scenario and never replay the completed tool.
- `e2e/helpers/opencode-capacity-retry.ts` restarts the backend with
  `KANDEV_MOCK_PROVIDERS=opencode-acp`, so the production OpenCode dialect and
  classifier run. It freezes the backend retry clock, drives one notice at a
  time, and asserts the ACP trace: one process, one connection, one native
  session, one initialize, one `session_new`, no load or resume, the expected
  prompt count, and one completed effect.
- Desktop coverage (`tests/session/opencode-capacity-retry.spec.ts`): recovery
  after the fifth retry, reload and a second viewer without another dispatch,
  repeated continuation failures through exhaustion, cancellation followed by
  new input, mixed completed and pending work kept manual, and window expiry.
- Phone coverage (`tests/session/mobile-opencode-capacity-retry.spec.ts`):
  the same recovery flow with a reachable 44px Cancel target and no horizontal
  overflow, plus cancellation followed by new input.
- Browser proof: 9 passed on `chromium` including the existing delayed-capacity
  regressions, and 6 passed on `mobile-chrome`, in 1.2 minutes and 45.9 seconds.
- `docs/public/tasks-and-workflows.md` names tested Codex and OpenCode support,
  explains native retries followed by bounded host continuation, and states that
  a provider wait hint can only raise one of the first five waits.
- The October 28 draft notes in the four referenced specifications are resolved.
  No UI markup or locale change was necessary.

The relative hint is applied at its exact observation time, so the displayed
whole-second value can be one second short of the armed deadline. The browser
helper advances one extra second instead of sleeping.
