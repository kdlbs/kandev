---
id: "05-kanban-recovery-convergence"
title: "Kanban recovery convergence"
status: pending
wave: 5
depends_on: ["04-dynamic-conductor-policy-integration"]
plan: "plan.md"
spec: "../../specs/platform/provider-error-recovery.md"
---

# Task 05: Kanban recovery convergence

- **Acceptance:** Make Kanban concrete-profile recovery consume shared class
  and timing metadata, remove its provider-code allow-list, retain its explicit
  interactive defaults, and keep unknown or effect-unsafe failures manual.
- **Files likely touched:**
  `apps/backend/internal/orchestrator/{event_handlers_transient,event_handlers_agent}.go`,
  Kanban retry state/event DTOs, and focused orchestrator tests.
- **Dependencies:** Task 04.
- **Parallelism:** sequential.
- **Inputs:** Provider Error Recovery Use across Kanban and effect-safety;
  current `routingerr.Decide(ContextKanban)` call sites and interactive retry
  persistence.
- **Output contract:** Report the removed allow-list, retained concrete
  defaults, timing behavior, cancellation/generation evidence, files changed,
  exact commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/orchestrator/... ./internal/agent/runtime/routingerr/...`
- **Risks:** Concrete-profile defaults must not silently become dynamic profile
  defaults. Existing manual recovery remains authoritative after effects.

## Results

Pending.
