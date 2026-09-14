---
id: "06-preview-state-and-renderer"
title: "Integrate the runtime with preview state and renderer"
status: cancelled
wave: 2
depends_on:
  - "05-script-capable-preview-runtime"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 06: Integrate the runtime with preview state and renderer

## Summary

Replace the static `srcDoc` execution path with the worker-backed runtime and a
scriptless snapshot renderer. Preserve the existing format-neutral file state,
legacy Markdown restoration, source buffer, and preview identity behavior.

## In scope

- Reuse or complete generic HTML preview eligibility and session-state
  migration.
- Replace source-controlled HTML sinks and iframe execution assumptions with
  the controlled snapshot renderer and event bridge.
- Apply static and dynamic resource, navigation, meta-refresh, form, and SVG
  link policies.
- Keep the current navigation-neutralization work as defense in depth.
- Add component and renderer tests for snapshots, events, failures, generation
  disposal, source restoration, and Markdown preservation.

## Out of scope

- Toolbar placement changes beyond the renderer contract.
- Public documentation.
- Desktop WebView smoke and broad browser suites.

## Acceptance

- HTML preview displays runtime snapshots and supports script-driven DOM
  updates without placing source scripts or inline event handlers in the native
  browser DOM.
- Static and dynamically-created URLs cannot cause a browser request,
  navigation, popup, download, form submission, or access to Kandev content.
- Runtime errors, budget failures, file identity changes, and source toggles
  dispose the generation safely and preserve the existing source/editor state.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/html-preview/html-preview-document.test.ts lib/html-preview/preview-surface.test.ts components/task/html-preview-content.test.tsx components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/html-preview/html-preview-document.ts`
- `apps/web/lib/html-preview/html-preview-document.test.ts`
- `apps/web/lib/html-preview/preview-surface.ts`
- `apps/web/lib/html-preview/preview-surface.test.ts`
- `apps/web/components/task/html-preview-content.tsx`
- `apps/web/components/task/html-preview-content.test.tsx`
- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- Existing file-state and storage files from the superseded prototype.

## Dependencies

Task 05 supplies the runtime message contract and capability policy.

## Risks

- A renderer that uses `innerHTML`, `dangerouslySetInnerHTML`, or native event
  attributes can turn a safe VM into an unsafe browser execution path.
- CSS URL, `srcset`, SVG, or mutation paths can bypass a policy that checks
  only initial HTML attributes.

## Parallelism

`sequential`

## Inputs

- All requirement criteria in the revised system design.
- Existing Markdown renderer, generic preview state, and mobile viewer patterns.
- The current local navigation-neutralization change.

## Results

Replaced the iframe `srcDoc` path with the worker-backed runtime client and a
scriptless Shadow DOM snapshot renderer. The component keeps the current
buffer, generation-safe disposal, source recovery, localized runtime failures,
and the existing generic file preview state.

Verification completed:

```text
pnpm exec vitest run lib/html-preview/html-preview-document.test.ts lib/html-preview/preview-surface.test.ts components/task/html-preview-content.test.tsx components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm exec tsc --noEmit --pretty false
pnpm run i18n:check
pnpm run i18n:ratchet
```
