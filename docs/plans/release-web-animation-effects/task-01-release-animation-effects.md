---
id: "01-release-animation-effects"
title: "Release animation effects"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHAT-MOTION-003
acceptance_criteria:
  - AC-UI-CHAT-MOTION-003.4
system_design:
  - ../../specs/ui/system-design/chat-motion.md
---

# Task 01: Release Animation Effects

## Summary

Detach the effect from every script animation when its owner cancels it, so
macOS WebKit can collect removed chat rows and text runs.

## Acceptance

1. Chat row and text motion cleanup cancels the animation and sets `effect` to null.
2. Every other `element.animate()` owner in `apps/web` and `apps/packages/ui` uses
   the same helper.
3. Settings-search motion releases an animation when it finishes, not only on cancel.
   It clears the owned finish callback before cancellation; queued old finish events
   cannot release a replacement handle.
4. Existing motion behavior and tests are unchanged.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/chat-motion.test.tsx components/shared/chat-markdown-motion.test.tsx components/grid-spinner.test.tsx lib/ui/ hooks/use-chat-motion.test.tsx components/app-sidebar/sections/settings/use-settings-search-motion.test.tsx)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Historical contributor results

- The new assertions failed before the change and pass after it.
- 12 files and 123 tests pass. Typecheck and scoped ESLint pass.

## Files likely touched

- `apps/packages/ui/src/animation-utils.tsx`
- `apps/packages/ui/src/compositor-spin.tsx`, `compositor-pulse.tsx`
- `apps/web/components/task/chat/chat-motion.tsx` and its test
- `apps/web/components/shared/chat-markdown-motion.tsx` and its test
- `apps/web/components/grid-spinner.tsx`
- `apps/web/components/app-sidebar/sections/settings/use-settings-search-motion.ts`

## Dependencies

None.

## Parallelism

`sequential`
