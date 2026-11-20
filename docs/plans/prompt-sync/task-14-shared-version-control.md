---
id: "14-shared-version-control"
title: "Shared Local | Repository control, several pending switches, and read-only styling"
status: done
wave: 12
depends_on: ["13-pending-switch-and-cleanup"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-004.3
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.6
  - AC-INTEGRATIONS-PROMPT-SYNC-009.2
  - AC-INTEGRATIONS-PROMPT-SYNC-010.3
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 14: Shared Local | Repository control, several pending switches, and read-only styling

## Summary

Test-phase feedback on Task 13:

1. The Prompt Sync section drew an empty bordered box while unconfigured.
2. The review step's control was `Local | switch | Repository` (on is the
   repository) while rows used a "Local override" switch (on is local).
   Rows now use the same control, direction, and words.
3. Only one switch could be pending at a time; several rows can now be
   switched and saved with one Save.
4. Read-only editors now sit on a muted background.

A page stall reported once after saving with the switch off was not
reproduced; the E2E run covers that flow.

## In scope

- `VersionSwitch` exported and reused by `PromptVersionControl`;
  `PendingVersions`, `choiceFor`, `withChoice`, and `planPromptSaves` in
  `lib/settings/prompt-versions.ts`; `PromptVersionPreview`; Edit on a synced
  row switched to Local; one save request running the plan in order.
- `data-read-only` on `SettingsPromptEditor` with a muted Monaco background.
- `frame="none"` for the unconfigured Prompt Sync section.
- Agent error, MCP description, and edit guidance reworded to "switch it to
  Local in Settings > Prompts and save"; locale copy in seven languages.
- Specs, design, public guide, unit tests, and E2E on desktop and phone.

## Out of scope

- Several prompts open for text editing at once (one draft stays the rule).

## ASCII UI preview

```
| @post-pr-review [Synced]  Local (o--) Repository  [edit] |
| +------------------------------------------------------+ |
| | My reviewer text            (muted, read-only)       | |
| +------------------------------------------------------+ |
| Your version is used after you save. Select Edit ...     |
| @principles [Overridden]  Local (--o) Repository  [edit][del] |
| | Shared principles           (muted, read-only)       | |
|                     Unsaved changes [Reset] [Save changes]
```

## Verification

- `pnpm exec vitest run lib/settings/prompt-versions.test.ts components/settings/prompts-settings-versions.test.tsx components/settings/prompts-settings-sync.test.tsx components/settings/prompt-sync-section.test.tsx components/settings/settings-prompt-editor.test.tsx`
- `go test ./internal/mcp/handlers/ ./internal/mcp/server/ ./internal/prompts/... ./internal/promptsync/`
- E2E: `e2e/tests/settings/prompt-sync.spec.ts`, `prompt-sync-conflicts.spec.ts`, `mobile-prompt-sync.spec.ts`
- `make fmt`, `make typecheck test lint`

## Results

- Implemented with TDD (helpers, page, section, editor, MCP wording tests).
- E2E: desktop Prompt Sync 4/4 (including several switches saved together
  and the muted read-only background), phone 2/2.
- `make fmt`, `make typecheck`, `make lint` pass. `make test` fails only on the
  known host-environment tests (`TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc`,
  `TestCreateWorktree_RefreshLockPolicy`, `seed-repository-origin.test.ts`,
  and two scripts tests).
- The one-time page stall was not reproduced.
