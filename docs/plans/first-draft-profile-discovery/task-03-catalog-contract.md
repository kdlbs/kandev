---
id: "03-catalog-contract"
title: "Preserve the structured catalog"
status: completed
wave: 3
depends_on:
  - "02-first-profile-browser-proof"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.5
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.4
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 03: Preserve the structured catalog

## Summary

Preserve native grouping through the existing live and settings projection paths.
Add a bounded optional catalog with a derived flat compatibility view, without changing current flat selection behavior.

## In scope

- Shared Go/TypeScript catalog types, validation, and serialization.
- Common ACP group conversion, typed/untyped compatibility, and the qualified Devin client capability.
- Stream, utility, host-cache, settings DTO, live snapshot, WebSocket, and boot-hydration projection.
- Flat-provider, older-record, malformed-metadata, and cross-provider handshake compatibility.

## Out of scope

- Fusion relationship decoding, UI navigation, new persistence owners, and changes to session mutation authority.

## Acceptance

1. Both live and probe paths preserve native group/choice identity, ordering, labels, descriptions, and exact values through DTO and snapshot round trips.
2. All three handshakes use the same qualified capability metadata, and flat providers/older snapshots retain their existing behavior.
3. Invalid richer data falls back to validated ordinary choices without leaking raw metadata or publishing partial composition bindings.

## TDD

First add `TestGroupedCatalogPreservedLiveAndProbe` fixtures against the actual conversion paths.
The RED assertion is a missing group ID/label after converting a typed ACP grouped option.
The temporary diagnostic tests proved this loss and were removed.
Use repeated choice labels in different groups and include descriptions and exact raw values.

Create `internal/common/modelcatalog` for neutral types and bounded validation.
Add shared ACP conversion in `internal/agentctl/acpcompat` and use it from live and utility callers.
Preserve untyped config-option compatibility and legacy-model fallback.
Add the Devin grouped extension through `ClientCapabilityMeta` without mutating input maps or changing other agents' metadata.

Thread the optional catalog through the existing named projection boundaries in the design.
Do not put the complete catalog into `sessionmodel.ConfigOption`, which only selects the model RPC.
Use one canonical TypeScript type imported by HTTP, session-payload, runtime-state, and selector types.
Add `TestGroupedCatalogRoundTrip` for stream, probe, host utility, DTO, and existing session snapshot serialization.
Add hydration/store tests that exercise the actual JSON-to-state mapping, not just structural typing.

## Verification

Run from the repository root after the new RED test and minimum implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/modelcatalog ./internal/agentctl/acpcompat ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/settings/controller -run 'Test.*(Catalog|Grouped|ClientCapabilityMeta|ConfigOption)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'Test.*(Catalog|SessionModel|ConfigOption)' -count=1)
(cd apps/web && pnpm exec vitest run lib/model-catalog.test.ts lib/model-catalog-selection.test.ts lib/state/slices/session-runtime/session-model-hydration.test.ts lib/state/slices/session-runtime/session-model-catalog-hydration.test.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Install frozen workspace dependencies once from `apps/` before the first pnpm command in a fresh worktree.
Run changed-file Go lint and frontend ESLint with the explicit paths from the actual diff, including new files.
Record their complete commands in Results.

## Files likely touched

- New `apps/backend/internal/common/modelcatalog/{catalog.go,catalog_test.go}`
- New `apps/backend/internal/agentctl/acpcompat/catalog.go` and its tests
- `apps/backend/internal/agentctl/acpcompat/cursor.go` and capability tests
- `apps/backend/internal/agentctl/server/adapter/transport/acp/meta_convert.go` and a new catalog conversion test file
- `apps/backend/internal/agentctl/server/utility/{acp_executor.go,types.go}` and new grouped-probe tests
- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agent/hostutility/{types.go,public.go}` and contract tests
- `apps/backend/internal/agent/settings/dto/dto.go` and controller projection helpers
- Existing lifecycle/orchestrator config-option copies and selector-snapshot serialization
- New `apps/packages/types/src/model-catalog.ts`
- `apps/web/lib/types/{http-agents.ts,session-runtime-payloads.ts}`
- `apps/web/lib/state/slices/session-runtime/{types.ts,session-model-hydration.ts,session-model-hydration.test.ts}`
- New `apps/web/lib/model-catalog.ts` validation/index helpers and `model-catalog.test.ts`

## Dependencies

Task 02. Read backend, agentctl, frontend, and relevant shared-package guidance before changes.

## Risks

- One missed projection silently restores flat data after reload or resume.
- Duplicate labels or repeated values cannot be normalized by display-name deduplication.
- Arbitrary vendor metadata must not become persisted catalog data.

## Parallelism

`sequential`

## Inputs

- [Catalog design](../../specs/agents/system-design/grouped-model-catalog.md)
- [Requirements 001 and 002](../../specs/agents/requirements/dynamic-provider-options.md)
- [Architecture decision](../../decisions/2026-11-03-provider-neutral-model-catalog.md)
- Existing `conversion_test.go`, utility grouped fixtures, `ClientCapabilityMeta` tests, and session hydration tests.

## Results

Completed 2026-11-03.

- New neutral package `internal/common/modelcatalog` with bounded validation, a derived flat projection, and exact composition resolution. Tests cover bounds, repeated labels, ambiguity, unknown values, and unadvertised models.
- New `internal/agentctl/acpcompat/catalog.go` converts typed ACP groups and the untyped `_meta.configOptions` group shape. Invalid or oversized richer data publishes no catalog, so the validated flat choices remain.
- `ClientCapabilityMeta` now adds `cognition.ai/groupedSessionConfigOptions` for Devin only and keeps the Cursor opt-in unchanged.
- The optional catalog threads through stream config options, the utility probe, host-utility results/caches, settings DTOs, and the live session snapshot JSON. Flat providers and older snapshots keep `catalog` absent.
- Frontend: `@kandev/types/model-catalog`, `lib/model-catalog.ts` validation/resolution, hydration and WebSocket mapping, and selector types.
- Commands: `go test -trimpath -tags fts5 ./internal/common/modelcatalog ./internal/agentctl/acpcompat ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility -count=1` passed. `go test -trimpath -tags fts5 ./internal/agent/hostutility ./internal/agent/settings/controller -count=1` passed. `pnpm exec vitest run lib/model-catalog.test.ts lib/state/slices/session-runtime/session-model-catalog-hydration.test.ts` passed.
