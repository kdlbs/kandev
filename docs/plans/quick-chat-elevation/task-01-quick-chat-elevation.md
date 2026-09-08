---
id: "01-quick-chat-elevation"
title: "Add Quick Chat elevation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-chat-elevation.md"
---

# Task 01: Add Quick Chat elevation

## Acceptance

1. Tablet and desktop Quick Chat opens above a subtle non-transparent backdrop while retaining its
   existing panel shadow, size, position, content, and close behavior.
2. Closing Quick Chat removes the backdrop and restores the page; the existing mobile Quick Chat
   entry, full-screen composition, close control, and no-overflow behavior remain intact.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web run typecheck
pnpm --dir web e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep "elevation"
pnpm --dir web e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts
```

The E2E test is written and run RED before changing the overlay class, then rerun GREEN after the
minimal frontend change. The final E2E commands use the managed runner so the production Vite build
is refreshed before browser verification.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`

Existing mobile coverage used as evidence:

- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`

## Dependencies

None. Reuse the existing `DialogOverlay`, Quick Chat modal, helpers, and mobile entry coverage.

## Parallelism

Sequential. The test and the one-line overlay change describe the same rendered contract and must
be verified together.

## Inputs

- `docs/specs/ui/requirements/quick-chat-elevation.md`
- `docs/plans/quick-chat-elevation/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Risks

- Do not replace the transparent overlay globally; the change is scoped to Quick Chat and should
  not alter other dialogs or the separate new-chat picker.
- Avoid changing phone geometry: the full-screen panel covers the backdrop by design.

## Results

- Added the desktop rendered contract test in `apps/web/e2e/tests/chat/quick-chat.spec.ts`.
- Confirmed RED before the production change: the overlay computed to `rgba(0, 0, 0, 0)`.
- Changed only the Quick Chat overlay class from `bg-transparent` to `bg-black/20`; the existing
  `shadow-2xl` panel and mobile full-screen layout remain unchanged.
- Typecheck passed.
- Focused desktop E2E passed (1 test) and existing mobile entry E2E passed (4 tests).
- Managed desktop/mobile PR screenshot capture passed; generated assets remain ignored in
  `apps/web/.pr-assets/` for PR publication.

## Output contract

Report the RED and GREEN E2E evidence, changed files, exact verification results, blockers or
residual risks, and update this task plus `plan.md` to `done` when implementation is complete.
