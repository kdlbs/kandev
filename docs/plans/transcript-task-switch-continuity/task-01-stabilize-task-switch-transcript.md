---
id: "01-stabilize-task-switch-transcript"
title: "Stabilize task-switch transcript entry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.12
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.14
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Stabilize Task-switch Transcript Entry

## Summary

Place cached transcript rows at the incoming task's intended position before
the latest-window refresh resolves. Reconcile the position afterward, and
require current sentinel geometry before older-history pagination starts.

## In scope

- Add token-scoped provisional and final task-entry placement phases.
- Use the incoming session's auto-scroll preference and saved position.
- Preserve unread-divider, explicit-navigation, and layout-restore priority.
- Keep older-history pagination blocked through final placement.
- Validate current geometry on eligibility retry and stale-view handoff.
- Add bounded development diagnostics.
- Add TDD regressions and desktop/mobile Playwright coverage.

## Out of scope

- Changing unread-divider behavior, auto-scroll persistence, backend cursors,
  message page size, or task navigation UI.

## Acceptance

- With cached messages and auto-scroll enabled, switching tasks shows the
  incoming task's newest cached message before a held refresh response and the
  final newest message after release. The browser-default top is never exposed.
- With auto-scroll disabled, the same flow uses the incoming session's saved
  reading position before and after refresh. It never reuses the outgoing
  transcript's offset.
- Releasing the placement block or transferring a stale view cannot start
  pagination from an old observer entry. A sentinel currently inside the
  preload region still loads older history.
- Desktop and mobile browser tests prove the cached-entry behavior, and request
  tracing proves task entry sends no unwanted older-page request.

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/transcript-auto-scroll.ts`
- `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
- `apps/web/hooks/use-lazy-load-sentinel.ts`
- `apps/web/hooks/use-lazy-load-sentinel.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-messages.test.ts`
- `apps/web/e2e/helpers/ws-response-hold.ts`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`

The response-hold helper is optional. Keep the WebSocket control local to a
single test file if it is not shared by both viewport suites.

## Verification

Follow TDD. Run the new focused tests before the production change and record
the expected failures. Then run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run hooks/use-lazy-load-sentinel.test.ts components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.test.ts hooks/domains/session/use-session-messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-lazy-load-sentinel.ts hooks/use-lazy-load-sentinel.test.ts components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.tsx components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.ts components/task/chat/transcript-auto-scroll.test.ts hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-messages.test.ts e2e/helpers/ws-response-hold.ts e2e/tests/chat/auto-scroll-toggle.spec.ts e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/chat/auto-scroll-toggle.spec.ts -- --grep "environment-changing task switch" --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts -- --grep "task switch" --retries=0)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If the optional helper is not created, omit it from the ESLint command. If the
managed Playwright runner does not accept `--no-build` after the desktop run,
remove only that flag and keep the same mobile project and grep.

## Dependencies

None.

## Parallelism

`sequential`. This task owns the shared placement and pagination lifecycle.

## Risks

- Do not clear the switch token during provisional placement.
- Do not let provisional placement override an unread-divider target.
- Re-resolve the live scroll element and session before every deferred write.
- Keep the geometry predicate optional for non-transcript sentinel consumers.
- Assert geometry while the refresh is held. An eventual-bottom assertion does
  not detect the visible top flash.

## Mobile design contract

- Use the current mobile task switcher and full-height Chat tab.
- Keep the transcript as the only vertical scroll owner.
- Keep the existing header, composer, touch targets, and safe-area geometry.
- Exercise the same cached-entry and final-reconciliation contract as desktop.
- Prove the task view has no horizontal document overflow after both phases.

## Output contract

Report the red and green evidence, placement ownership changes, sentinel
geometry rule, changed files, desktop/mobile browser results, and remaining
risks. Update this work order and `plan.md` with implementation results in the
same conversation.

## Results

- Added token-scoped provisional and final transcript placement. Cached enabled
  transcripts place at the bottom before refresh; cached disabled transcripts
  use the incoming session's saved position. Final placement reconciles current
  rows and clears only the matching token.
- Preserved unread-divider, layout-restore, explicit-navigation, and
  programmatic-scroll ownership.
- Added current-geometry checks to sentinel eligibility retry and stale-view
  handoff. Consumers without a geometry predicate retain their previous
  behavior.
- Added a correlated WebSocket latest-window response hold for browser tests.
  Desktop and mobile tests exercise both enabled and disabled cached returns.
  Desktop request tracing also proves no older-page HTTP request starts during
  placement release.
- Red evidence: four focused unit assertions failed on the old behavior. The
  desktop E2E remained 1,892 px from the bottom while refresh was held.
- Green evidence: 142 focused unit tests passed; targeted ESLint and typecheck
  passed; focused Chromium and mobile-chrome runs each passed one test with
  retries disabled; specification lint and diff checks passed.
