---
id: "01-native-renderer-restore"
title: "Native renderer restores captured offset regardless of auto-scroll state"
status: done
wave: 1
depends_on: []
parallel_safe: true
parallelism: sequential
plan: "plan.md"
spec: "../../specs/keep-transcript-position/spec.md"
---

# Task 01: Native Renderer Restore

## Acceptance

- `resolveNativeInitialScrollTop` returns `savedScrollTop ?? scrollHeight`
  whenever there is no pending layout restore, regardless of the
  `enabled` flag; the `enabled` field is removed from its params type.
- `useInitialScrollPosition` in `message-list-native.tsx` no longer
  passes `enabled` to the resolver; its doc comment reflects "always
  restore the captured offset, else bottom" instead of "bottom when
  enabled."
- The existing enabled-and-following live behavior (new messages pull
  the view to the bottom while mounted) is unaffected — this task only
  touches the mount-time resolver, not `useAutoScroll`'s scroll handling
  or `useCatchUpOnReEnable`.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/transcript-auto-scroll.test.ts`

## Files Likely Touched

- `apps/web/components/task/chat/transcript-auto-scroll.ts`
- `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
- `apps/web/components/task/chat/message-list-native.tsx`

## Inputs

Spec Data model, Failure modes, and Scenarios sections; plan Root Cause
and Frontend sections. Reuse the existing pending-layout-restore
short-circuit unchanged.

## Output Contract

Report the exact diff to `resolveNativeInitialScrollTop`'s signature and
behavior, the updated/added unit test cases and their pass/fail
transition (must fail before the fix, pass after), files touched, and
any doc-comment updates. Mark this task and its plan entry done.
