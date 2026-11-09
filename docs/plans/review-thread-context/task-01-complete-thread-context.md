---
id: "01-complete-thread-context"
title: "Complete supplied thread context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001
acceptance_criteria:
  - AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.1
  - AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.2
  - AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.3
  - AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.4
  - AC-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001.5
system_design:
  - ../../specs/integrations/system-design/review-thread-context.md
---

# Task 01: Complete supplied thread context

## Summary

Make the existing whole-thread action include the complete supplied direct
thread across author groups, including when the displayed fragment has no
visible replies. Preserve current display grouping and provider interactions.

## In scope

- Implement complete-thread association and scoped mobile action accessibility
  in the shared host detail, with independent TDD coverage at the actual
  `onAddContext` boundary.
- Verify desktop and phone Review flows with seeded mixed feedback and the
  actual pending context outcome.

## Out of scope

- Provider mapping or transport changes, plugin implementations, arbitrary
  nesting, display regrouping, and new copy.

## Acceptance

- Each visible eligible fragment emits its complete supplied thread exactly
  once in the existing format and order, including two-comment mixed threads,
  homogeneous threads and missing-parent cases.
- Human/bot visibility, toggle, individual/all actions and callback identities
  remain unchanged; a missing callback or standalone root offers no thread action.
- Desktop and phone real UI paths reach complete pending context; the phone
  action has a visible 44-by-44 target and causes no document overflow.

## ASCII UI preview

UI-01: shared Comments composition ([full preview](plan.md#ascii-ui-preview));
AC-001.1, .2, .4 and .5.

```text
Comments (2)                       [add all]
  human root: Please update         [+ thread]
> Bot comments (1)
```

The hidden bot reply is included in the action; a human reply to a bot root
stays visible and adds the complete supplied thread. Phone uses the same Review
scroll surface and a touch-sized action; desktop remains compact.

## Verification

After the later implementation authorization, install dependencies once in a
fresh worktree, then perform the focused checks serially under the user's heavy
work grant. The managed E2E runner rebuilds the current web/backend artifacts.
Use task-owned unique `KANDEV_RUN_QUIET_DIR` values if invoking run-quiet.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/integrations/change-request-detail.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/integrations/change-request-detail-comments.tsx components/integrations/change-request-detail.test.tsx components/github/pr-shared.tsx e2e/helpers/review-thread-context.ts e2e/tests/pr/pr-detail-thread-context.spec.ts e2e/tests/pr/mobile-pr-detail-thread-context.spec.ts -- --retries=0 --trace=on)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-detail-thread-context.spec.ts -- --retries=0 --trace=on)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-detail-thread-context.spec.ts -- --retries=0 --trace=on)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/integrations/change-request-detail-comments.tsx`
- `apps/web/components/github/pr-shared.tsx` (only if a scoped action prop is needed)
- `apps/web/components/integrations/change-request-detail.test.tsx`
- `apps/web/e2e/helpers/review-thread-context.ts`
- `apps/web/e2e/tests/pr/pr-detail-thread-context.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-detail-thread-context.spec.ts`
- This work order, plan, and paired specifications for delivery status.

## Dependencies

None. The user must separately authorize implementation after reviewing this
package. The global heavy-work grant governs installation and checks.

## Risks

A filtered fragment may start at a reply rather than its actual root. Do not
change reply action identities when associating its complete context. Verify
real phone geometry because the current shared button is compact.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/integrations/requirements/review-thread-context.md)
- [System design](../../specs/integrations/system-design/review-thread-context.md)
- Existing shared component, GitHub mapping and PR detail E2E patterns.

## Results

19 rendered component tests passed; scoped ESLint passed. Default frontend
typecheck exited 134 at the V8 4 GiB heap limit; a 6144 MiB retry passed.
Fresh managed desktop and mobile Playwright scenarios passed with one worker,
zero retries and trace enabled. Specification catalog and lint validation passed.
The desktop and phone captures were inspected and compressed.
