---
id: "01-panel-responsive-header"
title: "Make the PR detail header panel-responsive"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-detail-header-width.md"
---

# Task 01: Make the PR Detail Header Panel-Responsive

## Acceptance

- The action cluster shares the title row only when the complete title remains
  one line; otherwise it moves below and returns the full row to the title.
- Long titles wrap to show their complete text instead of clipping with an
  ellipsis, on desktop and phone Review surfaces. Lines fill the available
  title width normally instead of balancing into similarly sized rows.
- At narrow widths, the action cluster starts at the title's leading edge
  instead of floating independently on the trailing side.
- The transition uses intrinsic content width rather than a fixed breakpoint,
  and crossing it loses or remounts no action.
- The 320px phone Review flow retains touch-usable actions, one scroll owner,
  the existing re-request outcome, and no document-level horizontal overflow.

## TDD sequence

1. Add a desktop geometry regression covering inline-fit, squeezed, and
   full-width wrapping states. Confirm RED at a width where actions force a
   title that fits alone onto two lines.
2. Replace the fixed container query with intrinsic flex wrapping while keeping
   natural title wrapping, long-token protection, action order, and leading
   alignment.
3. Run the focused desktop scenario until green, then reuse its production
   build for the focused `mobile-chrome` scenario.
4. Refactor only if the minimal class change exposes duplication; rerun both
   scenarios after any refactor.

## Files likely touched

- `apps/web/components/integrations/change-request-detail.tsx`
- `apps/web/components/integrations/change-request-detail-header.tsx`
- `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-rerequest-review.spec.ts`
- `docs/plans/pr-detail-header-width/plan.md`
- `docs/plans/pr-detail-header-width/task-01-panel-responsive-header.md`

## Verification

Run from the repository root:

```bash
cd apps && \
pnpm install --frozen-lockfile && \
pnpm --filter @kandev/web lint -- components/integrations/change-request-detail.tsx components/integrations/change-request-detail-header.tsx e2e/helpers/layout-assertions.ts e2e/tests/pr/pr-detail-layout.spec.ts e2e/tests/pr/mobile-pr-rerequest-review.spec.ts && \
cd web && \
pnpm e2e:run --host --project chromium tests/pr/pr-detail-layout.spec.ts -- --grep "moves PR actions below before they force the title to wrap" --workers=1 && \
pnpm e2e:run --host --no-build --project mobile-chrome tests/pr/mobile-pr-rerequest-review.spec.ts -- --grep "uses bottom-nav Review to re-request a dismissed review without overflow" --workers=1
```

The first managed E2E run rebuilds production frontend and backend artifacts;
the second reuses those unchanged artifacts. Confirm each command discovers
one intended test before accepting it as evidence.

## Dependencies

None.

## Parallelism

`sequential`. Production layout and both geometry regressions define one
coupled responsive contract and belong in one Red-Green-Refactor cycle.

## Inputs

- Scenarios in `docs/specs/ui/requirements/pr-detail-header-width.md`.
- Frontend and mobile contracts in `plan.md`.
- Shared header implementation in
  `apps/web/components/integrations/change-request-detail-header.tsx`.
- Dockview resize helpers in `apps/web/e2e/helpers/dockview-resize.ts`.
- Existing desktop and phone PR setup in the two owning E2E specs.

## Risks

- Preserve the title's automatic flex basis; `flex-1` would use a zero basis
  and let the action cluster force title wrapping on the same row.
- Let the action cluster shrink within the available row after it wraps below,
  otherwise phone controls can overflow horizontally.
- Wait for both GitHub actions through observable locators. Do not add sleeps or
  increase timeouts to hide asynchronous setup.
- Keep long-title content inside its content box while allowing natural
  multiline layout.

## Output contract

Report changed files, observed RED failures, final commands and test counts,
rendered evidence, blockers and risks, plus synchronized task and plan statuses
in this conversation.

## Results

- RED: at the initial 1000px detail width, actions stayed inline and forced the
  title onto two lines instead of moving below.
- GREEN: intrinsic flex wrapping passed desktop Chromium 1/1 across 1200px,
  a content-derived squeezed width, and 600px detail widths. Actions remain
  inline only in the first single-line-fit state; squeezed and wrapping states
  give the title a full row.
- Mobile Chromium passed 1/1 while retaining 44px targets, action order,
  re-request behavior, natural wrapping, and zero horizontal overflow.
- CI follow-up replaced the fixed squeezed width with a content-derived width
  so font metrics cannot move the test across the flex boundary; desktop
  Chromium passed 1/1.
- Live rendering confirmed the 1200px inline, 1000px stacked single-line, 600px
  stacked multiline, and 320px phone states. Targeted ESLint and Prettier
  passed, `git diff --check` was clean, and the fixed container-query classes
  were absent.
