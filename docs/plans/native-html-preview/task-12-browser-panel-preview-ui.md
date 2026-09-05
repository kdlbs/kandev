---
id: "12-browser-panel-preview-ui"
title: "Replace the virtual runtime with Browser-panel UI"
status: done
wave: 3
depends_on:
  - "11-session-preview-publish-contract"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 12: Replace the virtual runtime with Browser-panel UI

## Summary

Wire `Preview HTML` to publish the current buffer. Desktop opens the resulting
URL in the existing Browser panel. Mobile opens it in the focused iframe. Remove
the QuickJS and virtual-DOM path after the server flow works.

## In scope

- Desktop toolbar activation, progress/error state, trusted-code affordance,
  Browser-panel focus/reuse, cache-busted refresh, and dirty-buffer preservation.
- Mobile publication, full-height iframe, 44-pixel target, `Show code`, retry,
  and repository-plus-path identity reset.
- Shared localized copy in English, Portuguese, Simplified Chinese, and
  Traditional Chinese catalogs.
- Unit/component tests across Monaco, CodeMirror, Dockview, center-panel, and
  mobile paths as applicable.
- Removal of the QuickJS worker, virtual DOM, renderer, navigation normalizer,
  obsolete state, preview-only packages, and obsolete tests.
- Regeneration of the lock and license artifacts.
- No behavior changes to Markdown or explicit development-server controls.

## Out of scope

- Public documentation and browser E2E.
- New Browser-panel inspector or console features.

## Acceptance

- Desktop publishes the unsaved buffer, keeps source open and dirty, and reuses
  the existing Browser panel on repeated activation.
- Mobile renders the same proxy URL in a contained focused viewer and reliably
  returns to unchanged source.
- Trust and recovery copy is localized and available outside preview content.
- QuickJS and the preview-only direct parse5 dependency no longer ship. The
  Markdown renderer may retain parse5 as a transitive license entry.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/dev-server-preview-button.test.tsx components/task/html-preview-content.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
cd ..
pnpm --filter @kandev/web licenses:gen
```

## Files likely touched

- `apps/web/components/task/html-preview-content.tsx`
- HTML preview entry points in Monaco, CodeMirror, Dockview, and mobile files.
- `apps/web/lib/state/dockview-panel-actions.ts`
- `apps/web/src/locales/**`
- `apps/web/lib/html-preview/**` (removed)
- `apps/web/package.json`
- `apps/pnpm-lock.yaml`
- Generated dependency/license artifacts.

## Dependencies

Task 11 supplies the publish API and proxy URL.

## Risks

- Generic Browser-panel reuse can replace unrelated content.
- Async publish completion can target a file that changed identity while the
  request was in flight.
- Removing generic preview state can regress Markdown restoration.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria `.1` through `.3` and `.7` through `.10`.
- Desktop/mobile control flow and responsive contract in the system design.
- Existing development-server button and mobile Markdown preview as exemplars.

## Results

Replaced the preview-specific QuickJS and virtual-DOM path with the native
Browser-panel and mobile focused-viewer flows.

- Desktop publishes the current buffer, preserves the source tab, and reuses
  the shared Browser panel with versioned refresh URLs.
- Mobile publishes the current buffer into a full-height iframe with a
  touch-sized control, trust warning, Show code, retry, and identity reset.
- Desktop task-center and file-editor publication now invalidate requests on
  session replacement and unmount, so stale success, error, and cleanup
  callbacks cannot affect the replacement session.
- Markdown preview and explicit development-server behavior remain unchanged.
- Removed the preview-specific runtime, worker, direct QuickJS dependency, and
  direct parse5 dependency. Markdown keeps its transitive parse5 dependency.
- Added localized trust, loading, retry, and unavailable copy in all catalogs.

Verification:

- Focused Vitest suite passed: 13 files and 69 tests, including deferred
  stale-session success/error coverage for task-center and file-editor hooks.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- `pnpm --filter @kandev/web licenses:gen` completed successfully.
