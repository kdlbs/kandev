---
id: "03-hybrid-recovery"
title: "Recover failed and unsupported hybrid editors"
status: completed
wave: 3
depends_on: ["02-comment-identity"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.4
  - AC-UI-MARKDOWN-FILE-EDITING-001.10
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-002.15
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 03: Recover failed and unsupported hybrid editors

## Summary

Keep Source usable after missing capability or engine failure. Dispose a failed instance once and remount only after an explicit retry.

## In scope

- Detect required capability at the adapter boundary before allocating the engine.
- Separate stored mode preference from the effective fallback mode.
- Reset desktop and phone retention flags on failure. Keep healthy retained instances alive.
- Add localized recovery status and content-free diagnostics. Update all locale catalogs required by the repository.

## Out of scope

- An EditContext polyfill, an engine upgrade, or native Safari hybrid support.
- Changing source-position transfer or table behavior.

## Acceptance

- Initialization and update failure tests first reproduce the blank retry. Explicit retry then creates a fresh working instance or safely remains in Source.
- Missing capability leaves Preview and Source usable, explains unavailable Edit, and preserves the stored preference and exact draft.
- Cleanup disposes each resource once. A failed instance cannot emit late edits, and recovery does not save the file.

## UI preview

UI-02: Source retains the draft and displays localized recovery status. Edit retries explicitly after failure. Without required capability, Edit explains its unavailable state. Desktop uses inline controls. Phone uses its current-mode menu with visible Save. See [combined preview](plan.md#ui-preview).

## Verification

Add the named regression scenarios before the correction. Run the focused test to establish the expected failure, then run this complete block after the fix. Commands start at the repository root.

```bash
(cd apps/web && pnpm exec vitest run components/editors/markdown/hybrid-markdown-editor.test.tsx components/task/markdown-file-mode.test.ts components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project=chromium tests/task/markdown-file-editing.spec.ts --grep "review regression: hybrid recovery")
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts --grep "review regression: hybrid recovery")
```

## Files likely touched

- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/components/task/markdown-file-mode.ts`
- `apps/web/components/task/markdown-file-mode.test.ts`
- `apps/web/components/task/markdown-file-editor.tsx`
- `apps/web/components/task/markdown-file-editor.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/e2e/tests/task/markdown-file-editing.spec.ts`
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 02-comment-identity. Coordinators and browser test files overlap, so execution is sequential.

## Risks

Capability absence is not runtime failure. A supported constructor can still fail. Do not loop automatic retries or overwrite preferences.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [System design](../../specs/ui/system-design/markdown-file-editing.md).
- [Review evidence and UI composition](plan.md).

## Results

Implemented capability detection, preserved-mode Source fallback, explicit
retry with a fresh instance, and single-disposal cleanup. Failed and inactive
instances cannot publish late edits. The localized recovery status explains
how to retry without discarding source.

Adapter and coordinator regressions pass in the 255-test targeted aggregate.
Desktop and phone browser tests passed for failed-instance retry with an
unsaved draft and for Source editing without EditContext.
Native Firefox and Safari are unavailable in this environment. Capability
simulation does not establish native browser coverage.
