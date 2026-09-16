---
id: "02-explicit-profile-policy"
title: "Persist and apply explicit profile strictness"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.8
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.9
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---
# Task 02: Persist and apply explicit profile strictness

## Summary

Deliver an independently testable backend/API policy from persisted profile to prompt.

## In scope

Deliver an independently testable backend/API policy from persisted profile to prompt.

Add the additive false-default migration and profile field across storage,
DTO/controller/handler mappings, discovery schema, copy/import paths, resolvers,
internal projections, and session snapshots. Keep omitted partial updates intact.
Validate strict+empty/dynamic/passthrough without invalidating legacy profiles.
Regenerate discovery snapshots with the existing generator.

Extend the shared policy with explicit strictness and restore compatible
selection/error behavior from the oracle. Update reset/rebind/replacement waits
and workflow drift eligibility. Preserve fresh-client-only evidence, cancellation,
validated reuse candidates, prompt ordering, and warning persistence.

Add a real legacy-store-to-agentctl/lifecycle launch regression. Exercise normal
launch and all replacement boundaries. Do not only test synthetic policy structs.
Parameterize the real store migration/round-trip evidence for SQLite and Postgres.
Add projection tests proving true and false reach the runtime and snapshot.

## Out of scope

Global policy, provider authentication, Office post-start routing redesign,
unrelated cleanup, and publication.

## Acceptance

- A migrated old profile launches through the real lifecycle/transport boundary
  under the same missing-model conditions as before. Explicit strictness stops
  that launch before its prompt. Saved fallback fields remain identical.
- The complete policy matrix, partial-update/copy paths, and generated contract
  pass. Strictness survives every projection, including dynamic candidate resolution.
- Recovery and workflow tests preserve existing fixes while applying the new
  flag, with deterministic delayed events, cancellation, and empty reuse lookup.


## Verification

Run from the repository root. Use TDD for changed logic; first record the
behavioral failure, then run the listed checks after implementation.

```bash
(cd apps/backend && go run ./cmd/settings-catalog)
(cd apps/backend && go test ./internal/agent/settings/... ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator/... ./internal/backendapp)
(cd apps/backend && go run ./cmd/settings-catalog -check)
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated Postgres test DSN before this check}"
(cd apps/backend && go test -count=1 ./internal/agent/settings/store -run 'TestPostgres.*(RequireExactModel|Schema)')
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/settings/{models,dto,store,controller,handlers}/`
- `apps/backend/internal/agent/settings/store/sqlite_require_exact_model_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/{types,profile_resolver,manager_profile,start_model}.go`
- `apps/backend/internal/agent/runtime/lifecycle/{session,manager_interaction,manager_workspace_rebind,manager_kubernetes_refresh}.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_model_upgrade_test.go` (new)
- Existing lifecycle model, reset, rebind, and remote transport tests.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` and profile session policy tests.
- `apps/backend/internal/orchestrator/executor/{executor,executor_execute}.go` and tests.
- `apps/backend/internal/backendapp/adapters.go` and projection tests.
- `apps/web/lib/settings-discovery/{contract,profile-contract}.generated.json` (generator output).

## Dependencies

None; preserve Task 01 transport and candidate-lookup fixes.

## Risks

Do not convert old auto=false values into strict intent. Do not swallow
initialization/transport errors as missing catalogs. Postgres tests must actually
run with an isolated DSN. A stale cached profile must not lose true strictness.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/no-silent-model-fallback.md)
- [Policy design](../../specs/agents/system-design/no-silent-model-fallback-01.md)
- [Recovery/UI design](../../specs/agents/system-design/no-silent-model-fallback-02.md)
- [Plan and regression matrix](plan.md)
- Scoped AGENTS.md, existing tests beside the affected code, and compatibility
  behavior at `ba960f973205854733e0dcd9afa355bdda3dddf6`.

## Results

Implemented on 2026-09-15. The profile field now has an additive SQLite
migration, full DTO/controller/handler/store mappings, generated discovery
contracts, runtime lifecycle and workflow propagation, strict and compatible
selection behavior, and validation for unsupported profile kinds. Existing
fallback values remain dormant while strictness is enabled and survive an
explicit disable.

Focused settings, lifecycle, executor, backend adapter, and orchestrator tests
passed. The broader combined backend command reached unrelated pre-existing
orchestrator failures and an SSH close test timeout. The Postgres-specific
check was skipped because `KANDEV_TEST_POSTGRES_DSN` was not available.
