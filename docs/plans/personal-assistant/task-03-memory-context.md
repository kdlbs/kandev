---
id: "03-memory-context"
title: "Scoped memory and worker handoffs"
status: in_progress
wave: 2
depends_on: ["01-durable-intake"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-002
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-002.1
  - AC-ORCHESTRATION-ASSISTANT-002.2
  - AC-ORCHESTRATION-ASSISTANT-002.3
  - AC-ORCHESTRATION-ASSISTANT-002.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 03: Scoped memory and worker handoffs

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S07, S08, S20, and [plan](plan.md), Backend 3. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Confirmed relevant preferences survive newer activity and fit a deterministic bounded packet with provenance and overflow references.
2. Two eligible tasks receive the same scoped synthetic credential descriptor without secret values; unrelated accounts/scopes cannot retrieve it.
3. Edit/forget/expiry invalidate future packets and queued deliveries, and legacy memory is never promoted to global confirmed user policy.

## Likely files

- apps/backend/internal/orchestration/models/context.go (new); repository/sqlite/memory.go
- apps/backend/internal/orchestration/repository/sqlite/context.go, credential_descriptors.go, memory_test.go (new)
- apps/backend/internal/orchestration/runtime/memory.go, context.go, context_test.go (new files where absent)
- apps/backend/internal/backendapp/adapters_workspace_tasks.go, adapters_workspace_results.go
- apps/backend/internal/integrations/secretadapter (reuse, read scoped AGENTS.md first)

## Implementation sequence

Add additive memory fields and owner mutation API. Implement the 12-KiB packet/1-KiB excerpt budgets with protected objective/account constraints and exact scoped continuation references. Wire packet snapshots into delegation and follow-up, and revision checks at dispatch. Use a fake locked/unlocked resolver with synthetic canary values; never query the user's vault to test memory.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Memory|Context|Credential)|TestEachTurnUsesCurrentGlobalRoleAndWorkspaceContext')
```

## Dependencies and risks

Dependencies: `01-durable-intake`. Execute in the primary session unless the user explicitly authorizes subagents.

Deletion cannot erase already-delivered provider context or historical backups. Secret redaction is not a license to ingest a whole vault. Application memories are not grants.

## Output

A shared context contract and human-editable memory with secret-free credential knowledge.

## Remaining implementation checklist

1. Preserve the implemented packet/queue/owner invariants recorded below; do not
   redo task 01/02 or replace scoped references with task metadata.
2. Extend `CredentialHealthReader` from a bare health string to a typed,
   response-only validation result. Bind descriptor revision/profile/reference
   and configuration generation; include a timestamp only after a real metadata
   check. Return unknown/locked/missing/unavailable distinctly. Never reveal or
   enumerate vault secrets. No schema change is required for this response data.
3. Update the metadata-only backend adapter and synthetic resolvers together.
   Client-submitted validation time/status must be rejected or ignored under the
   existing strict DTO contract. A changed descriptor/configuration is rechecked
   on the next read; no cached ready state may outlive its inputs.
4. Complete owner memory listing with stable bounded pagination and a cursor bound
   to owner/filter/order. Default 50, cap 100; reject malformed/foreign cursors.
   Cover more than one full page, deterministic tie ordering, expiry/forgotten
   filtering and concurrent edits without exposing another owner's data.
5. Add explicit scope validation at creation/update/read and packet assembly for
   workspace/project/task/environment combinations. Resolve referenced objects
   through authorized native adapters; do not trust a caller-supplied scope ID.
   Recheck CAS before writes and prove a rejected update leaves stored data intact.
6. Inspect complexity warnings from the current normal lint rather than assuming
   an older implementation still violates thresholds. Extract cohesive helpers
   only if needed, with existing behavior preserved. Keep descriptor/API docs in
   sync and do not label task complete until all new tests execute.

## Detailed evidence map

| Criterion | Planned evidence | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-002.1 | `TestAssistantMemoryOwnerPagination` and `TestAssistantMemoryScopeValidation` in runtime/context tests | >100 rows, foreign owner/cursor, bad scope, CAS conflict, no mutation on rejection |
| AC-ORCHESTRATION-ASSISTANT-002.2 | Existing context budget/ordering tests plus `TestAssistantContextPaginationContinuations` | Mandatory overflow, UTF-8 boundary, stable scoped references |
| AC-ORCHESTRATION-ASSISTANT-002.3 | `TestAssistantCredentialValidationMetadata` in runtime and adapter tests | Synthetic two-worker handoff, locked/missing/unavailable, forged timestamp, canary redaction |
| AC-ORCHESTRATION-ASSISTANT-002.4 | Existing queue/native guard tests and `TestAssistantContextScopeNarrowing` | Forget/expiry/revision change, queued restart, launch/resume/steer/send-now |

The work order's existing test filter matches these `TestAssistantMemory`,
`TestAssistantContext` and `TestAssistantCredential` prefixes. Also run the
queue/executor race commands already recorded below after touching those seams.

## Scope boundaries and delivery

Human memory editing UI belongs to task 08; this task provides the owner API and
packet/validation contract. Integrating a real new vault provider is not required.
Commit the completion separately and update the experimental API reference.
Record existing baseline passes as historical evidence, then append new command,
count and SHA results. The old “no commits were made” statement below refers to
the original implementation checkpoint, not the private publication.

## Parallelism

`sequential`

## Results

Implemented additive memory scope/provenance/confirmation/revision/expiry/forgetting, owner CAS APIs, deterministic 12-KiB packets and 1-KiB UTF-8 excerpts, protected mandatory constraints, stable context references, and typed credential descriptors. Kandev credential health uses metadata-only scoped lookup; Bitwarden remains explicitly unavailable without an attached resolver. Tests use synthetic locked/ready/unavailable resolvers and canary tokens; no vault was scanned.

Delegations and follow-ups carry server-loaded packets. Idle assignment refresh replaces the old packet while retaining the base instruction. Core queues persist the original per-message reference across restart; manual merge and send-now reject mixed references. Optional native executor guards cover launch/process start, resume, prompt, steer and PTY paths; the feature-off path cannot bypass a recorded handoff's guard. Stale queued messages are retained for recovery rather than sent with newer task metadata.

Observed red/green: descriptor endpoint missing; known token not redacted; manual merge discarded a context revision; native dispatch ignored its guard; unknown packet accepted for delegation; idle assignment retained old context; context CLI command missing. Existing confirmed-preference and prompt-selection red tests were also fixed. Migration fixtures now explicitly represent the legacy column layout and assert unconfirmed workspace scope.

Passing checks (repository Go 1.26.0):

```sh
GIN_MODE=release go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Memory|Context|Credential)|TestEachTurnUsesCurrentGlobalRoleAndWorkspaceContext' -v
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/... ./cmd/agentctl
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestrator/messagequeue ./internal/orchestrator/executor
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestrator ./internal/backendapp -run 'TestAssistant|TestWorkspace|TestSteer|Test.*Queued|Test.*SendNow|Test.*AutoStart'
```

The exact task filter selected **10 tests**: 8 runtime (7 new plus the named existing role/workspace-context compatibility test) and 2 backend adapter tests. Its repository/root packages have no matching tests; their migration/compatibility tests passed in the full orchestration race command. The executor guard regression additionally tests five native dispatch variants, and the queue test covers automatic/manual/send-now merge behavior. The SQLite restart experiment verifies a forgotten-memory packet stays stale even after task metadata is refreshed.

Those passes preceded the binding-switch privacy regression. The [ownership repair](ownership-checkpoint.md) is now implemented and verified, and this task's exact filter passed again (**10 tests**). The unchanged redactor was moved to a dependency-free common package to remove a task-service test import cycle. This task remains **in_progress**: finish descriptor validation-time metadata, dedicated owner-list pagination/scope-validation coverage and function-complexity checks before marking done. No frontend/live-provider experiment or production deployment was performed. No commits were made.

The docs-maintainer skill added the experimental backend API/CLI reference and corrected the old eight-memory description. Its checks passed: `node --test scripts/validate-public-docs.test.mjs` (**61 tests**) and `node scripts/validate-public-docs.mjs` (**43 pages**). The reference now documents retained ownership while still disclosing the incomplete UI/read-only enforcement; it is not a release/readiness claim.
