---
id: "01-preserve-read-cursor-across-task-switches"
title: "Preserve read cursor across task switches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-UNREAD-DIVIDER-001
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-OFFICE-UNREAD-DIVIDER-001.3
  - AC-OFFICE-UNREAD-DIVIDER-001.4
  - AC-OFFICE-UNREAD-DIVIDER-001.7
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14
system_design:
  - ../../specs/office/system-design/unread-divider.md
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Preserve Read Cursor Across Task Switches

## Summary

Keep each session's successful mark-read response eligible across unrelated
task activity, then expose enough debug evidence to distinguish cursor races,
divider ownership, and running-agent bottom writes. Prove the user flow with
completed tasks on desktop and mobile.

## In scope

- Add a bounded per-session latest-request coordinator.
- Preserve same-session out-of-order response rejection.
- Add `messages:read-tracking` events and extend
  `messages:scroll-placement` events.
- Add focused hook and native-scroll tests.
- Add desktop and mobile completed-task switch regressions with a shared
  mark-read response hold.

## Out of scope

- Backend schema, endpoint, or monotonic repository changes.
- New UI, copy, navigation, or auto-scroll preferences.
- Changes to genuine unread-divider placement.

## Acceptance

- A delayed successful response for task A updates A's local cursor even after
  task B dispatches, unless a newer task A request superseded it.
- Returning to completed task A after visiting completed task B shows no stale
  divider and leaves the enabled transcript at the bottom without any new
  running or message event.
- Debug output identifies visit capture, per-session request freshness,
  placement delegation, and later work/message bottom writes without logging
  content or hot-loop events.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/use-session-read-tracking.test.ts components/task/chat/message-list-native.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/task/chat/use-session-read-tracking.ts components/task/chat/use-session-read-tracking.test.ts components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx lib/debug/log.ts e2e/helpers/mark-read-response-hold.ts e2e/tests/chat/unread-divider.spec.ts e2e/tests/chat/mobile-unread-divider.spec.ts
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/unread-divider.spec.ts -- --grep "completed task switch" --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-unread-divider.spec.ts -- --grep "completed task switch" --retries=0
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/use-session-read-tracking.ts`
- `apps/web/components/task/chat/use-session-read-tracking.test.ts`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/lib/debug/log.ts`
- `apps/web/e2e/helpers/mark-read-response-hold.ts`
- `apps/web/e2e/tests/chat/unread-divider.spec.ts`
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts`
- `docs/specs/office/system-design/unread-divider.md`
- `docs/specs/office/README.md`

## Dependencies

None.

## Risks

- The request coordinator must retain only in-flight entries and fence older
  same-session callbacks after the latest request settles.
- E2E routing must let the backend complete task A's write while delaying only
  browser delivery of the response.
- Placement diagnostics must stay bounded during streaming.

## Parallelism

`sequential`

## Inputs

- `REQ-OFFICE-UNREAD-DIVIDER-001`, especially acceptance criteria `.3`, `.4`,
  and `.7`.
- Office unread-divider and UI transcript auto-scroll system designs.
- Existing read-tracking, native-scroll, and unread-divider test patterns.

## Results

- Replaced the hook-local cross-task freshness ref with a bounded per-session
  generation map while preserving same-session stale-response rejection.
- Added `messages:read-tracking` lifecycle logs and bounded
  `messages:scroll-placement` owner/write logs.
- Added focused unit coverage plus desktop and mobile completed-task switch
  regressions that hold task A's accepted response until task B dispatches.
- Verification passed: 74 focused unit tests, typecheck, scoped ESLint, one
  Chromium E2E, one Mobile Chrome E2E, 30 specification-linter tests, the full
  specification lint, and `git diff --check`.
- Review remediation: delayed mark-read responses now also require the cached
  cursor to match the dispatch snapshot, preventing an external hydration from
  regressing local state. The deferred hydration regression passes locally.
