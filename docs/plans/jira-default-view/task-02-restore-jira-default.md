---
id: 02-restore-jira-default
title: Restore the default Jira view
status: done
wave: 2
depends_on:
  - 01-persist-jira-default
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.1
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.2
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.3
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.4
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.5
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.7
system_design:
  - ../../specs/integrations/system-design/jira-default-view.md
---

# Task 02: Restore the default Jira view

## Summary

Hydrate the saved views and default ID together, then resolve the first Jira search from the selected view. Keep a manual choice ahead of late hydration and preserve the current no-default landing behavior.

## Scope and likely files

- `apps/web/components/jira/my-jira/use-saved-views.ts` and `use-saved-views.test.ts`
- A small pure resolver and focused tests under `apps/web/components/jira/my-jira/`
- `apps/web/app/jira/jira-page-client.tsx` and a focused page-state test
- `apps/web/components/jira/my-jira/use-jira-search.ts` only if its current empty-JQL gate cannot safely delay the initial request

## Exclusions

- No toolbar markup, new URL state, or changes to Jira search composition.

## Implementation acceptance

1. A valid default restores its exact saved filters or custom JQL before the first ticket search; missing defaults and absent preferences use the specified fallback.
2. Set/clear mutations publish only after persistence succeeds; default deletion clears the ID and view together, and a failed write restores prior state.
3. A manual selection during hydration and later default changes do not replace the active view.

## TDD and verification

Add regression tests for a custom JQL default, a deleted ID, the no-default project-key case, manual selection during delayed hydration, and failed/ordered mutations. Confirm expected red failures, implement, then run:

```bash
cd apps/web && pnpm exec vitest run components/jira/my-jira/use-saved-views.test.ts components/jira/my-jira/jira-default-view.test.ts app/jira/jira-page-client.test.tsx
cd apps/web && pnpm run typecheck
```

If a new test file gets a different final name, update this command in the work order before implementation.

Red: the initial frontend run showed that the search fired before settings resolved, readiness/default state was absent from `useSavedViews`, and the resolver/page-state modules did not exist.

Green:

```text
cd apps/web && pnpm exec vitest run components/jira/my-jira/use-saved-views.test.ts components/jira/my-jira/jira-default-view.test.ts app/jira/jira-page-client.test.tsx
19 passed, 0 failed
cd apps/web && pnpm run typecheck
PASS
```

The expanded focused run also passed all 28 tests across those three files, search gating, and project-status hydration.

## Dependencies and risks

Depends on Task 01's settings field. `useSavedViews` currently stages mutations before hydration and uses a queued array write; extend that queue without dropping concurrent view saves or default changes. `useJiraSearch` must never run the old query first when a saved default exists.

## Result

Saved views and the default ID hydrate from the same response. An available built-in or custom view restores its saved filters and exact custom JQL before the first ticket search. Missing IDs use the existing Assigned to me filters with the workspace default project key. Selection and filter edits made during hydration win over the late response. Default writes publish only after acknowledgment; deleting the default clears its ID and saved-view row in one PATCH, and a failed write retains both.
