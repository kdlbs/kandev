---
id: "03-e2e-and-verification"
title: "E2E regression coverage and full verification"
status: done
wave: 2
depends_on: ["01-native-renderer-restore", "02-virtuoso-renderer-restore"]
parallel_safe: false
parallelism: sequential
plan: "plan.md"
spec: "../../specs/keep-transcript-position/spec.md"
---

# Task 03: E2E And Verification

## Acceptance

- A new test in `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
  reproduces the bug (fails before tasks 01/02, passes after): seeds an
  overflowing task, leaves auto-scroll **enabled** (no toggle click),
  scrolls to mid-transcript, navigates to the kanban board and back, and
  asserts the restored `scrollTop` is within tolerance of the
  pre-navigation value instead of at the bottom.
- A second case covers the `virtuoso` renderer by appending
  `?renderer=virtuoso` to the task URL (confirmed exact param/values via
  `message-list.tsx`'s `resolveStrategy`) — this is a new pattern in the
  E2E suite (no existing spec parameterizes renderer choice). The test
  must assert the Virtuoso branch actually rendered (e.g. a Virtuoso-only
  DOM attribute/class, not just the absence of a native-only one) before
  asserting on restored position, so a resolver regression can't make
  this silently exercise native instead.
- All existing tests in `auto-scroll-toggle.spec.ts` still pass
  unmodified, including every disabled-state case.
- No temporary/throwaway spec files remain in the repo.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/transcript-auto-scroll.test.ts
cd apps/web && pnpm e2e:run tests/chat/auto-scroll-toggle.spec.ts
cd apps/web && pnpm run typecheck && pnpm run lint
make -C apps/backend fmt
```

## Files Likely Touched

- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`

## Inputs

Tasks 01 and 02's landed fixes; spec Scenarios section; the existing
"preserves the frozen scroll position across navigating away and back"
test as the structural template.

## Output Contract

Report the new test names, their fail-before/pass-after confirmation,
the full verification command output, files touched, and current
plan/task status. Flag if the virtuoso renderer cannot be reliably
activated/asserted on in E2E (e.g. no stable DOM marker) as a blocker
requiring a scope decision rather than a skipped assertion.
