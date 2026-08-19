---
id: "02-derived-visibility"
title: "Derive and render empty-column visibility"
status: done
wave: 2
depends_on: ["01-persist-preference"]
plan: "plan.md"
spec: "../../specs/kanban-auto-hide-empty-columns/spec.md"
---

# Task 02: Derive and render empty-column visibility

## Acceptance

- A pure, tested projection derives occupied and auto-hidden live steps after workflow, repository,
  and plugin filters but before free-text search.
- Ordinary Kanban and Pipeline rendering removes auto-hidden empty steps while leaving manual hidden
  semantics unchanged.
- A workflow whose every real step is auto-hidden retains its lane, Columns menu, and contextual
  translated empty state.

## Likely files

- `apps/web/components/kanban/swimlane-container.tsx` and tests
- `apps/web/lib/kanban/workflow-swimlanes.ts` and tests
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban/swimlane-graph-content.tsx`
- `apps/web/src/locales/*/kanban.json`

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/kanban/swimlane-container.test.ts lib/kanban/workflow-swimlanes.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:ratchet)
```

## Risks

- Reusing the fully searched task list would cause layout churn while typing.
- Filtering steps too early can turn valid tasks into orphans.
- All-empty lanes must remain recoverable without restoring full empty columns.
