---
id: "02-upgrade-stored-prompts"
title: "Upgrade stored Improve prompts"
status: done
wave: 2
depends_on: ["01-lifecycle-evidence-guidance"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-VALIDATION-002
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.1
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.2
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.3
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.4
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.5
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.6
  - AC-WORKSPACES-IMPROVE-VALIDATION-002.7
system_design:
  - ../../specs/workspaces/system-design/improve-kandev-validation.md
---

# Task 02: Upgrade stored Improve prompts

## Summary

Advance known stored template and step prompts before subsequent Improve admission. Preserve custom content and identities.

## In scope

- Keep the preceding shipped prompt text in a small fixture and add a narrow initialization update.
- Cover stored system template JSON and materialized steps, including fallback workspaces and instances without is_system.
- Use exact-value conditional writes. Preserve unrelated fields and concurrent edits without a retry loop or hash registry.
- Add production bootstrap/reuse, SQLite reopen, and environment-gated PostgreSQL behavior tests.

## Out of scope

Unrelated behavior changes, generic verification passes, and automatic publication.

## Acceptance

1. TestImprovePromptUpgrade covers old templates, old instances, new instances from old templates, repeated reopen, custom content, and identity/property preservation.
2. TestImprovePromptUpgradeConcurrentEdit preserves the winning edit. TestImprovePromptUpgradeFailure propagates a real write failure and prevents admission.
3. TestImproveBootstrapUsesUpgradedInstructions exercises actual workflow reuse. Transcripts and accepted turns remain unchanged.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/workflow/repository ./internal/improvekandev -run 'TestImprovePrompt|TestImproveBootstrap' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/workflow/repository -run 'TestImprovePromptPostgres' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/workflow/repository/sqlite.go apps/backend/internal/workflow/repository/improve_prompts.go apps/backend/internal/workflow/repository/improve_prompts_test.go apps/backend/internal/workflow/repository/improve_prompts_postgres_test.go apps/backend/internal/improvekandev/prompt_upgrade_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/repository/improve_prompts.go (new)`
- `apps/backend/internal/workflow/repository/improve_prompts_test.go (new)`
- `apps/backend/internal/workflow/repository/improve_prompts_postgres_test.go (new)`
- `apps/backend/internal/workflow/repository/testdata/improve-prompts-* (new)`
- `apps/backend/internal/improvekandev/prompt_upgrade_test.go (new)`

## Dependencies

Complete `01-lifecycle-evidence-guidance` first.

## Risks

Prompt matching must not destroy unknown JSON fields. Unknown historical content remains preserved. A skipped PostgreSQL test is not executed PostgreSQL evidence.

## Parallelism

`sequential`

## Inputs

Read Stored instruction upgrades and Failure and compatibility in the validation design. Read startup, GetTemplate, CreateStepsFromTemplate, bootstrap reuse, and prompt-seeding precedent.

## Execution notes

Write failing upgrade tests before production edits. Use ordinary constructors and real writes.
Set KANDEV_TEST_POSTGRES_DSN to an owned test database for PostgreSQL and store-conformance checks.
Record the database identity without credentials. Do not mark PostgreSQL verification complete from a skip.

## Results

Completed. Startup now upgrades the known preceding Improve prompt revision in the stored system template and in workflows bound to that template. It compares SHA-256 hashes after the embedded loader's trim normalization, requires matching creation and update timestamps, and conditions writes on the original row values. Unknown or edited prompts remain unchanged. A template compare conflict rereads once and preserves unrelated JSON fields; a second conflict fails repository construction. Fresh instances read the upgraded template, and reused workflows retain their IDs and steps.

Review follow-up drains the first workflow service before closing the original
database. It retains the reopened workflow service and event bus, registers
cleanup immediately, and closes both before the reopened database cleanup.
`TestImproveBootstrapUsesUpgradedInstructions` passes under `-race`.
The PostgreSQL integration fixture rebinds both `?` lookups for the active
database dialect. It remains environment-gated and was skipped because
`KANDEV_TEST_POSTGRES_DSN` was unset.

Verification passed:

- `(cd apps/backend && go test -trimpath -race ./internal/workflow/repository ./internal/improvekandev -run 'TestImprovePrompt|TestImproveBootstrap' -count=1)`
- `(cd apps/backend && go test -trimpath -race ./internal/workflow/repository -run 'TestImprovePromptPostgres' -count=1)` compiled and passed with the PostgreSQL test skipped because `KANDEV_TEST_POSTGRES_DSN` was unset.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)`
- `(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)`
- `bash scripts/lint-go-changed apps/backend/config/workflows/loader.go apps/backend/internal/workflow/repository/sqlite.go apps/backend/internal/workflow/repository/improve_prompts.go apps/backend/internal/workflow/repository/improve_prompts_test.go apps/backend/internal/workflow/repository/improve_prompts_postgres_test.go apps/backend/internal/improvekandev/prompt_upgrade_test.go`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

PR fixup build passed:

- `make -C apps/backend build`

PR fixup CI remediation: the PostgreSQL package run exposed un-rebound `?`
placeholders in the integration test; both reads now use `database.Rebind`.
The backend shard exposed migration-lite workflow tables without
`workflow_template_id`; startup now skips materialized-step inspection when
that binding column is absent. A focused regression covers repository startup
with that schema.

Verification passed:

- `(cd apps/backend && go test -trimpath -race ./internal/workflow/repository ./internal/improvekandev -run 'TestImprovePrompt|TestImproveBootstrap|TestImprovePromptUpgradeAllowsWorkflowsWithoutTemplateBindingColumn' -count=1)`
- `(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^TestHandleReorderWorkflowStepsOrderValidation$' -count=1)`
- `make -C apps/backend build`
- `bash scripts/lint-go-changed apps/backend/internal/workflow/repository/improve_prompts.go apps/backend/internal/workflow/repository/improve_prompts_test.go`
- `git diff --check`
