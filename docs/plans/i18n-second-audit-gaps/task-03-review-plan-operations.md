---
id: "03-review-plan-operations"
title: "Localize review and plan operations"
status: done
wave: 3
depends_on: ["02-azure-shortcut-metadata"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-second-audit-gaps.md"
---

# Task 03: Review And Plan Operations

## Acceptance

- Review finding, send-to-agent, review-dialog discard, walkthrough request, and task-plan fallbacks use
  active-locale copy at operation time.
- Fetch, save, delete, revision-load, and revert plan fallbacks are all covered.
- Server error detail and agent-facing prompt content remain untranslated.

## Likely Files

The three review files named in the spec, `apps/web/hooks/domains/session/use-task-plan.ts`,
`use-request-changes-walkthrough.ts`, their focused tests, catalogs, and guard configuration.

## Verification

```bash
cd apps/web && pnpm test -- --run hooks/domains/session/use-request-changes-walkthrough.test.ts components/review/review-dialog-pr-state.test.tsx && pnpm run lint:i18n -- hooks/domains/review hooks/domains/session/use-task-plan.ts hooks/domains/session/use-request-changes-walkthrough.ts components/review/review-dialog.tsx
```

## Risks

Callback dependency lists must include translation resolution so live locale changes do not retain stale copy.

## Results

Review, walkthrough, discard, and all five task-plan fallback paths resolve active-locale copy at
operation time while raw server detail and prompt content remain unchanged.
