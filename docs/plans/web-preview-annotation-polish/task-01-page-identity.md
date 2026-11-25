---
id: "01-page-identity"
title: "Page identity without Kandev parameters"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 01: Page identity without Kandev parameters

## Summary

Stop the HTML preview's republish version from changing page routes, so
reopening a preview restores its markers, outlines, and numbering.

## In scope

- `sanitizePreviewPageRoute` and the inspector's `sanitizedSearch` drop query
  parameters whose names start with `kandev_`.
- `buildHtmlPreviewProxyUrl` sends the version as `kandev_preview_version`
  instead of `v`.

## Out of scope

- Migrating pending items already saved with a `?v=` route.
- Any other change to page identity or the sensitive-parameter rule.

## Acceptance conditions

1. Both sanitizers return the same route for two URLs that differ only in a
   `kandev_` parameter, and keep other non-sensitive parameters.
2. The URL builder emits `kandev_preview_version`; the publisher hook still
   reloads the iframe when the version changes.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/preview-feedback-source.test.ts lib/api/domains/process-api.test.ts hooks/use-html-preview-publisher.test.ts lib/preview-inspector-script.test.ts
```

## Likely files

- `apps/web/lib/preview-feedback-source.ts`, `.test.ts`
- `apps/web/lib/api/domains/process-api.ts`, `.test.ts`
- `apps/web/hooks/use-html-preview-publisher.test.ts`
- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
- `apps/web/lib/preview-inspector-script.test.ts`

## Risks

- The two sanitizers must stay identical, or the inspector filters out
  markers the parent sends. Test both with the same cases.
