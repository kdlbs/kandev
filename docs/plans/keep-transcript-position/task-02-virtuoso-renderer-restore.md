---
id: "02-virtuoso-renderer-restore"
title: "Virtuoso renderer restores captured snapshot regardless of auto-scroll state"
status: done
wave: 1
depends_on: []
parallel_safe: true
parallelism: sequential
plan: "plan.md"
spec: "../../specs/keep-transcript-position/spec.md"
---

# Task 02: Virtuoso Renderer Restore

## Acceptance

- `restoreStateFrom`'s lazy initializer in `message-list-virtuoso.tsx`
  returns the captured `virtuosoStateBySessionId[sessionId]` snapshot
  whenever `sessionId` and a snapshot exist, regardless of the `enabled`
  flag (drop `enabled ||` from the guard). No new exported function —
  the guard stays a single inline condition.
- The doc comment above the guard no longer says "when disabled"; it
  describes restoring the captured snapshot unconditionally, falling
  back to Virtuoso's default (last item) only when nothing was captured.
- Re-enable catch-up semantics (`shouldCatchUpOnAutoScrollEnable`) and
  the disable-transition `captureSnapshot`/`captureBaseline` calls are
  untouched — this task only changes the mount-time read gate.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run components/task/chat`
(no new unit test target — this gate has no extracted pure function;
covered by task 03's E2E case)

## Files Likely Touched

- `apps/web/components/task/chat/message-list-virtuoso.tsx`

## Inputs

Spec Data model, Failure modes, and Scenarios sections; plan Root Cause
and Frontend sections. `captureSnapshot` already writes on every unmount
unconditionally — no capture-side change needed.

## Output Contract

Report the exact diff to the `restoreStateFrom` guard, confirmation that
capture-side code and re-enable catch-up logic were untouched, files
touched, and any doc-comment updates. Mark this task and its plan entry
done.
