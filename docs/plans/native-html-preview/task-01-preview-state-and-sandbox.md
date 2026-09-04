---
id: "01-preview-state-and-sandbox"
title: "Establish preview state and sandbox contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 01: Establish preview state and sandbox contract

## Summary

Create the format-neutral preview-kind and state contract used by every file
surface. Add the pure HTML preview-document builder and preserve existing
Markdown preview restoration through a one-way legacy read bridge.

## In scope

- Detect Markdown, HTML, and unsupported preview kinds case-insensitively.
- Exclude binary files before preview selection.
- Add and persist format-neutral rendered-preview state.
- Read legacy `markdownPreview` session records without dual-writing them.
- Build a complete HTML preview document with the required CSP.
- Add focused unit tests before implementation changes.

## Out of scope

- Rendering an iframe or adding toolbar controls.
- User-facing localized copy.
- Review-diff preview.
- Backend routes or workspace asset resolution.

## Acceptance

- Preview kind and eligibility have deterministic tests for `.md`, `.mdx`,
  `.html`, `.htm`, case variants, unsupported files, and binary exclusion.
- Preview state survives the two desktop restoration paths and accepts legacy
  Markdown records while new records use only the generic field.
- The document builder always places the restrictive CSP before workspace
  content and never grants network, form, frame, object, worker, base, or
  Kandev-origin capabilities.

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/utils/file-types.test.ts lib/html-preview/html-preview-document.test.ts lib/local-storage.test.ts hooks/use-file-editors.build-state.test.ts hooks/use-file-editors.open-action.test.tsx components/task/task-center-panel-file-tabs.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/utils/file-types.ts`
- `apps/web/lib/utils/file-types.test.ts`
- `apps/web/lib/html-preview/html-preview-document.ts`
- `apps/web/lib/html-preview/html-preview-document.test.ts`
- `apps/web/lib/types/workspace-files.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/lib/local-storage.test.ts`
- `apps/web/hooks/use-file-editors.ts`
- `apps/web/hooks/use-file-editors.build-state.test.ts`
- `apps/web/hooks/use-file-editors.open-action.test.tsx`
- `apps/web/components/task/task-center-panel-file-tabs.ts`
- `apps/web/components/task/task-center-panel-file-tabs.test.ts`
- `apps/web/components/task/task-center-panel-restoration.ts`

## Dependencies

None.

## Risks

- The two file-tab restoration paths can overwrite generic state if either
  whole-state write omits the normalized flag.
- CSP meta insertion order is security-sensitive and must not depend on source
  documents having a valid head element.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-NATIVE-HTML-PREVIEW-001`, especially acceptance criteria `.3` through
  `.6` and `.8`.
- The preview eligibility, state, HTML construction, security, and persistence
  sections of the system design.
- Existing Markdown preview persistence tests and file-type helpers.

## Results

Implemented generic preview-kind detection, rendered-preview state, legacy
`markdownPreview` restoration, and the sandbox document builder. Added focused
coverage for file eligibility, storage migration, center-panel restoration, and
the restrictive CSP. Verification passed:

```text
pnpm exec vitest run lib/utils/file-types.test.ts lib/html-preview/html-preview-document.test.ts lib/local-storage.test.ts hooks/use-file-editors.build-state.test.ts hooks/use-file-editors.open-action.test.tsx components/task/task-center-panel-file-tabs.test.ts
pnpm run typecheck
```
