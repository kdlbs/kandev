---
id: "02-normalize-structured-app-confirmations"
title: "Normalize structured app confirmations"
status: completed
wave: 2
depends_on:
  - "01-standardize-surface-typography-primitives"
plan: "plan.md"
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
acceptance_criteria:
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.5
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
---

# Task 02: Normalize Structured App Confirmations

## Summary

Normalize the two non-task AlertDialog descriptions that contain nested prose
or lists: Quick Chat deletion and agent-profile deletion conflicts.

## In scope

- Keep each Radix description boundary while using semantic paragraphs/lists,
  readable spacing, explicit left alignment, and local min-width containment.
- Preserve all existing localized copy, conflict groups, conditional blockers,
  actions, callbacks, and scroll containment.
- Add or extend focused component tests for rendered structure and long dynamic
  values.

## Out of scope

- Task-domain confirmations.
- Rewriting catalog copy, changing action variants/sizes, or changing conflict
  business rules.
- Shared primitive files owned by Task 01.

## Implementation acceptance

1. Both structured descriptions render as one accessible description with
   left-aligned prose and semantic paragraph/list structure.
2. Long chat/profile labels remain inside their existing surface and scroll
   body without hiding any conflict group.
3. Existing action availability and callback behavior remain unchanged.

## TDD sequence

1. Add focused assertions for description semantics, alignment, spacing, and a
   long-value case; observe failures against the current markup.
2. Apply only local structure/classes needed by the shared design.
3. Run focused tests GREEN, then typecheck, lint, and i18n checks.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- components/quick-chat/quick-chat-delete-dialog.test.tsx components/settings/agent-profile-delete-dialog.test.tsx
pnpm --filter @kandev/web run typecheck
cd web
pnpm exec eslint --max-warnings 0 components/quick-chat/quick-chat-delete-dialog.tsx components/quick-chat/quick-chat-delete-dialog.test.tsx components/settings/agent-profile-delete-dialog.tsx components/settings/agent-profile-delete-dialog.test.tsx
pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-delete-dialog.tsx`
- `apps/web/components/quick-chat/quick-chat-delete-dialog.test.tsx` (new)
- `apps/web/components/settings/agent-profile-delete-dialog.tsx`
- `apps/web/components/settings/agent-profile-delete-dialog.test.tsx`

## Dependencies

- Task 01 must be integrated before final verification.

## Parallelism

`parallel-wave-2`

## Inputs

- `docs/specs/ui/requirements/surface-text-hierarchy.md`
- `docs/specs/ui/system-design/surface-text-hierarchy.md`
- `docs/plans/surface-text-hierarchy/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

- RED: The focused component tests failed on the pre-change markup because the
  Quick Chat description had no local `min-w-0` containment and the
  agent-profile conflict body had no structured `space-y-2` spacing. Existing
  behavior tests remained green.
- GREEN: Added local structured-description classes and semantic list-item
  containment to the Quick Chat delete and agent-profile delete conflict
  dialogs. Preserved all localized copy, blocker groups, action variants,
  callbacks, and existing scroll containment.
- Added focused component assertions for the rendered description boundary,
  direct paragraph/list structure, long dynamic task values, class-level
  containment, and Quick Chat Delete callback behavior.
- Focused verification passed with 2 test files and 13 tests:
  `pnpm --filter @kandev/web test --
  components/quick-chat/quick-chat-delete-dialog.test.tsx
  components/settings/agent-profile-delete-dialog.test.tsx`
- Final exact-head checks passed: web typecheck, focused ESLint with
  `--max-warnings 0`, `pnpm run i18n:check`, Prettier checks for all four
  owned source/test files, `git diff --check`, and
  `python3 scripts/lint-spec-files.py --all`.
- Owned files:
  `apps/web/components/quick-chat/quick-chat-delete-dialog.tsx`,
  `apps/web/components/quick-chat/quick-chat-delete-dialog.test.tsx`,
  `apps/web/components/settings/agent-profile-delete-dialog.tsx`, and
  `apps/web/components/settings/agent-profile-delete-dialog.test.tsx`.
