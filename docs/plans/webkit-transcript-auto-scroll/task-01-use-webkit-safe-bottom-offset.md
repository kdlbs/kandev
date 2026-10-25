---
id: "01-use-webkit-safe-bottom-offset"
title: "Use a WebKit-safe bottom offset"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Use a WebKit-safe Bottom Offset

## Summary

Use the largest signed 32-bit integer for write-only bottom placement. Add a
regression that protects WebKit behavior and the no-layout-read contract.

## Failing regression first

Add a test named `uses a WebKit-safe maximum offset for pinned appends`.
Require the setter to receive `2_147_483_647`. The current test receives
`Number.MAX_SAFE_INTEGER`, so the regression fails before the correction.

## In scope

- Change the native bottom offset constant.
- Keep `scrollNativeToBottom` write-only.
- Update the focused unit regression.
- Run the existing desktop and mobile live-message scenarios.
- Update the transcript auto-scroll design with the WebKit range constraint.

## Out of scope

- Other transcript scroll paths or controls.
- Pagination, prepend restoration, and explicit message navigation.
- New user-facing copy or responsive composition.
- Playwright configuration and CI browser installation.

## Acceptance

- Live append writes `2_147_483_647` without reading content size.
- Safari-compatible WebKit clamps the request to the transcript bottom.
- Existing desktop and mobile live-message scenarios remain green.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/message-list-native.test.tsx
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web/components/task/chat
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/auto-scroll-toggle.spec.ts -- --grep "enabled auto-scroll stays at the bottom for live messages" --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts -- --grep "enabled auto-scroll stays at the bottom for live messages on mobile" --retries=0
```

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `docs/specs/ui/system-design/transcript-auto-scroll.md`
- `docs/plans/webkit-transcript-auto-scroll/plan.md`
- `docs/plans/webkit-transcript-auto-scroll/task-01-use-webkit-safe-bottom-offset.md`

## Dependencies

None.

## Risks

- The sentinel must remain positive after WebKit converts the scroll offset.
- A regression that only checks Chromium cannot detect this browser difference.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001` acceptance criteria 5 and 7.
- The bottom-placement section in the transcript auto-scroll design.
- The existing native-scroll unit harness and live-message browser scenarios.

## Results

- Changed the native bottom sentinel from `Number.MAX_SAFE_INTEGER` to
  `2_147_483_647`.
- Added a regression that keeps the append path write-only and requires the
  WebKit-safe offset.
- The regression failed on the old sentinel and passed after the correction.
- All 27 focused unit tests passed.
- Typecheck, targeted ESLint, specification lint, and diff checks passed.
- The desktop Chromium and mobile Chrome live-message scenarios passed with
  retries disabled.
- Direct Chromium, Firefox, and WebKit probes accepted the new offset and
  placed the 900 px test container at its bottom.
