---
id: "02-repair-override-metadata"
title: "Repair detached override metadata"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.7
  - AC-INTEGRATIONS-PROMPT-SYNC-009.1
  - AC-INTEGRATIONS-PROMPT-SYNC-009.3
  - AC-INTEGRATIONS-PROMPT-SYNC-010.2
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 02: Repair detached override metadata

## Summary

When its file leaves the source, an override keeps its row and local
content. Clear its repository path alongside its repository version. If a
valid matching file returns, restore its current path and text together.

## In scope

- Clear `source_path` in the existing `DetachOverride` update that clears
  the repository version and conflict.
- Replace `SetOverrideRepositoryContent` with a guarded update for file
  path and repository version. Change the row when either value differs.
  Use the existing prompt-change publication through `result.wrote`.
- Project an empty `source_path` on HTTP, boot, and MCP reads of an
  override with no repository version. Reuse a single projection in
  `promptdto.FromPrompt` and the MCP read. This also covers older detached
  rows. Make the MCP guidance reflect the detached state.
- Use a localized badge tooltip for a detached override without a path
  placeholder. Keep the visible **Removed from repository** state and
  editable local text on desktop and phone.
- In the [Prompt Sync guide](../../public/prompt-sync.md), describe the
  detached override and its recovery when a matching file returns.

Automatic release to manual ownership, rename detection, and per-file age
display need distinct decisions recorded in the [plan](plan.md#other-issue-findings).

## Acceptance

- After removal, the same prompt ID and local text remain. The repository
  version, path, and conflict are cleared. Repository Save still returns
  HTTP 409 while the prompt has only a local version.
- An older detached row returns an empty path and accurate guidance
  before its next sync. When a valid same-name file returns or its path
  changes, both reads and switch behavior use the current file.
- Settings shows **Removed from repository** and an editable local version
  on desktop and phone. A restored file brings back the version switch.

## ASCII UI preview

UI-02 from the [plan](plan.md#ui-02-override-after-its-file-leaves-the-source)
applies to AC-INTEGRATIONS-PROMPT-SYNC-007.2, 008.7, and 009.3.

```text
Desktop
@hook [Overridden]                Removed from repository  [Edit] [Delete]
Badge tooltip: Your local version is still editable.

Phone
@hook [Overridden]
Removed from repository
[Edit] [Delete]
```

The phone uses the inline message and existing Edit action. A tooltip
adds explanatory copy for keyboard and pointer users.

## Verification

Add `TestDetachedOverrideClearsAndRestoresRepositoryPath` in
`sync_apply_versions_test.go` before changing writes. The first run fails
because the detached row still contains the repository path. Test a
returning file at a different path, the same repository text at a new
path, and repeat reconciliation with identical input.

Check the guarded update with the existing PostgreSQL ownership test.
The PostgreSQL harness uses `KANDEV_TEST_POSTGRES_DSN`.

Add `TestSharedPromptReadOmitsStaleRepositoryPathForDetachedOverride`.
Exercise the DTO and MCP read with a stale path on an older detached row
and a valid path on a linked override.
Check the existing stored-version test for item 5's spec correction.
Use the browser test title phrase `restores an override file after removal`
in both projects. Exercise the real fixture repository and sync API.
Reload after detachment and again after repository recovery. Verify the cleared
or restored path, Local selection, detached state, and local content after each
reload before continuing the version-switch assertions.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/prompts/... ./internal/promptsync ./internal/mcp/handlers)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^TestPostgresRepository_OwnershipTransitionsKeepAlternateContent$' -v ./internal/prompts/store)
(cd apps/web && pnpm exec vitest run components/settings/prompts-settings-versions.test.tsx lib/settings/prompt-versions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/prompt-sync.spec.ts -- --grep 'restores an override file after removal' --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-prompt-sync.spec.ts -- --grep 'restores an override file after removal' --retries=0)
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/prompts/store/interface.go`
- `apps/backend/internal/prompts/store/ownership_writes.go`
- `apps/backend/internal/prompts/store/alternate_writes_test.go`
- `apps/backend/internal/prompts/store/postgres_conflict_test.go`
- `apps/backend/internal/prompts/service/sync_apply.go`
- `apps/backend/internal/prompts/service/sync_apply_versions_test.go`
- `apps/backend/internal/prompts/dto/dto.go`
- `apps/backend/internal/prompts/dto/dto_test.go`
- `apps/backend/internal/mcp/handlers/config_prompt_handlers.go`
- `apps/backend/internal/mcp/handlers/config_prompt_synced_test.go`
- `apps/web/components/settings/prompt-override-controls.tsx`
- `apps/web/components/settings/prompts-settings-versions.test.tsx`
- `apps/web/src/locales/en/settings.json` and the six other shipped catalogs
- `apps/web/e2e/tests/settings/prompt-sync.spec.ts`
- `apps/web/e2e/tests/settings/mobile-prompt-sync.spec.ts`
- `docs/public/prompt-sync.md`

## Dependencies

None. Run after work order 01 to keep shared E2E files and doc edits
sequential.

## Risks

- A removed file and a frozen invalid file have different outcomes. Cover
  both so invalid content does not detach an override.
- A valid file can return with the same text at a different path. Guard
  the update on both fields and publish the path change.

## Parallelism

Sequential.

## Inputs

- [Local version requirements](../../specs/integrations/requirements/prompt-sync-local-versions.md)
- [Lossless switching design](../../specs/integrations/system-design/prompt-sync.md#lossless-switching)
- `TestRenamedOverrideFileKeepsOldOverrideAndCreatesSyncedPrompt`
- `TestSaveVersionsUsesTheStoredRepositoryVersionUntilTheNextSync`
- `TestSaveVersionsRouteRejectsRepositoryWhenTheFileIsGone`

## Results

Completed.

- Prompt store, service, DTO, Prompt Sync, and MCP tests passed. SQLite coverage
  verifies detach clears the path, returning files restore repository text and
  path together, path-only changes publish, and repeated unchanged syncs do not
  write.
- A legacy detached-row regression verifies that a path-only stale field does
  not change the saved version timestamp or reject a pending Local draft.
- DTO and MCP read tests cover a legacy detached row with a stale path and a
  linked override with its current path. MCP guidance describes the remaining
  local version.
- Settings tests passed for the pathless tooltip and editable local version.
  Desktop Chromium and mobile Chrome tests passed for removal and restoration
  at a changed configured path. Both browser tests reload after detachment and
  after recovery, then recheck the path and Local content/selection.
- Web typecheck, i18n validation, public documentation validation, changed Go
  lint, and whitespace checks passed. The PostgreSQL ownership test was skipped
  because `KANDEV_TEST_POSTGRES_DSN` was not configured.
- The stored-version Save behavior remains unchanged and matches the corrected
  requirement: it uses the stored repository version until the next sync.
