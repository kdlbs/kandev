---
id: "01-reconcile-transcript-scroll-on-activation"
title: "Reconcile transcript scroll on session activation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.8
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.9
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.10
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Reconcile Transcript Scroll on Session Activation

## Summary

Make native transcript placement visibility-aware and restore bottom-follow
intent after a persistent Dockview session panel becomes visible. Prove the
repair with failing hook regressions and real two-session browser flows.

## Failing regressions first

Add focused cases to `message-list-native.test.tsx` before production changes:

1. Mount populated scroll management with `isVisible=false`, verify that no
   initial bottom write occurs, simulate the generic one-frame absolute-offset
   restore, activate the panel, and require the later placement to win.
2. Append while hidden and require a bottom-following transcript to catch up
   only after activation.
3. Require disabled and manually-away transcripts to keep their saved offsets
   through the same inactive-to-active transition.
4. Require scheduled activation work to cancel when visibility changes again or
   when another scroll owner becomes active.
5. Mount a hidden transcript, let the original settling interval elapse, then
   activate it with an unread divider and prove the activation-scoped
   reassertion window still gives the divider priority.
6. Exercise `SessionPanelContent` with controllable `ResizeObserver` and
   `requestAnimationFrame` delivery to prove unchanged offsets restore and
   newer scroll positions win.

The current code consumes the hidden initial latch and writes against hidden
geometry, so the first two cases must fail before the repair.

## In scope

- Thread `isVisible` through native initial placement, divider placement, and
  auto-follow behavior.
- Add the bounded post-panel-restore activation reconciliation.
- Preserve bottom-follow intent without new persisted state.
- Preserve disabled, manual, unread-divider, explicit navigation, layout
  restoration, pagination, and prepend behavior.
- Add desktop two-session Playwright regressions for refresh activation, hidden
  live content, and disabled-position preservation.
- Run the existing mobile auto-scroll Playwright suite.
- Add direct coverage for the shared `SessionPanelContent` restore guard.
- Keep the generic panel restore from overwriting a newer transcript or user
  scroll scheduled in the same visibility transition.
- Update plan/work-order results and statuses after implementation.

## Out of scope

- Changing `SessionPanelContent` into a chat-aware component.
- Changing the Dockview portal lifecycle or session-tab reconciliation.
- New copy, controls, settings, persisted fields, or responsive layout.
- New mobile navigation or a mobile hidden-panel model.

## Acceptance

- An enabled inactive transcript does not consume initial placement before it
  is visible and opens at the newest message after refresh.
- Hidden live content catches up on activation only when bottom-follow intent
  remains active.
- Disabled and manually-away positions survive a session-tab round trip.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.test.ts
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.tsx components/task/chat/message-list-native.test.tsx e2e/tests/chat/auto-scroll-toggle.spec.ts
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/auto-scroll-toggle.spec.ts -- --grep "inactive session transcript" --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts -- --retries=0
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web/components/task/chat apps/web/e2e/tests/chat
```

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/transcript-auto-scroll.ts`
- `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/packages/ui/src/pannel-session.tsx`
- `apps/web/components/task/session-panel-content.test.tsx`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `docs/specs/ui/requirements/transcript-auto-scroll.md`
- `docs/specs/ui/system-design/transcript-auto-scroll.md`
- `docs/plans/transcript-session-activation-scroll/plan.md`
- `docs/plans/transcript-session-activation-scroll/task-01-reconcile-transcript-scroll-on-activation.md`

## Dependencies

None.

## Risks

- Post-restore placement must run after the generic restore without adding an
  unbounded observer or timer. The generic restore skips its write when the
  element changed after scheduling, so the two-frame reconciliation can own the
  newer position.
- The activation path must re-check live guards so stale animation frames cannot
  override a newer user or layout action.
- Browser evidence must use a real inactive Dockview tab; merely hiding a test
  div does not reproduce the persistent-portal lifecycle.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001` acceptance criteria 8 through 10.
- `docs/specs/ui/system-design/transcript-auto-scroll.md`.
- The existing native scroll-management hook harness.
- The existing desktop and mobile transcript auto-scroll Playwright suites.

## Results

Implemented visibility-aware native placement, two-frame activation
reconciliation, hidden-update catch-up, reader-position preservation, the
activation-scoped divider settling window, and the cooperative stale
generic-restore guard.

The PR fixup pass added the shared `useActivationPending` lifecycle helper,
preserved the last visible disabled-reader offset through immediate hide and
unmount cleanup, and covered cancellation before the first activation frame.

Checks passed:

- Focused Vitest tests, including the hidden-activation settling-window
  regression and direct shared-panel restore coverage.
- Web TypeScript typecheck.
- Targeted ESLint for changed web files.
- Desktop inactive-session Playwright regression, 1 passed.
- Mobile auto-scroll Playwright suite, 5 passed.
- Specification tests and full specification lint.
- Scoped `git diff --check`.
