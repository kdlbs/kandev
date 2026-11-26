---
id: "15-independent-rows-and-drafts"
title: "Rows open independently of the switch, several drafts, single outline, and read-only contrast"
status: done
wave: 13
depends_on: ["14-shared-version-control"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-004.3
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 15: Rows open independently of the switch, several drafts, single outline, and read-only contrast

## Summary

Test-phase feedback on Task 14:

1. Read-only text had too little contrast: the prompt editor uses the dark
   Monaco theme in every app theme, so the light muted surface sat behind
   light text.
2. Switching between Local and Repository opened or closed rows. Rows now
   open and close only from their own control.
3. The configured Prompt Sync section drew two outlines (group card and
   status card).
4. Switching an open row to Local required a save, then Edit, then another
   save. Local rows are now editable at once, any number of prompts can be
   edited before one Save, and each unsaved prompt is marked.
5. A synced row had a read-only view of its local version. Now the eye
   shows and hides the read-only repository version, and **Edit** shows and
   hides the editable local version, matching manual prompts.
6. Follow-up test round: the Custom prompts group drew a card outline
   around the search bar and rows, which carry their own outlines. It now
   uses `frame="none"`.

## In scope

- `PromptDrafts`, `draftForm`, `withDraftEdit`, `showsEditor`,
  `isDraftValid`, and a drafts-based `planPromptSaves` in
  `lib/settings/prompt-versions.ts`.
- `use-prompts-page.ts`: open rows, drafts, and pending switches keyed by
  prompt ID; one `PromptListItem` for every row; `PromptRowActions` with an
  Edit or eye toggle; `PromptRepositoryView`; an "Unsaved" row badge.
  `SyncedPromptListItem`, `PromptVersionPreview`, and
  `use-prompt-editor-effects.ts` removed.
- Read-only editor: lighter dark surface, hidden cursor, dashed frame.
- `PromptSyncSection` and the Custom prompts group always `frame="none"`.
- Locale copy in seven languages; specs, design, public guide; unit and E2E
  tests on desktop and phone.

## Out of scope

- Backend changes; the versions and update APIs are unchanged.

## ASCII UI preview

```
| @post-pr-review [Synced] [Unsaved]  Local (o--) Repository  [edit] |
| +----------------------------------------------------------------+ |
| | My reviewer text                       (editable)              | |
| +----------------------------------------------------------------+ |
| @principles [Overridden] [Unsaved]  Local (--o) Repository  [eye][del] |
| Shared principles (collapsed, first line)                           |
|                              Unsaved changes [Reset] [Save changes]
```

## Verification

- `pnpm exec vitest run lib/settings/prompt-versions.test.ts components/settings/prompts-settings components/settings/prompt-sync-section.test.tsx components/settings/settings-prompt-editor.test.tsx`
- E2E: `prompt-sync.spec.ts`, `prompt-sync-conflicts.spec.ts`,
  `prompts-settings.spec.ts`, `saved-prompt-search.spec.ts`,
  `shared-prompt-mcp-writes.spec.ts` (chromium); `mobile-prompt-sync.spec.ts`,
  `mobile-prompts-settings.spec.ts`, `mobile-saved-prompt-search.spec.ts`
  (mobile-chrome)
- `make fmt`, `make typecheck test lint`

## Results

- Implemented with TDD (draft helpers, page rows, section outline, read-only
  editor frame).
- E2E: desktop 14/14 (Prompt Sync, conflicts, prompts settings, search, MCP
  writes), including switching and editing several prompts before one Save
  and a read-only text contrast above 4.5:1; phone 5/5.
- `make fmt`, `make typecheck` (8 GiB Node heap), `make lint`, and spec lint
  pass. `make test` fails only on the known host-environment tests
  (`TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc`,
  `TestCreateWorktree_RefreshLockPolicy`, `seed-repository-origin.test.ts`,
  and two scripts tests).
- Follow-up: prompts settings and Prompt Sync E2E 9/9 desktop, phone prompts
  settings 2/2, with the same verification results.
