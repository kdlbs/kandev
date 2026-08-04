---
id: "02-prove-responsive-cancellation-e2e"
title: "Prove responsive cancellation end to end"
status: done
wave: 2
depends_on: ["01-break-cancellation-stream-cycle"]
plan: "plan.md"
spec: "../../specs/workflow/cancelled-turn-completion/spec.md"
---

# Task 02: Prove responsive cancellation end to end

## Acceptance

- The existing desktop delayed-turn scenario proves the real cancel control and input settle within two seconds after cancellation acknowledgement and terminal-frame publication while enabled and disabled workflow completion policies still produce their existing single outcomes.
- The existing `mobile-chrome` scenario invokes the shared control by touch and proves the same post-acknowledgement two-second outcome without changing mobile composition, touch geometry, scroll ownership, or overflow behavior.
- The test uses deterministic mock-agent cancellation and locator/event assertions, not fixed sleeps, a live Codex credential, or a relaxed 10-30-second timeout that would hide the regression.

## Verification

```bash
(
  cd apps
  pnpm install --frozen-lockfile
  cd web
  pnpm e2e:run tests/workflow/workflow-cancel-completion.spec.ts
  pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts -- --repeat-each=3 --workers=1
)
```

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-cancel-completion.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-cancel-completion.spec.ts`
- `apps/web/e2e/helpers/cancellation.ts`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/script.go`
- `apps/backend/cmd/mock-agent/mock_agent_test.go`
- `apps/backend/cmd/mock-agent/script_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. The test is meaningful only after the backend operation and guard semantics are fixed.

## Inputs

- Spec: acknowledged-cancellation responsiveness and existing completion-policy scenarios.
- Plan: `Existing shared cancel control`, `Mobile design contract`, and `E2E Tests`.
- Nearest shipped mobile exemplar: the current mobile cancelled-turn completion spec and shared `SubmitButton` touch path.

## Risks

- Build fresh artifacts before the first Playwright run; the second command may reuse them with `--no-build`.
- Bound only the cancellation settlement assertion. Keep workflow-transition assertions at their existing event-driven timeouts so unrelated CI scheduling does not masquerade as a responsiveness failure.
- Do not add production delays, test-only endpoints, hardcoded credentials, new copy, or viewport-specific product logic.

## Output contract

Report desktop/mobile scenarios, discovered test counts, exact commands, generated Playwright artifacts, cleanup evidence, residual timing risk, and synchronize this task plus `plan.md` in the same primary conversation.

## Results

- Tightened the existing desktop and mobile cancellation scenarios to assert that the cancel control leaves its progress state and the input becomes promptable within two seconds after cancellation acknowledgement and terminal-frame publication. Workflow transition assertions retain their event-driven timeouts.
- Moved the shared cancellation settle timeout and assertion into `apps/web/e2e/helpers/cancellation.ts` so desktop and mobile cannot drift.
- Made the delayed mock-agent script honor prompt cancellation so the fixture emits terminal ACP frames promptly instead of waiting for an artificial delay after cancellation. Added a focused context-cancellation unit test and rebuilt `apps/backend/bin/mock-agent`.
- Desktop: `cd apps/web && pnpm e2e:run --no-build --project chromium tests/workflow/workflow-cancel-completion.spec.ts -- --retries=0` — 2 tests passed.
- Mobile: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts -- --retries=0` — 1 test passed. A final repeat with `--repeat-each=3 --workers=1` passed 3/3 after rebuilding the cancellation-aware mock agent.
- The mobile flow uses touch assertions, viewport containment, and the existing no-horizontal-overflow check. No production frontend component or copy changed; no screenshots/traces were checked in, and managed E2E teardown completed.
- `cd apps/web && pnpm run typecheck` and `cd apps/web && pnpm run lint` — passed. `git diff --check` — passed.
