---
id: "01-cycle-analyzer"
title: "Workflow replay cycle analyzer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow-cycle-guardrails/spec.md"
---

# Task 01: Workflow replay cycle analyzer

## Acceptance

- A pure deterministic analyzer returns the diagnostic contract from the spec
  for cycles over `on_turn_start` and `on_turn_complete` move actions.
- Fully automatic versus user-mediated classification accounts for source-step
  auto-start, trigger, and approval, and ignores cycles without auto-start
  re-entry through `on_turn_complete`. An `on_turn_start` edge into an
  auto-start step is covered as a safe non-replay regression.
- Trace choice, identities, affected steps, dangling targets, reorder behavior,
  and all three prompt-source categories have focused unit coverage.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/workflows/replay-cycle-analysis.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/workflows/replay-cycle-analysis.ts`
- `apps/web/lib/workflows/replay-cycle-analysis.test.ts`

## Dependencies

None.

## Inputs

- Spec sections `What`, `Analysis Contract`, and `Scenarios`.
- `apps/web/lib/types/workflow-actions.ts` for action shapes.
- `apps/backend/internal/workflow/engine/types.go` for transition resolution
  and trigger semantics.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` for the
  auto-start/user-turn distinction.

## Output Contract

When finished, change this task's `status` to `done`, check it in `plan.md`, and
report the analyzer API, files changed, tests run, blockers, and residual risks.
