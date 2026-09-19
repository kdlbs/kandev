---
id: "02-expose-duplicate-action"
title: "Expose duplicate action"
status: done
wave: 2
depends_on: ["01-build-duplication-drafts"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-duplication.md"
---

# Task 02: Expose Duplicate Action

## Acceptance

- Every clean persisted Kanban workflow exposes a localized Duplicate action. A sync-managed source remains copyable into a manual draft.
- A temporary or dirty source shows a localized save-first explanation. A load error shows an error message and creates nothing.
- The direct mobile action has at least a 44px active height, remains inside the wrapping card footer, and public workflow guidance documents the save and exclusion semantics.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/settings/workflow-card-header-actions.test.tsx app/settings/workspace/use-workflow-creation.test.ts && pnpm --filter @kandev/web i18n:check && pnpm --filter @kandev/web i18n:ratchet && cd .. && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-card-header-actions.tsx`
- `apps/web/components/settings/workflow-card-header-actions.test.tsx`
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/src/locales/en/workflows.json`
- `docs/public/workflow-tips.md`

## Dependencies

Task 01.

## Parallelism

Sequential. The UI calls the draft constructor and parent insertion handler from Task 01.

## Inputs

- Spec sections: What, Permissions, Failure Modes, mobile Scenario.
- Plan sections: Duplicate action, Mobile design contract, Public documentation.
- Existing `WorkflowCardHeaderActions`, `useRequest`, toast, tooltip, and manual-save contributor patterns.

## Risks

- Treat sync read-only as a source property only. Do not mark the new draft read-only.
- Do not copy a dirty in-memory editor state while the contract promises saved configuration.
- Preserve keyboard accessibility and do not make hover the only discovery path.

## Output Contract

Report desktop/mobile entry points, disabled/error behavior, translations, public docs impact, files changed, exact checks run, blockers, risks, and task/plan status updates.

## Results

- Added the localized Duplicate action between Export and Delete with a 44px phone hitbox, loading state, stable test ID, and disabled explanations for Improve Kandev, unsaved sources, and pending mutations.
- Added authoritative saved-step loading with error toast handling and a ref-backed same-render request gate. Sync-managed sources remain copyable because the new draft is manual and independent.
- Kept the request and disabled-state logic in `use-workflow-duplication.ts` and moved dialog composition into `workflow-card-dialogs-content.tsx` to keep the main workflow card below the configured size limit.
- Added English and pseudo-locale strings and documented the save-first flow, numbered names, copied settings, and task/history exclusions in `docs/public/workflow-tips.md`.
- Checks passed:
  - `cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-card-header-actions.test.tsx app/settings/workspace/workflow-duplication.test.ts app/settings/workspace/use-workflow-creation.test.ts app/settings/workspace/use-workflow-duplication.test.ts` (4 files, 20 tests including same-render double activation and loading-label coverage).
  - `cd apps/web && pnpm run typecheck`.
  - `cd apps && pnpm --filter @kandev/web lint` passed.
  - `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`.
  - `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs` passed 59 tests and validated 41 docs pages.
