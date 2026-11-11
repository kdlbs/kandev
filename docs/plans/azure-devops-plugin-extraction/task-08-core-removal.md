---
id: "08-core-removal"
title: "Remove the built-in integration from Kandev"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001
  - REQ-PLUGINS-PLUGINS-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.1
  - AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.2
  - AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.3
  - AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.4
  - AC-PLUGINS-PLUGINS-001.16
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.13
system_design:
  - ../../specs/plugins/system-design/azure-devops-extraction.md
  - ../../specs/plugins/system-design/plugins-03.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 08: Remove the built-in integration from Kandev

## Summary

Delete the built-in Azure DevOps integration from backend and web per the extraction design inventory, release the reserved identities, drop its tables and credentials in a replay-safe migration after the boot backup, and update docs and spec lifecycle. The code lands on the local branch first (wave 1) so the plugin can be developed; the PR (K-A) opens after task 07.

## In scope

- Backend and web deletions and edits listed in the extraction design; `rg` completion check.
- Migration: `DROP TABLE IF EXISTS` for the eight tables, `DELETE FROM secrets WHERE name LIKE 'azure_devops:%'`, and the `https://dev.azure.com` provider host on built-in repositories with a clone URL on that origin; SQLite and env-gated PostgreSQL fresh and replay tests; `requiredstores`, `storeconformance`, upgrade fixture manifest.
- Reservation release in `registry.ts`, `registry-provider-ownership.ts`, `integration-settings-route.tsx`, `reserveBuiltinMentionIdentities`.
- Docs: `docs/public/integrations.md` pointer to the plugin and other public pages; `azure-devops-integration.md` deprecated and its designs superseded with banners; `internal/integrations/AGENTS.md`, root `AGENTS.md`/`CLAUDE.md` mentions; changelog via breaking-change PR title.
- Plugin bundle and UI asset responses send `Cache-Control: no-cache`, so a same-version reinstall (the plugin's development loop) is not hidden by browser heuristic caching.
- The provider repository that replaces a task-worktree repository keeps its clone URL (`safeRepositoryIDForTaskWorktree`), so plugin providers can verify Git credential requests.
- Rebase on `main` before opening the PR.

## Out of scope

- Registry entry (task 09).
- A notice page at the removed routes.

## Acceptance

- `rg -i 'azure.?devops|azuredevops|dev\.azure\.com' apps/backend apps/web` matches only the migration, plugin-neutral provider examples, lockfiles, and removal-pinning tests.
- Backend and web build, unit tests, i18n checks, and the Bitbucket contract E2E pass; a database with Azure tables and a PAT upgrades cleanly and a replay is a no-op.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd apps/backend && go build -trimpath ./... && go test -trimpath -race ./... -p=2)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1 && go run ./cmd/sqlguard ./internal)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=12288 pnpm run typecheck && pnpm exec vitest run && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/bitbucket-plugin-contract.spec.ts)
rg -il 'azure.?devops|azuredevops|dev\.azure\.com' apps/backend apps/web --glob '!**/*_test.go' --glob '!**/*.test.ts' --glob '!**/base_migrations*.go' --glob '!**/pnpm-lock.yaml'
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/azuredevops/`
- `apps/backend/internal/orchestrator/`
- `apps/backend/internal/backendapp/`
- `apps/backend/internal/task/`
- `apps/backend/internal/delivery/`
- `apps/backend/internal/agentctl/server/process/git_pr_providers.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/web/components/azure-devops/`
- `apps/web/lib/plugins/registry*.ts`
- `apps/web/src/locales/*/azuredevops.json`
- `docs/public/integrations.md`
- `docs/specs/integrations/requirements/azure-devops-integration.md`

## Dependencies

None

## Risks

- Large diff (about 17k lines plus tests); keep it mechanical, list the inventory in the PR body.
- `main` moves during plugin development; rebase before the PR and rerun verification.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/plugins/system-design/azure-devops-extraction.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-05: removal implemented locally on `feature/azure-devops-plugin-migration` (uncommitted). PR K-A opens after task 07, rebased on `main`.
- Verification:
  - `go build -trimpath ./...` and `go vet ./...` clean.
  - Full backend `go test -trimpath -p=2 ./...`: only `internal/notifications/service` fails (`TestRuntimeUpdateSummaryKeepsRecipientClaimsIsolated`, `TestRuntimeUpdateSummaryOmitsFailedMemberClaimAndRetriesItAlone`). Both fail identically on a clean checkout of `56af766`, so they are pre-existing and unrelated.
  - `go test -race ./internal/persistence/storeconformance -count=1` passes; `go run ./cmd/sqlguard ./internal` exits 0.
  - `bash scripts/lint-go-changed` on all 69 changed Go files: 0 issues.
  - New migration tests pass on SQLite; the env-gated PostgreSQL test was not run (no `KANDEV_TEST_POSTGRES_DSN`).
  - Web: typecheck, oxlint, `i18n:check`, `i18n:ratchet`, `e2e:sleep-ratchet`, and the guard allowlist pass. Full Vitest: one failure, the pre-existing `e2e/helpers/seed-repository-origin.test.ts`.
  - `pnpm e2e:run --project chromium e2e/tests/plugins/bitbucket-plugin-contract.spec.ts`: 1 passed.
  - `validate-public-docs`, `list-docs.py validate`, `lint-spec-files.py --all`, and `git diff --check` pass.
  - Completion `rg` (non-test files) leaves the retired-schema migration and its call site, the `AGENTS.md` pointers to the plugin, the zh-Hant brand glossary, and a test-only fixture that uses `azure_devops` as a plugin provider example.
- Known leftovers: other specs still mention Azure DevOps historically; `docs/screenshots/settings-integrations.png` still shows it; locale keys `common:presetIcon*` are orphaned.
- 2026-11-10: plugin bundle and UI asset responses send `Cache-Control: no-cache` (`serveInstalledFile`). Found while testing the plugin: a same-version reinstall kept the old bundle in the browser for hours. `TestInstalledUIFilesRevalidateBeforeReuse` failed before the change and passes after it; it also checks the `304` revalidation. `go test -trimpath -race ./internal/plugins -count=1` passes and `bash scripts/lint-go-changed` reports 0 issues.
- 2026-11-10: owner testing on `contoso` found that a task on a repository recorded by the built-in integration failed with "needs a provider host for managed credentials": the built-in integration never stored `provider_host`. The removal migration now records `https://dev.azure.com` on `azure_devops` rows whose clone URL is on that origin (`recordRetiredAzureDevOpsProviderHost`). `TestRetiredAzureDevOpsRepositoriesRecordTheirProviderHost` failed before the change and passes after it, including replay; the PostgreSQL twin is env-gated and was skipped (no `KANDEV_TEST_POSTGRES_DSN`). `TestResolveRepositoryIdentityForAzureReposCloneURL` pins that such a row resolves managed credentials and that an empty host still fails closed.
- 2026-11-10 (second owner test round): a task on that repository still failed to clone, now with "Azure DevOps Git credential unavailable". The dev instance lives under `~/.kandev/tasks`, so Kandev replaced the repository with a provider row (`safeRepositoryIDForTaskWorktree`), which copied the provider identity but not the clone URL; the plugin had nothing to verify the credential request against. The replacement now keeps `RemoteURL`. `TestService_CreateTask_TaskWorktreeReplacementKeepsTheRemoteURL` failed before the change and passes after it; `go test -trimpath -race ./internal/task/service -count=1` passes and `bash scripts/lint-go-changed` reports 0 issues. The plugin also verifies such rows by provider identity (task 02).
- 2026-11-11 (third owner test round): the clone still failed. The task reused a replacement row created before the previous fix, which had a provider host but no clone URL; Kandev then derived `https://dev.azure.com/Contoso/<repo>.git` from owner and name (no organization, no `_git`), and the plugin rightly refused it. Reusing a replacement now records the source's clone URL when the replacement has none and never replaces one it has (`backfillReplacementRemoteURL`, same rule as the find-or-create backfill). `TestService_CreateTask_TaskWorktreeReplacementReuseBackfillsTheRemoteURL` failed before the change and passes after it; `TestService_CreateTask_TaskWorktreeReplacementReuseKeepsItsOwnRemoteURL` pins the no-overwrite rule. `go test -trimpath -race ./internal/task/service -count=1` passes and `bash scripts/lint-go-changed` reports 0 issues.
- PR review (#5266):
  - The retired-schema cleanup ran its provider-host backfill before the migrations that add `repositories.remote_url` and `provider_host`, so a database older than those columns failed to start. The backfill now runs right after them. `TestRetiredAzureDevOpsCleanupRunsOnARepositoriesTableWithoutHostColumns` failed before the change (`no such column: provider_host`) and passes now.
  - Reusing a replacement repository saved the whole row read earlier, which could overwrite a clone URL or settings saved in between. `FillRepositoryRemoteURL` writes only `remote_url`, and only while it is still empty. `TestFillRepositoryRemoteURLOnlyFillsAnEmptyURL` covers SQLite, with an env-gated PostgreSQL twin.
  - Documentation coverage: the plan and this work order now declare `REQ-PLUGINS-PLUGINS-001`, `REQ-PLUGINS-REPOSITORY-TASK-CREATION-001`, and their system designs; the validator reports `covered` locally.
  - Verification: `go test -trimpath -race` for `internal/task/repository/sqlite`, `internal/task/service`, `internal/task/handlers`, and `internal/orchestrator/executor` passes; `bash scripts/lint-go-changed` reports 0 issues; `lint-spec-files.py --all` and `list-docs.py validate` pass.
