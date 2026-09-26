---
id: "01-merge-baseline"
title: "Integrate the merged survival baseline"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 01: Integrate the merged survival baseline

## Summary

Import current main into the PR branch and preserve both implementations.

## In scope

Import current main into the PR branch and preserve both implementations.
- Prefer a normal merge after saving the planning commit. No history rewrite is required for this reconciliation.
- Re-read current PR/base heads. Preserve user edits and use an isolated candidate checkout when needed.
- Resolve all eleven paths with the plan's map. Inspect automatic merges at startup, shutdown, API, executor, and schema boundaries.
- Keep journal commit barriers and survival retention at every event producer. Do not publish an event after a failed journal write.
- Retain both UI test sets and typed error families. Make no whole-file ours/theirs choices.
- Update merged executor design and companion plan links for durable-versus-legacy adoption.
- Preserve upstream required-store registrations and both task schema additions. Add missing conformance coverage if the merged schema requires it.
- A compiling merge is an intermediate integration result. It is not release-ready until Tasks 02-05 pass.

## Out of scope

New survival runtimes, new flags, automatic context continuation, and unrelated CI fixes.

## Acceptance

1. No conflict markers remain, and both protocol identities and metadata sets survive the merge.
2. Production packages compile. Existing journal and recovery contract tests pass.
3. Both companion packages link this pending work without replacing historical results.

## Test cases

Preserve all changed tests from both sides. Use the existing journal, handler, and frontend recovery suites as merge guards.
Task 02 owns the new adoption tests. Task 03 owns end-to-end terminal replay tests.

## Verification

Run from the repository root. Proposed test files must exist before these commands pass.

```bash
git fetch origin main
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/agent/runtime/... ./internal/agentctl/... ./internal/orchestrator/... ./internal/backendapp -run '^$')
(cd apps/backend && go test ./internal/agentctl/journal ./internal/orchestrator/handlers -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts lib/services/session-recovery-service.test.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.test.ts`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/lib/services/session-recovery-service.test.ts`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `docs/specs/executors/system-design/agent-survival-across-restart-03.md`
- `docs/plans/agent-survival-across-restart/plan.md`

## Dependencies

None.

## Inputs

- [Plan and baseline](plan.md).
- [Adoption contract](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
- Read the scoped AGENTS.md files and the tests next to every changed implementation.

## Risks

The upstream branch can advance during implementation. Re-inventory changed paths after refresh.
Do not publish an intermediate merge that passes compilation but lacks safe durable adoption.

## Parallelism

`sequential`

## Results

Completed 2026-09-14. Current `origin/main` was merged as `f00f328dec` after
preserving durable delivery, survival ownership, recovery guards, and both
desktop/mobile recovery surfaces. The merge has no conflict markers and the
normal hooks passed after restaging one hook-formatted web test file.

The merge gates passed: backend compile and focused journal/recovery tests, web
typecheck and recovery tests, specification validation, and whitespace
validation. The implementation changes for adoption, replay, and integrated
restart behavior are recorded in Tasks 02 through 05.
