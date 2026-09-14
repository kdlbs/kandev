---
id: "02-recognize-visible-dockview-chat-panels"
title: "Recognize visible Dockview chat panels"
status: done
wave: 2
depends_on:
  - "01-preserve-read-cursor-across-task-switches"
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.15
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 02: Recognize Visible Dockview Chat Panels

## Summary

Treat the selected Chat tab as visible independently of which side-by-side
Dockview group owns global focus. Prove that a completed task restores its
transcript placement without relying on a later agent update.

## In scope

- Subscribe to Dockview panel visibility rather than global active state.
- Cover the focused-sibling-group contract in the visibility hook.
- Extend the desktop completed-task switch regression with Changes-group focus.

## Out of scope

- Dockview layout persistence or active-group restoration.
- Transcript auto-scroll preference semantics or stored offsets.
- Mobile composition, which does not mount Dockview.

## Acceptance

- A selected Chat tab reports visible when another Dockview group owns focus.
- Hiding Chat behind another tab in its own group reports not visible.
- Returning to an idle completed task places the transcript at the bottom while
  Changes remains the globally active panel.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run hooks/use-panel-active.test.ts)
(cd apps/web && pnpm exec eslint hooks/use-panel-active.ts hooks/use-panel-active.test.ts e2e/tests/chat/unread-divider.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --host --project chromium tests/chat/unread-divider.spec.ts -- --grep "completed task switch" --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-unread-divider.spec.ts -- --grep "completed task switch" --retries=0)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/use-panel-active.ts`
- `apps/web/hooks/use-panel-active.test.ts`
- `apps/web/e2e/tests/chat/unread-divider.spec.ts`
- `docs/specs/ui/requirements/transcript-auto-scroll.md`
- `docs/specs/ui/system-design/transcript-auto-scroll.md`

## Dependencies

Task 01 established the environment-switch placement lifecycle and its
completed-task regression.

## Risks

- Using global Dockview focus recreates the defect when another visible group
  remains active after layout restoration.
- Treating every mounted portal as visible could mark hidden session tabs read.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001`, especially acceptance criteria `.11`,
  `.14`, and `.15`.
- UI transcript auto-scroll system design.
- Dockview panel visibility and active-state contracts.

## Results

- Replaced Dockview's global active-state subscription with the group-local
  visibility property and event.
- Added unit coverage for a visible Chat panel whose sibling group owns focus.
- Extended the desktop completed-task switch regression to retain Changes as
  the active panel and prove the visible transcript returns to the bottom.
- Focused Vitest, scoped ESLint, TypeScript typecheck, desktop Chromium E2E,
  Mobile Chrome parity E2E, specification lint, and diff checks passed.
