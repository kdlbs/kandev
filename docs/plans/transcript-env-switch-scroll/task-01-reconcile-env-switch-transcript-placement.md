---
id: "01-reconcile-env-switch-transcript-placement"
title: "Reconcile environment-switch transcript placement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.12
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Reconcile Environment-switch Transcript Placement

## Summary

Arm a session-scoped placement request for environment-changing Dockview
switches, defer the incoming transcript until its layout and latest history are
ready, and block automatic older-history pagination during that window. Prove
that enabled and disabled sessions resolve from their own state while same-
environment and ordinary layout behavior remain unchanged.

## Failing regressions first

1. Add a store action test that performs an `env-a` to `env-b` switch and
   requires a tokened request for the incoming session while
   `pendingChatScrollTop` remains null. Require the same-environment early
   return not to arm a request.
2. Add a native-scroll hook case with outgoing and incoming sessions, distinct
   saved offsets, a matching environment-switch token, and a pending history
   refresh. Require no initial write or pagination eligibility until the
   incoming rows settle.
3. Complete the refresh with a larger scroll height and require an enabled
   session to land at the current bottom.
4. Repeat with auto-scroll disabled and require the incoming session's saved
   offset, never the outgoing session's offset.
5. Add a cached-entry session-message test that holds the background
   `message.list` promise and requires the placement-readiness signal to remain
   pending until that exact refresh settles.
6. Add a desktop Playwright case that switches between two overflowing tasks
   in different environments and fails on the current top reset.

## In scope

- Add and consume the tokened Dockview transcript-placement request.
- Extend activation pending to support a visible external reactivation token.
- Expose cached history-refresh readiness without changing loading copy or
  presentation.
- Block the older-history sentinel while environment placement is unresolved.
- Cover the env-switch store action, absolute scroll preservation isolation,
  initial placement, session-specific saved offset, pagination block, and
  same-environment behavior.
- Add the desktop differing-environment browser regression and run the existing
  mobile auto-scroll suite.
- Update plan and work-order results after implementation.

## Out of scope

- Reusing `preserveChatScrollDuringLayout` for the environment switch.
- Changing saved-offset persistence, auto-scroll controls, pagination page
  sizes, or message fetching contracts.
- Changing maximize, un-maximize, preset/custom layout, explicit navigation,
  unread divider, or live-tailing behavior.
- New copy, layout, responsive composition, or mobile navigation.

## Acceptance

- A cross-environment switch places an enabled incoming transcript at the
  bottom after its latest history settles, without applying the outgoing
  session's offset.
- A disabled incoming transcript restores only its own saved offset.
- Automatic top-intersection pagination remains blocked until placement is
  complete, and same-environment switching retains the current path.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/state/dockview-env-switch-action.test.ts lib/state/dockview-scroll-preserve.test.ts hooks/domains/session/use-session-messages.test.ts components/task/chat/transcript-auto-scroll.test.ts components/task/chat/message-list-native.test.tsx
cd apps && NODE_ENV=production pnpm --filter @kandev/web test -- lib/state/dockview-env-switch-action.test.ts lib/state/dockview-scroll-preserve.test.ts hooks/domains/session/use-session-messages.test.ts components/task/chat/transcript-auto-scroll.test.ts components/task/chat/message-list-native.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/auto-scroll-toggle.spec.ts -- --grep "environment-changing task switch" --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts -- --retries=0
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui docs/plans/transcript-env-switch-scroll apps/web/components/task/chat apps/web/components/task/task-chat-panel.tsx apps/web/hooks/domains/session apps/web/lib/state apps/web/e2e/tests/chat
```

Run `pnpm install --frozen-lockfile` from `apps/` before the first package
command in a fresh worktree.

## Files likely touched

- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-env-switch-action.test.ts`
- `apps/web/lib/state/dockview-scroll-preserve.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-messages.test.ts`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/transcript-auto-scroll.ts`
- `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`

## Dependencies

None.

## Risks

- A request that is not session- and token-checked can clear or reposition a
  later task switch.
- Placement must wait for both layout and cached-history refresh readiness, not
  only one signal.
- The sentinel block must be released after the final placement write, not when
  the request is first observed.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001`, criteria 11 through 13.
- `docs/specs/ui/system-design/transcript-auto-scroll.md`, especially
  Environment-change rebuild lifecycle.
- Existing env-switch store-action and native-scroll hook harnesses.
- Existing desktop and mobile transcript auto-scroll Playwright suites.

## Results

- Added a session-scoped, tokened Dockview placement request that is armed only
  by environment-changing switches and never captures the outgoing transcript's
  absolute offset.
- Added cached-entry history-refresh readiness and delayed native placement
  until both Dockview and message history settle. Auto-scroll writes and the
  older-history sentinel stay inactive while the request owns placement.
- Added unit regressions for enabled bottom placement, disabled incoming offset,
  stale-token completion, same-environment behavior, pagination blocking, and
  isolation from absolute layout restore.
- Added a desktop Playwright regression using two confirmed distinct task
  environments. It covers enabled and disabled return trips through the task
  sidebar.
- Verification passed: focused Vitest in test and production modes (128 tests),
  full Vitest (1,756 files and 15,076 tests, with 4 skipped), typecheck, lint,
  Prettier, spec lint, desktop Playwright (1 test), and mobile Playwright (5
  tests).
