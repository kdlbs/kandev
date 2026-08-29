---
spec: docs/specs/ui/requirements/dialog-content-containment.md
design: docs/specs/ui/system-design/dialog-content-containment.md
created: 2026-08-26
status: complete
---

# Implementation Plan: Contain Growing Dialog Content

## Overview

Five dialogs let repeated or plugin-provided content determine the outer modal
height. Long content can therefore grow beyond the viewport and make close,
completion, or item actions difficult or impossible to reach. Apply the same
bounded-shell pattern locally to each surface, with one designated scroll body
and surface-specific persistent controls.

## Root cause

The affected Dialog and AlertDialog contents have no dynamic-viewport cap and
no child with both a bounded row and `min-height: 0`. Their repeated content is
rendered in normal content-sized flow, so the outer modal expands instead of
creating an internal scroll range. The shared primitives behave as designed;
the missing constraint belongs to each composition.

The plugin drawer is not affected because it already combines a `dvh` cap with
a `min-h-0 overflow-y-auto` content body.

## Scope

- Contain the agent-profile deletion conflict list and preserve its footer.
- Contain Marketplace source rows and preserve the add-source form.
- Contain System Health issue cards and keep every Fix action reachable.
- Contain opaque plugin dialog content while preserving dismissibility rules.
- Contain the Office Create Project form and preserve its action footer.
- Add focused desktop and phone browser geometry regressions for every surface.

## Exclusions

- No backend, API, persisted model, store, or plugin SDK changes.
- No remediation buttons or changed conflict semantics in profile deletion.
- No shared Dialog or AlertDialog primitive behavior change.
- No plugin drawer change and no forced change to plugin presentation choice.
- No user-facing copy changes and no audit expansion beyond these five dialogs.

## Technical approach

For each affected content surface:

1. Add a dynamic-viewport maximum height and outer overflow containment.
2. Use explicit auto/minmax/auto rows, or the equivalent flex composition.
3. Give the growing body `min-h-0`, vertical auto overflow, overscroll
   containment, and a stable browser-test selector.
4. Keep the local header and designated persistent controls outside the body.
5. Preserve short-content intrinsic height and existing action semantics.

Do not introduce a speculative shared wrapper. The five components use
different primitives and different persistent-row semantics; the established
utility-class composition is small and clearer when applied locally.

## Mobile design contract

- Centered dialog presentation remains unchanged; plugin drawer presentation
  remains the existing bottom drawer.
- Every affected dialog fits within the dynamic phone viewport and has exactly
  one new vertical scroll owner.
- Required phone actions are at least 44 pixels high and remain reachable.
- Long labels, URLs, repository chips, and plugin content do not create
  document horizontal overflow.
- Phone and desktop retain equivalent content, actions, and outcomes.

## Testing strategy

Follow RED-GREEN-REFACTOR separately for each work order. The RED case must use
enough real or intercepted data to prove the current modal exceeds the
viewport. The GREEN case asserts the outer dialog bounds, body scroll range,
final-item reachability, persistent controls, and phone overflow/touch rules.

Component tests remain focused on callback and conditional-rendering behavior.
They may assert stable layout seams, but jsdom is not accepted as geometry
evidence.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- components/settings/agent-profile-delete-dialog.test.tsx components/settings/plugins/marketplace-sources-dialog.test.tsx components/system-health/health-indicator.test.tsx components/plugins/plugin-modal-host.test.tsx)
(cd apps/web && pnpm e2e:run --host -- tests/settings/agent-profile-delete.spec.ts tests/settings/plugin-marketplace-sources.spec.ts tests/system-health-dialog.spec.ts tests/plugins/plugins.spec.ts tests/office/project-repository-picker.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/settings/mobile-agent-profile-delete.spec.ts tests/settings/mobile-plugin-marketplace-sources.spec.ts tests/mobile-system-health-dialog.spec.ts tests/plugins/mobile-plugin-modal.spec.ts tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/agent-profile-delete-dialog.tsx components/settings/plugins/marketplace-sources-dialog.tsx components/system-health/health-indicator.tsx components/plugins/plugin-modal-host.tsx app/office/projects/create-project-dialog.tsx e2e/tests/settings/agent-profile-delete.spec.ts e2e/tests/settings/mobile-agent-profile-delete.spec.ts e2e/tests/settings/plugin-marketplace-sources.spec.ts e2e/tests/settings/mobile-plugin-marketplace-sources.spec.ts e2e/tests/system-health-dialog.spec.ts e2e/tests/mobile-system-health-dialog.spec.ts e2e/tests/plugins/plugins.spec.ts e2e/tests/plugins/mobile-plugin-modal.spec.ts e2e/tests/office/project-repository-picker.spec.ts e2e/tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Implementation waves

One sequential wave. The work orders are logically independent and have no
data dependencies, but each owns its own RED-GREEN evidence and should be
completed before the next surface starts.

- [x] [Task 01: Contain agent conflict details](task-01-agent-profile-conflict.md)
- [x] [Task 02: Contain Marketplace sources](task-02-marketplace-sources.md)
- [x] [Task 03: Contain health issue cards](task-03-health-issues.md)
- [x] [Task 04: Contain plugin dialog content](task-04-plugin-dialog.md)
- [x] [Task 05: Contain Create Project form](task-05-create-project.md)

## Risks and boundaries

- `minmax(0, 1fr)` and `min-h-0` are both required; omitting either can restore
  content-sized expansion.
- Marketplace and Office need their add/completion rows outside the scroll body;
  moving them into it would technically contain the dialog but fail the
  reachability outcome.
- Plugin content is opaque. The host can guarantee reachability through scroll,
  but cannot infer or extract a plugin-owned footer.
- Health Fix actions remain in the scroll body because they are item-local.
- Test fixtures must preserve actual API and plugin-host shapes so the browser
  regressions exercise production composition paths.

## Public documentation

No public documentation change. This restores reachability of existing UI and
does not add or rename a user-facing capability.

## Open questions

None.

## Results

- All five dialogs use local dynamic-viewport containment with one designated
  vertical scroll body. The shared Dialog and AlertDialog primitives, APIs,
  business actions, and plugin drawer remain unchanged.
- Agent conflict, Marketplace Sources, System Health, packaged plugin modal,
  and Office Create Project cases each have focused desktop Chromium and
  mobile-Chrome geometry coverage. The Office shell uses `overflow: clip` so
  the existing inline repository picker cannot move the dialog horizontally.
- The RED/GREEN work-order evidence is recorded in the five task files.
- Final verification passed: 24 focused component tests, 29 focused desktop
  Chromium tests, 7 focused mobile-Chrome tests, web typecheck, targeted ESLint
  with no errors or warnings, i18n check and ratchet, specification lint, and
  `git diff --check`.
