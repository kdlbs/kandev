---
id: "04-settings-ui"
title: "Workflow cancellation setting UI"
status: completed
wave: 3
depends_on: ["02-configuration-surfaces"]
plan: "plan.md"
spec: "../../specs/workflow/cancelled-turn-completion/spec.md"
---

# Task 04: Workflow cancellation setting UI

## Acceptance

- Frontend workflow-step normalization, create/update payloads, drafts, dirty comparison, and save flows preserve `cancel_triggers_turn_complete` without changing custom-step defaults.
- A localized, read-only-aware checkbox appears beneath configured turn-complete transitions, discloses destination auto-start behavior, and writes the shared step draft once.
- Desktop and phone use the same state/update logic; the phone's associated label is the actual 44px touch target and introduces no new drawer, nested scroll owner, or horizontal overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- components/settings/workflow-card-actions.test.ts components/settings/workflow-pipeline-editor-step-actions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm run i18n:ratchet && pnpm run i18n:check)
```

## Files Likely Touched

- `apps/web/lib/types/http.ts`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.test.tsx`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

Task 02.

## Parallelism

Parallel-safe with Task 05 after Task 02: frontend source/locales and public documentation files are disjoint.

## Inputs

- Spec `What`, `API Surface`, desktop/mobile settings scenario, and mobile design contract in `plan.md`.
- Adjacent `ExplicitCompletionToggle` and workflow dirty-state/save patterns.
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` as the nearest shipped phone interaction.

## Risks

- New copy must use the `settings` i18n namespace and pass the changed-line ratchet.
- The checkbox must be hidden when no transition exists and disabled for synced/read-only workflows without losing the persisted checked state.
- Do not fork business logic for mobile; only touch target/spacing may be responsive.

## Output Contract

Report the desktop/mobile interaction, translation keys, files changed, focused unit/type/i18n results, rendered-check evidence if performed, blockers, and residual risks. Update this task and `plan.md` status in the same conversation.

## Results

Implemented the frontend contract and editor control:

- `WorkflowStep`, template normalization, draft creation, create/update actions, dirty comparison, and the domain API now preserve `cancel_triggers_turn_complete`.
- Added the localized, read-only-aware checkbox beneath configured turn-complete transitions. It uses a shared draft update path, retains persisted state in read-only mode, gives the associated semantic label a `min-h-11` touch target, and does not add a mobile drawer or scroll owner.
- Added English and pseudo-locale keys describing explicit user cancellation and destination `on_enter` auto-start behavior.

Verification:

- `rtk pnpm --filter @kandev/web test -- app/actions/workspaces.test.ts components/settings/workflow-card-actions.test.ts components/settings/workflow-pipeline-editor-step-actions.test.tsx` — 27 passed.
- `rtk pnpm --filter @kandev/web run typecheck` — passed.
- `rtk pnpm run i18n:ratchet && rtk pnpm run i18n:check` from `apps/web` — passed; 808 referenced keys, 0 orphan keys.
- `rtk pnpm --filter @kandev/web exec vitest run lib/api/domains/workflow-api.test.ts components/settings/workflow-pipeline-editor-step-actions.test.tsx` — 5 passed, including create-step API forwarding coverage.

Residual risk: browser-level mobile rendering is covered by the Task 06 E2E scenarios rather than a separate screenshot run here.
