---
id: "05-isolated-ui-leaks"
title: "Localize isolated UI leaks"
status: done
wave: 5
depends_on: ["04-production-operation-audit"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-second-audit-gaps.md"
---

# Task 05: Isolated UI Leaks

## Acceptance

- The task PR shortcut and Mermaid toast use localized owned fallbacks without translating remote details.
- The orphan swimlane title is localized while its sentinel, filtering, and move-target exclusion remain stable.
- Existing focused tests cover toast behavior and orphan-step semantics.

## Likely Files

`apps/web/components/task/task-pr-shortcut.tsx`, `components/shared/mermaid-error-toast.tsx`,
`components/kanban/swimlane-kanban-content.tsx`, their tests, catalogs, and guard configuration.

## Verification

```bash
cd apps/web && pnpm test -- --run components/task/task-pr-shortcut.test.tsx components/shared/mermaid-error-toast.test.tsx components/kanban/orphan-move-target.test.ts && pnpm run lint:i18n -- components/task/task-pr-shortcut.tsx components/shared/mermaid-error-toast.tsx components/kanban/swimlane-kanban-content.tsx
```

## Risks

Do not place translated text into the module-level synthetic workflow-step constant.

## Results

PR shortcut and Mermaid owned fallbacks are localized, and the orphan step remains locale-neutral at
module scope with its title injected during render. Focused tests passed.
