---
id: "02-uncertain-delivery-actions"
title: "Retain continuation in uncertain delivery"
status: done
wave: 2
depends_on:
  - "01-typed-block-guidance"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.18
system_design:
  - ../../specs/agents/system-design/session-recovery-guidance.md
---

# Task 02: Retain continuation in uncertain delivery

## Summary

Show the server's current continuation action in the uncertain-delivery adapter.
Reuse the ordinary recovery choice and preserve profile, pending, and refusal checks.

## In scope

- Repair the early-return path in `useStoppedRecoveryChoices`.
- Share continuation-choice construction without exposing unrelated workspace/reset operations.
- Put a matching continuation hint first and explain its effect before activation.
- Add permanent component and desktop/phone browser regressions.
- Supply new disclosure copy in every shipped locale and pseudo.

## Out of scope

Restart's cross-component guidance handoff and backend recovery semantics.
Do not reinterpret Stop as recovery-block resolution.

## Acceptance

1. Identical validated guidance exposes continuation in ordinary and uncertain-delivery recovery. No request occurs before explicit activation.
2. One activation dispatches exactly one `continue_from_history` request. Profile loss, relocation, active operations, and non-retryable guards retain their restrictions.
3. Desktop and phone show the disclosed effect, pending state, and failed-attempt feedback. Actions remain reachable without overflow or extra diagnostic scrolling.

## ASCII UI preview

UI-01 and UI-03 from the [combined preview](plan.md#ascii-ui-preview), mapped to the acceptance criteria in frontmatter.

```text
Desktop
+----------------------------------------------------+
| Delivery uncertain. Saved history starts a new      |
| native conversation. Old prompt outcome is unknown. |
| [Continue from saved history] [eligible alternatives]|
+----------------------------------------------------+

Phone
+----------------------------------+
| Delivery uncertain               |
| New native conversation from     |
| saved history. Old outcome stays |
| unknown.                         |
| [Continue from saved history   ] |
| [Eligible alternative          ] |
+----------------------------------+

Pending: action disabled + accessible status
Failed:  localized failure + explicit retry
Blocked: prerequisite + no enabled forbidden action
```

Hierarchy and order are required. Copy and spacing are illustrative.
Phone actions stack at full width and measure at least 44 pixels.
Desktop actions wrap and retain standard sizing.
Reuse the inline recovery region's scroll owner and safe-area handling.

## Regression tests

First add `retains continuation guidance when delivery is uncertain` to `session-stopped-banner.test.tsx`.
Recreate the temporary reproduction described by the plan.
It must fail on the missing continuation button before the correction.
Add explicit-dispatch, missing-profile, guard, busy, and no-unrelated-actions assertions.
Keep composer-card history and relocation-precedence coverage.

Extend `history-continuation-recovery.ts` with an uncertain-delivery scenario.
Use its existing desktop and `mobile-` wrappers.
Assert request count before and after activation, pending feedback, and contained controls.
Measure phone and coarse-pointer hitboxes, including a narrow fine-pointer phone width.
Capture the rendered phone view and compare it with UI-01.
Use causal WebSocket waits, not fixed sleeps.

## Verification

Run from the repository root.
In a fresh worktree without dependencies, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/session-stopped-banner.test.tsx components/task/chat/session-recovery-card-history.test.tsx components/task/chat/pruned-delivery-recovery.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/history-continuation-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-history-continuation-recovery.spec.ts)
git diff --check
```

The managed browser runner rebuilds current assets and tears down its disposable backend.
Run the desktop and phone commands sequentially.
These intercepted browser replies prove UI routing, not the issue 5256 backend repair.

## Files likely touched

- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/components/task/chat/session-stopped-banner.test.tsx`
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/messages/action-message-recovery.tsx`
- `apps/web/e2e/helpers/history-continuation-recovery.ts`
- `apps/web/e2e/tests/session/history-continuation-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-history-continuation-recovery.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`

Use `pnpm run i18n:zh-hant` for the Traditional Chinese pair and `pnpm run i18n:pseudo` for pseudo.
Inspect generated catalog changes before recording results.

## Dependencies

Task 01 supplies canonical typed feedback and validated normalization.
Component rendering can be reproduced independently with injected guidance.

## Risks

Appending all optional stopped-session actions can expose unsupported operations during uncertain delivery.
Add only the validated continuation choice and retain existing specialized action policy.

## Parallelism

`sequential`

## Inputs

- [Active recovery surface](../../specs/agents/system-design/session-recovery-guidance.md#active-recovery-surface).
- [Plan reproduction and previews](plan.md).
- Existing history-continuation helper and `assertDeliveryRecoveryGeometry`.

## Results

Completed 2026-11-11.

- Reused the validated continuation-choice path for uncertain delivery and added the localized disclosure before explicit activation.
- Added permanent component coverage for ordinary and uncertain-delivery rendering, guards, pending state, and explicit dispatch.
- `(cd apps/web && pnpm exec vitest run lib/session/session-recovery-guidance.test.ts hooks/domains/session/use-session-restart.test.ts hooks/domains/session/use-session-recovery-*.test.ts components/task/chat/session-recovery-context.test.tsx components/task/chat/session-recovery-card-history.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/session-stopped-banner-continuation.test.tsx)`: passed, 14 files and 97 tests.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- Desktop uncertain-delivery E2E: passed, 2/2. Phone uncertain-delivery E2E: passed, 2/2. Both were rerun after Task 03 integration with `--no-build`.
- `git diff --check`: passed.
- Browser routing tests use intercepted recovery responses and do not verify the issue 5256 backend repair.
