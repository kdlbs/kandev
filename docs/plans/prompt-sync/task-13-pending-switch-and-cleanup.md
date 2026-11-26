---
id: "13-pending-switch-and-cleanup"
title: "Override switch as a pending edit, and repository-only cleanup on removal"
status: done
wave: 11
depends_on: ["12-compact-review"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-007.6
  - AC-INTEGRATIONS-PROMPT-SYNC-009.2
  - AC-INTEGRATIONS-PROMPT-SYNC-009.5
  - AC-INTEGRATIONS-PROMPT-SYNC-009.6
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 13: Override switch as a pending edit, and repository-only cleanup on removal

## Summary

Test-phase feedback on Task 12:

1. The Local override switch was the only control on Settings > Prompts that
   applied at once. It becomes a pending edit that the shared save bar
   applies or resets, together with any edit to the local version.
2. After Prompt Sync is removed, prompts that only came from the repository
   cannot be told apart from the user's own, so the removal step offers to
   delete them.

## In scope

- Backend: one atomic version save (`SavePromptVersions` in the store and the
  prompt service, `promptsync.Service.SaveVersions`, `PUT
  /api/v1/prompt-sync/prompts/:id/versions`) replacing the override and
  revert routes; `alternate_content` on prompt DTOs; `RemovalReview` with
  `repository_only`; `ReleaseOptions.DeleteRepositoryOnly` in the release
  transaction; `DELETE /config` `delete_repository_only`.
- Frontend: controlled `PromptOverrideSwitch`; `PromptListItem` extracted to
  `prompt-list-item.tsx` with linked-prompt editing; `lib/settings/prompt-versions.ts`
  draft helpers; `savePromptVersions`; `useRemovalReview` and
  `PromptRemovalReview` with the checkbox; copy in seven locales.
- Specs, design, public guide, and E2E on desktop and phone.

## Out of scope

- A permanent "came from sync" marker on released prompts (the checkbox
  covers the clean-up at the only point where the origin is known).

## ASCII UI preview

```
| @post-pr-review [Synced]   (o) Local override  |
| +--------------------------------------------+ |
| | My reviewer text (editable)                | |
| +--------------------------------------------+ |
|                    Unsaved changes [Reset] [Save changes]

Remove step:
| [ ] Also delete 2 prompts that only came from the repository |
|     @principles, @release-notes                              |
|     Unchecked, they stay as normal prompts. ...              |
```

## Verification

- `go test ./internal/prompts/... ./internal/promptsync/...`
- `pnpm exec vitest run lib/settings/prompt-versions.test.ts lib/api/domains/prompt-sync-api.test.ts hooks/domains/settings/use-prompt-sync.removal.test.ts components/settings/prompts-settings-sync.test.tsx components/settings/prompt-sync-section.test.tsx`
- E2E: `e2e/tests/settings/prompt-sync.spec.ts`, `prompt-sync-conflicts.spec.ts`, `mobile-prompt-sync.spec.ts`
- `make fmt`, `make typecheck test lint`

## Results

- Implemented with TDD. A desktop E2E run also exposed that a sync which only
  refreshed an override's stored repository version published no prompt
  change, so open lists kept stale repository text; it now counts as a change
  (`TestApplyRecordsRepositoryVersionOnOverrides`).
- Backend: `go test ./internal/prompts/... ./internal/promptsync/...` pass;
  PostgreSQL store tests pass, including version saves and repository-only
  deletion; `sqlguard` clean.
- E2E: desktop Prompt Sync 4/4, phone 2/2.
- `make fmt`, `make typecheck`, `make lint` pass. `make test` fails only on the
  known host-environment tests (`TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc`,
  `TestCreateWorktree_RefreshLockPolicy`, `seed-repository-origin.test.ts`,
  and two scripts tests).
