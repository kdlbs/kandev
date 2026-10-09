---
id: "02-status-recovery"
title: "Recover compaction status errors"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.8
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 02: Recover compaction status errors

## Summary

Clear a recovered status-read error on the next successful background poll.
Keep action errors independent so a successful GET cannot hide a failed command.

## In scope

- Separate read and action errors in `useToolPayloadRetention`.
- Preserve its public `error` field, error translation, and manual dismissal behavior.
- Keep lifetime, generation, pending, accepted-operation, and draft protections.
- Extend hook, rendered component, desktop, and phone regression coverage.

## Out of scope

- New layout, user-facing copy, retry policy, polling cadence, and backend APIs.
- Automatic retries of save, analysis, cleanup, or cancellation.

## Acceptance

1. Failed GET then successful background GET removes the banner without clicking Refresh status.
2. Successful GET retains action errors and persisted operation failures. Stale GET results change neither errors nor policy.
3. Desktop and phone preserve policy edits, existing controls, and error recovery without duplicate mutations.

## ASCII UI preview

### UI-01: Compaction error recovery

Entry: Data & Logs > Database. Full preview: [plan](plan.md#ascii-ui-preview).

```text
Read failure:  Messages compaction
               [Existing error] [Refresh status]
Recovery:     Messages compaction
               Tasks inactive for [3] [Months v] [Analyze savings]
Action error: Messages compaction
               [Existing action error] [Refresh status]
```

Phones retain adjacent age/unit inputs and a separate full-width Analyze savings
row. The existing route owns scrolling. Touch targets and focus behavior remain
unchanged. Banner removal is required only for recovered reads (003.7).
Action and persisted errors remain visible (003.8). Spacing is illustrative.

## TDD and test design

First add `clears a recovered status error on background polling` to the hook
test file. Use fake timers: initial success, failed idle poll, successful idle
poll. Assert that the current implementation leaves the error set.

Add cases for initial GET failure, repeated failures, action-error precedence,
successful GET after failed save/analyze/run/cancel, and persisted failed status.
Use deferred responses to cover stale GET success and failure around mutations,
unmount, and remount. Preserve existing same-tick command serialization tests.
Assert that recovery creates no mutation request and does not change drafts.

Add a rendered component regression with the real hook and mocked API module.
Do not prove recovery by directly replacing the mocked hook's error value.

Add `recovers status polling without manual refresh` and
`preserves action failures after status recovery` to both existing E2E files.
Extract response scripting into the existing retention helper when shared.
Intercept only the intended endpoint and HTTP method. Return a real successful
status response after the injected failure. Advance the browser clock to the
next poll, then await the specific GET before checking the banner.
Use `.tap()` for phone actions. Preserve all existing cleanup and geometry cases.

## Verification

Run from the repository root. Install workspace dependencies once when absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/system/use-tool-payload-retention.test.ts components/settings/system/tool-payload-retention-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/system/tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use managed E2E builds and run projects sequentially. Record discovered test
counts, results, and a focused phone screenshot. Do not use production data.

## Files likely touched

- `apps/web/hooks/domains/system/use-tool-payload-retention.ts`
- `apps/web/hooks/domains/system/use-tool-payload-retention.test.ts`
- `apps/web/components/settings/system/tool-payload-retention-card.test.tsx`
- `apps/web/e2e/tests/system/tool-payload-retention.spec.ts`
- `apps/web/e2e/tests/system/mobile-tool-payload-retention.spec.ts`
- `apps/web/e2e/helpers/tool-payload-retention.ts`

## Dependencies

No code dependency on Task 01. Execute after Task 01 as the default package order.

## Risks

A shared unconditional `setError(null)` loses action failures. Keep error origin
explicit. Apply the existing generation guard before any error update.
Controlled browser failures prove UI recovery, not backend maintenance admission.
Task 01 supplies that independent evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/system-page/requirements/tool-payload-retention.md), background operation and access.
- [Design](../../specs/system-page/system-design/tool-payload-retention.md), status error recovery.
- Existing hook, component, and retention E2E tests.

## Results

Implemented separate status-read and action error ownership in
`useToolPayloadRetention`. A successful background status poll now clears only
the recovered read error. Mutation failures and persisted failed-operation
states remain visible, while stale status responses continue to obey the
existing generation and operation guards.

Added hook and rendered-card regressions for read recovery, action-error
preservation, and stale response ordering. Added controlled desktop and phone
browser scenarios that fail one status read, recover on the next poll, and
exercise touch actions on mobile. The existing phone cleanup test continues to
capture the focused 390px card screenshot.

Verification passed:

- `pnpm exec vitest run hooks/domains/system/use-tool-payload-retention.test.ts components/settings/system/tool-payload-retention-card.test.tsx` (23 tests)
- `pnpm run typecheck`
- `pnpm e2e:run --project chromium tests/system/tool-payload-retention.spec.ts` (4 tests)
- `pnpm e2e:run --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts` (4 tests)
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
