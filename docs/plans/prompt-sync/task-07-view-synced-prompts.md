---
id: "07-view-synced-prompts"
title: "View synced prompt content"
status: done
wave: 5
depends_on: ["05-prompt-sync-ui", "06-e2e-and-docs"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-004.3
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 07: View synced prompt content

## Summary

Test-phase feedback: synced prompts showed only a one-line preview, and with
edit hidden there was no way to read the full text. Add a read-only View
toggle to synced rows on desktop and phone.

## In scope

- `SyncedPromptListItem` in `components/settings/prompt-synced-view.tsx`:
  name, synced badge, View toggle, and read-only `SettingsPromptEditor`
  content with a source-file note.
- `PromptListContent` renders it for `source === "sync"`; the editable row is
  unchanged for manual and built-in prompts.
- Copy in seven locales.

## Out of scope

Editing synced prompts and backend changes.

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview), UI-01:

```text
| @principles  [Synced]                                    [view]  |
| <full content, read-only editor>                                 |
| Read-only. Synced from .kandev/prompts/principles.md; edit ...   |
```

## Acceptance

- Synced rows offer View and no edit or delete; View toggles the full content
  read-only with the source note. Manual rows keep edit and delete.
- The View control is at least 44px tall on phones.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/settings/prompts-settings
pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts
```

## Results

- RED: the component test could not find `prompt-view-button`.
- `pnpm exec vitest run components/settings/prompts-settings`: 26 passed.
- `pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts`: 1 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts tests/settings/mobile-prompts-settings.spec.ts`: 3 passed.
