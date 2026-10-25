---
id: "01-quick-chat-backdrop-strength"
title: "Strengthen Quick Chat backdrop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-CHAT-ELEVATION-001
acceptance_criteria:
  - AC-UI-QUICK-CHAT-ELEVATION-001.1
  - AC-UI-QUICK-CHAT-ELEVATION-001.2
  - AC-UI-QUICK-CHAT-ELEVATION-001.3
  - AC-UI-QUICK-CHAT-ELEVATION-001.4
  - AC-UI-QUICK-CHAT-ELEVATION-001.5
  - AC-UI-QUICK-CHAT-ELEVATION-001.6
  - AC-UI-QUICK-CHAT-ELEVATION-001.7
system_design:
  - ../../specs/ui/system-design/quick-chat-elevation.md
---

# Task 01: Strengthen Quick Chat backdrop

## Summary

Apply a stronger, surface-local backdrop treatment to the shared Quick Chat
dialog so conversations and Quick Terminal tabs read as elevated on tablet and
desktop pages. Extend the existing desktop rendered regression and run the
existing mobile Quick Chat scenarios to preserve the full-screen phone flow.

## In scope

- Update the `DialogContent` overlay class in
  `apps/web/components/quick-chat/quick-chat-modal.tsx`.
- Extend the existing elevation scenario in
  `apps/web/e2e/tests/chat/quick-chat.spec.ts` to cover light backdrop blur.
- Verify the existing mobile entry, close, containment, and overflow behavior.

## Out of scope

- Changing shared dialog primitives or unrelated dialog overlays.
- Changing the new-Quick-Chat picker, Quick Chat state, terminal state, tab
  behavior, or mobile composition.

## Acceptance

1. Tablet and desktop Quick Chat opens above a clearly visible darkened and
   lightly blurred backdrop while its existing panel shadow and geometry remain
   unchanged.
2. Closing Quick Chat removes the dialog and backdrop, preserving the current
   focus and interaction lifecycle.
3. The existing mobile Quick Chat entry and explicit close control remain
   usable with no document horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web run typecheck
cd web
pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep "elevation"
pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts
```

The desktop rendered assertion should be run RED before the overlay class is
changed, then GREEN after the minimal frontend change. The managed E2E runner
rebuilds production assets before browser verification.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`

Existing mobile evidence:

- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`

## Dependencies

None.

## Risks

- Keep the stronger treatment scoped to `QuickChatModal`; do not alter global
  dialog defaults or the separate picker dialog.
- Avoid changing the phone full-screen geometry because the panel naturally
  covers the backdrop there.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/quick-chat-elevation.md`
- `docs/specs/ui/system-design/quick-chat-elevation.md`
- `docs/plans/quick-chat-backdrop-strength/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

Implemented the responsive surface-local
`bg-black/20 sm:bg-black/40 sm:backdrop-blur-sm` treatment in `QuickChatModal`,
extended the desktop rendered regression to assert the backdrop filter, and
added a mobile regression for the lighter no-filter phone treatment.

Verification passed:

- `pnpm --filter @kandev/web run typecheck`
- Desktop elevation E2E: 1 test passed.
- Mobile Quick Chat E2E: 4 tests passed.
- Focused mobile backdrop E2E: 1 test passed.

The desktop regression was confirmed RED before the production class change:
the computed overlay filter was `none`.

The mobile regression was confirmed RED against the unscoped treatment: the
computed backdrop was `oklab(0 0 0 / 0.4)` with `blur(8px)` instead of the
lighter phone treatment.
