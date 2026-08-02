---
id: "04-editor-and-carry-analysis"
title: "Build the conditional rule editor and carry-forward diagnostics"
status: done
wave: 2
depends_on: ["01-workflow-contract-and-validation"]
plan: "plan.md"
spec: "../../specs/workflow-session-settings/spec.md"
---

# Task 04: Build the Conditional Rule Editor and Carry-forward Diagnostics

## Acceptance

- The step editor presents keep-current, fixed-profile, and conditional-original-session behaviors with visible explanatory copy and mutually exclusive state.
- Authors can add at most one rule per discovered agent family and select `set`, `keep`, or `restore_original`.
- `set` reuses `ModelConfigSelector` with that family's advertised models and all ACP select options; no option names or values are hard-coded.
- Existing unavailable rules remain readable and removable, while capability discovery failure prevents saving a new unverifiable `set` rule.
- A pure fixed-point utility analyzes configured next/previous/explicit transitions, joins, and cycles per family and warns when changed values may carry into a step with no explicit decision.
- Warning actions create `keep`, `restore_original`, or `set` intent and resolve the diagnostic for that family.
- Read-only synced workflows display rules and warnings without editable controls.
- On phones, rule cards use one-column composition, one document scroll owner, 44px touch targets, and no horizontal overflow; desktop keeps the denser row treatment.

## TDD sequence

1. Add failing pure utility tests for straight paths, joins, relative/explicit edges, cycles, restore, and keep.
2. Add failing editor component tests for mode exclusivity, rule uniqueness, serialization, discovery failure, and read-only state.
3. Implement the graph utility by extracting/reusing existing transition resolution, then build small rule editor components around the shared selector.
4. Add responsive component assertions and keep business logic shared across desktop/mobile composition.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/workflows components/settings
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- New focused components under `apps/web/components/settings/`
- `apps/web/components/model-config-selector.tsx` only if a small reusable API is missing
- `apps/web/hooks/domains/settings/use-available-agents.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/workflows/replay-cycle-analysis.ts`
- New `apps/web/lib/workflows/session-config-carry-analysis.ts`
- Related utility and component tests

## Dependencies

Task 01. This task can proceed in parallel with Task 02.

## Inputs

- Spec sections `What`, `State machine`, and mobile/read-only scenarios.
- Existing `StepAgentProfileSelect`, `ModelConfigSelector`, `useAvailableAgents`, workflow cycle diagnostics, and responsive breakpoint/touch patterns.
- Mobile reference: current workflow settings phone layout and shared model-selector touch disclosure.

## Output contract

Implemented the mutually exclusive conditional editor, stable agent-family choices, shared `ModelConfigSelector` reuse, read-only rendering, and 44px mobile controls. Carry analysis now follows configured on-turn start/complete transition edges with an original/changed fixed-point traversal that handles joins, cycles, restore, and explicit keep decisions; manual card moves remain outside the graph. Component and utility tests passed, as did the full affected frontend test slice and typecheck. Fresh desktop and mobile screenshots show the editor and shared picker without horizontal overflow.
