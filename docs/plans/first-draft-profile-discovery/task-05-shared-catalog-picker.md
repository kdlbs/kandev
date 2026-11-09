---
id: "05-shared-catalog-picker"
title: "Build the shared hierarchical picker"
status: completed
wave: 5
depends_on:
  - "03-catalog-contract"
  - "04-fusion-qualification"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.5
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 05: Build the shared hierarchical picker

Issue #5053 adds an explicit rendered singleton outcome under AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5.
The [singleton repair work order](../model-catalog-selector-repair/task-01-singleton-presentation.md) owns that correction.
This completed work order retains its original scope and historical results. Its pure-tree singleton coverage does not prove heading suppression.

## Summary

Extend the existing selector with generic group navigation and exact composition drafts.
Use one shared state model with desktop popover and phone sheet presentation.

## In scope

- Pure tree search, paths, valid-combination filtering, and ambiguity detection.
- Back navigation, selected context, in-picker composition, loading/retry, and cancellation.
- One phone sheet, internal navigation, safe areas, focus return, and viewport draft continuity.
- Localized shared controls and a second synthetic provider with different dimension identifiers.

## Out of scope

- Provider decoding, a second selection store, persistence, or a Devin-specific renderer.
- Per-surface mutation integration, owned by Task 06.

## Acceptance

1. Flat, grouped, nested, and composed catalogs use one selector without vendor branches or invented model IDs.
2. Complete unique combinations enable Apply. Invalidated choices, stale resolution, and ambiguous combinations remain visible and cannot apply.
3. Desktop and phone preserve the same unfinished selection through navigation, cancellation, and viewport changes.

## ASCII UI preview

Use [UI-GROUP-01 through UI-GROUP-04](plan.md#ascii-ui-preview).

```text
Desktop: < Models                     Group
         Dimension A [current value v]
         Dimension B [current value v]
         Thinking    [provider value v]
                    [Use configuration]

Phone:   < Models                     Group
         Dimension A
         [current value                  >]
         Dimension B
         [current value                  >]
         [       Use configuration        ]

Invalid: Choose an available replacement.
         [Use configuration (disabled)]
```

Dimension labels come from the catalog. The renderer never recognizes Lead or Sidekick as special keys.
The phone sheet owns one vertical scroller and replaces its body for dimension choices.
Shared primitives provide 28px desktop and at least 44px touch targets.

## TDD

Add `lib/model-catalog-selection.test.ts` before the pure selection helper.
Cover repeated labels, scoped search, stable order, singleton groups, incomplete/ambiguous combinations, and invalidated choices.
Include a synthetic provider with dimensions named `planner` and `worker-size`, rather than Devin's identifiers.
Use arbitrary exact advertised values so name parsing cannot make the test pass.

Add component tests for the existing flat path and new hierarchy/composition path.
Require provider-dependent options to hide while resolving and replace their snapshot afterward.
Keep a current model outside the active group discoverable.
Cover Back, Escape, cancellation, focus return, and viewport changes without an Apply callback.
Expose one generic completed-configuration callback for Task 06, alongside existing ordinary model selection.

Use shared primitives for the phone shell rather than importing task-owned navigation state.
Add all copy in seven locales. Generate Traditional Chinese through the existing conversion command.

## Verification

Run from the repository root after the fresh-worktree dependency install, if needed.

```bash
(cd apps/web && pnpm exec vitest run lib/model-catalog-selection.test.ts components/model-config-selector.test.tsx components/model-config-selector-custom-model.test.tsx components/model-config-selector-catalog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/model-catalog-selection.ts lib/model-catalog-selection.test.ts lib/model-catalog.ts lib/model-catalog.test.ts components/model-config-selector.tsx components/model-config-selector-content.tsx components/model-config-selector-rows.tsx components/model-catalog-groups.tsx components/model-catalog-composition.tsx components/model-config-sheet.tsx components/model-config-selector-catalog.test.tsx hooks/use-model-catalog-navigation.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Add any additional component/test path to the exact lint and test commands and record it in Results.
Task 07 owns the package's rendered desktop/phone proof.

## Files likely touched

- New `apps/web/lib/model-catalog-selection.ts` and its tests
- `apps/web/components/model-config-selector.tsx` and selector tests
- `apps/web/components/model-config-selector-content.tsx` and content tests
- New `apps/web/components/model-catalog-picker.tsx` and focused extracted presentation components
- Shared phone picker shell where extraction avoids a task-domain dependency
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/agents.json`

## Dependencies

Tasks 03/04 and the linked permission controls precede final UI integration.

## Risks

- Automatic replacement of an invalidated dimension can silently select another pairing.
- Duplicate mounted selectors can lose focus or expose two active drafts.
- A phone presentation must retain shared state rather than mount another hidden desktop picker.

## Parallelism

`sequential`

## Inputs

- [Shared selector design](../../specs/agents/system-design/grouped-model-catalog.md#shared-selector)
- [Requirements 002 through 004](../../specs/agents/requirements/dynamic-provider-options.md)
- Existing selector components and mobile picker interaction precedents.

## Results

Completed 2026-11-03.

- `lib/model-catalog-selection.ts` owns tree building, scoped search (labels, values, group paths, dimension labels/values through bindings), pruning, dimension availability, invalid-dimension detection, exact completion, and initial drafts. `lib/model-catalog-selection.test.ts` uses a synthetic provider with `planner` and `worker-size` dimensions and opaque model values.
- `components/model-config-selector.tsx` keeps the unfinished composition draft per group in `hooks/use-model-catalog-navigation.ts`. Back to the model list, dismissal, dimension Back, and viewport changes retain it; re-entering the same group restores the draft. A superseded catalog or changed owning model invalidates it.
- `components/model-catalog-groups.tsx` renders native groups and opens a focused composition view. Catalog leaves always keep their full group path in the command-filter keywords, so a provider description cannot hide them from a group-name search.
- `components/model-catalog-composition.tsx` renders one row per dimension, the candidate model's real provider options, an explicit Apply action, and visible invalid state. Back, dimension, choice, Apply, and provider-option controls use the shared density: 28px on fine-pointer desktop, at least 44px below the 768px phone boundary and on coarse pointers.
- `components/model-config-sheet.tsx` is the phone shell: fixed header, one vertical scroller with a sticky Apply action, safe-area padding, and focus return to the trigger.
- Component regressions cover Back navigation, dismissal, viewport changes with the same unfinished draft, candidate-option loading/failure/retry, path search, and phone sizing classes.
- Commands: `pnpm exec vitest run lib/model-catalog-selection.test.ts components/model-config-selector.test.tsx components/model-config-selector-custom-model.test.tsx components/model-config-selector-catalog.test.tsx` passed. `pnpm run i18n:check` passed after adding the six new copy keys in seven locales.
