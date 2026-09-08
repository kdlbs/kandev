---
id: "11-mobile-parity"
title: "Mobile parity for the review findings surface"
status: pending
wave: 8
depends_on: ["10-review-surface-controls"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 11: Mobile parity for the review findings surface

Review is a primary mobile surface. Every desktop capability must be reachable with native mobile presentation.

## Inputs

- Load the `mobile-parity` skill first and follow its surface decision guide, design contract, and verification requirements.
- Spec's phone scenario.
- `apps/web/AGENTS.md` → Responsive and touch surfaces: use `hooks/use-responsive-breakpoint.ts` (640px phone boundary), not the UI package's `useIsMobile`; use `useTouchDrawer` for coarse-pointer disclosure; reuse the existing inset safe-area bottom-sheet treatment.
- `components/task/mobile/mobile-changes-panel.tsx` and `session-mobile-layout.test.tsx`.

## Work

1. `components/task/mobile/mobile-review-findings-sheet.tsx` — a `Drawer` bottom sheet listing findings grouped by repository then file, severity-sorted, with Resolve / Dismiss / Undo / Send-to-agent per finding and a tap target that navigates to the anchored line in the mobile diff.
2. `components/task/mobile/mobile-changes-panel.tsx` — a findings entry point showing the open count, plus the **Review changes** run control (reusing `review-run-button.tsx`) in the mobile changes toolbar with the same run states and the same `review_agent_unavailable` inline message.
3. Ensure the inline finding cards inside the mobile diff remain usable at phone width: the card actions collapse into an overflow menu below 640px rather than wrapping.
4. Confirm no capability is desktop-only: run the mobile-parity checklist against the finding actions, the run control, cancel, and clear.

## Acceptance

- Every desktop finding action is reachable on a 390×844 viewport.
- The sheet respects safe-area insets and closes on backdrop tap and Escape.
- Tapping a finding opens the right file and scrolls to the anchored line.

## Verification

```
cd apps/web && pnpm vitest run components/task/mobile
cd apps/web && pnpm e2e --project=mobile-chrome e2e/mobile/code-review-findings.spec.ts
```

## Files likely touched

`components/task/mobile/{mobile-review-findings-sheet.tsx,mobile-changes-panel.tsx}`, `components/diff/review-finding-card.tsx`, plus tests.

## Output contract

Summary, files changed, tests run with results, the mobile-parity checklist outcome, blockers, risks, `status: done`, plan checkbox.
