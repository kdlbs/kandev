---
id: "05-prompt-sync-ui"
title: "Prompt Sync settings UI"
status: done
wave: 3
depends_on: ["02-prompt-ownership", "03-promptsync-backend"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.1
  - AC-INTEGRATIONS-PROMPT-SYNC-001.3
  - AC-INTEGRATIONS-PROMPT-SYNC-003.6
  - AC-INTEGRATIONS-PROMPT-SYNC-003.7
  - AC-INTEGRATIONS-PROMPT-SYNC-004.3
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 05: Prompt Sync settings UI

## Summary

On Settings > Prompts, add the Prompt Sync card and dialog, and render synced
prompts read-only with a synced badge. Desktop and phone have the same
capabilities.

## In scope

- Types and API client: `CustomPrompt.source` and `source_path`, plus
  `lib/types/prompt-sync.ts` and the four prompt-sync calls in
  `lib/api/domains/settings-api.ts`.
- Extract the shared repository fields (provider tabs, repository URL, branch
  and directory, polling) from `workflow-sync-dialog.tsx` into
  `repo-sync-fields.tsx`, with no change to Workflow Sync rendering or tests.
- `prompt-sync-section.tsx` (UI-01, UI-02, UI-04), `prompt-sync-dialog.tsx`
  (UI-03) with a workspace select listing the visible workspaces, a status
  line, and an expandable warning list.
- Prompt rows: a `PromptSyncedBadge` with a source-path tooltip, and edit and
  delete hidden when `source === "sync"`.
- Copy in the `settings` namespace for all seven locales, with no em dash.
  Run `pnpm run i18n:zh-hant` for the Traditional Chinese pair.

## Out of scope

Backend changes and E2E (task 06).

## ASCII UI preview

The full previews are in [plan.md](plan.md#ascii-ui-preview). Excerpts:

```text
UI-01 row (desktop):
| @principles  [Synced]                                            |
| Always prefer the simplest implementation that ...               |

UI-03 dialog field order:
Provider | Workspace | Repository | Branch | Directory | Auto-sync + Interval
[Remove sync]                                      [Cancel]  [Save]
```

On phones the card actions wrap below the status line, and the dialog uses
the existing Workflow Sync mobile presentation (UI-04).

## Acceptance

- Component tests: synced rows show the badge and no edit or delete buttons,
  while manual and built-in rows keep them. The card renders the unset, ok,
  warning, and error states. The dialog validates the interval and requires a
  workspace and repository. Save, Sync now, and Remove call the API and
  refresh the status.
- Workflow Sync dialog tests pass unchanged after the field extraction.
- `i18n:check` and `i18n:ratchet` pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/settings/prompt-sync components/settings/prompts-settings components/settings/workflow-sync
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
```

## Likely files

- `apps/web/components/settings/{prompt-sync-section.tsx,prompt-sync-dialog.tsx,prompt-synced-badge.tsx,repo-sync-fields.tsx}` (new) and tests
- `apps/web/components/settings/{prompts-settings.tsx,prompt-row-actions.tsx,workflow-sync-dialog.tsx}`
- `apps/web/lib/types/{http.ts,prompt-sync.ts}` and `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/src/locales/*/settings.json`

## Risks

- `prompts-settings.tsx` is already 617 lines. Keep the new section in its
  own file to stay within the lint limits.
- The workspace select must list only workspaces the user can see. Use the
  existing workspaces store slice.

## Results

- `pnpm exec vitest run components/settings/prompt-sync components/settings/prompts-settings components/settings/workflow-sync`: pass.
- `pnpm run typecheck`: pass (classic `tsc` needs
  `NODE_OPTIONS=--max-old-space-size=8192` on the development host).
- `pnpm run lint`: pass.
- `pnpm run i18n:check`: pass.
- `pnpm run i18n:ratchet`: pass.
