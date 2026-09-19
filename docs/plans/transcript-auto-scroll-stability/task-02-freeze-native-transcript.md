---
id: "02-freeze-native-transcript"
title: "Freeze disabled native transcript"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/transcript-auto-scroll.md"
---

# Task 02: Freeze disabled native transcript

## Acceptance

- Disabling auto-scroll from the bottom prevents native browser anchoring from
  advancing the transcript after a live message arrives.
- Enabled auto-scroll remains bottom-pinned.
- Desktop and Pixel-5 mobile tests cover the bottom-anchor case.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
cd web
pnpm e2e:run tests/chat/auto-scroll-toggle.spec.ts -- --grep 'disabling while genuinely at the bottom'
pnpm e2e:run tests/chat/mobile-auto-scroll-toggle.spec.ts
```

## Files likely touched

- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`

## Dependencies

None.

## Parallelism

Sequential.

## Output contract

Report the scroll-owner style change, desktop/mobile verification results,
files changed, and updated task and plan status.
