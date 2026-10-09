---
id: "01-configuration"
title: "Preserve policy and workflow configuration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.1
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.2
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.9
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.10
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---
# Task 01: Preserve policy and workflow configuration

## Summary

Add the optional candidate policy and workflow veto through storage, APIs, and browser data paths. Runtime decisions remain unchanged until Task 02.

## In scope

- Add optional `unclassified` policy, canonical disabled normalization, and threshold bounds in DTO and runtime validation.
- Propagate `disable_unclassified_fallback` through workflow models, migration, requests, export/import, clone, sync, and events.
- Use presence-aware updates. Cover omitted, null, false, and true with the design's distinct rules.
- Audit task-side step projections and all positional SQL scans, inserts, and copies using the existing profile-session-policy field as reference.
- Preserve policy in browser normalization, serialization, editor state/drafts, and workflow API/WS state. No rendered controls change.
- Add `TestUnclassifiedPolicyValidation`, `TestUnclassifiedPolicyRoundTrip`, `TestUnclassifiedStepVetoRoundTrip`, and `TestUnclassifiedStepVetoPayload`.
- Use `workflow_profile_session_policy_test.go`, `profile_session_policy_test.go`, and `export_test.go` as nearby propagation patterns.

## Out of scope

Automatic unknown-error retry loops, shared health changes, Office/utility enablement, and new visual settings controls.

## Acceptance

- Old documents and fresh/upgrade databases retain disabled behavior. Invalid input changes no persisted state.
- API, SQL, workflow import/export/sync/copy, and event round trips preserve the veto and enabled threshold.
- Browser read-edit-save preserves enabled policy on desktop and phone. Classified-policy tests continue to pass.

## Verification

Run from the repository root. New tests must fail for the intended missing behavior before production edits.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingpolicy ./internal/agent/settings/controller ./internal/agent/settings/store ./internal/workflow/... ./internal/task/models ./internal/task/repository/sqlite)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test lib/api/domains/agent-profile-normalize.test.ts lib/api/domains/workflow-api.test.ts lib/ws/handlers/workflows.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/routingpolicy/policy.go`
- `apps/backend/internal/agent/runtime/routingpolicy/policy_test.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/dynamic_policy.go`
- `apps/backend/internal/agent/settings/controller/dynamic_profile_test.go`
- `apps/backend/internal/agent/settings/store/sqlite_dynamic_profile_test.go`
- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/sync_apply.go`
- `apps/backend/internal/workflow/handlers/handlers.go`
- `apps/backend/internal/workflow/handlers/ws_handlers_test.go`
- `apps/backend/internal/workflow/stepevents/stepevents.go`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/config/workflows/`
- `apps/web/lib/types/agent-profile.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/api/domains/agent-profile-normalize.ts`
- `apps/web/lib/api/domains/agent-profile-normalize.test.ts`
- `apps/web/lib/api/domains/workflow-api.ts`
- `apps/web/lib/api/domains/workflow-api.test.ts`
- `apps/web/lib/ws/handlers/workflows.ts`
- `apps/web/lib/ws/handlers/workflows.test.ts`
- `apps/web/components/settings/dynamic-agent-profile-editor-draft.ts`
- `apps/web/components/settings/dynamic-agent-profile-editor-state.ts`

## Dependencies

None.

## Risks

An omitted field in any projection or whole-document save can silently disable policy. Reuse existing migration conventions for every supported database dialect.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/dynamic-agent-routing.md), `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002`.
- [System design](../../specs/agents/system-design/dynamic-unclassified-fallback.md), all sections.
- [Package evidence and test matrix](plan.md).
- [Decision](../../decisions/2026-09-28-repeated-unclassified-fallback.md).

## Results

- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingpolicy ./internal/agent/settings/controller ./internal/agent/settings/store ./internal/workflow/... ./internal/task/models ./internal/task/repository/sqlite)`: passed before the final SQLite-backed controller assertion was added.
- `(cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/store)`: passed after adding API-to-SQLite policy persistence and invalid-update non-mutation coverage.
- `(cd apps && pnpm install --frozen-lockfile)`: passed; dependencies were already current.
- Focused web tests covering policy normalization, workflow API/WS, editor save state, workflow step save equality, and workspace actions: 8 files and 68 tests passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `git diff --check`: passed.
