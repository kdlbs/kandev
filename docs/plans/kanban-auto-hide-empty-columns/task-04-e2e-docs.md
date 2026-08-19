---
id: "04-e2e-docs"
title: "Prove end-to-end behavior and document it"
status: done
wave: 4
depends_on: ["03-drag-targets"]
plan: "plan.md"
spec: "../../specs/kanban-auto-hide-empty-columns/spec.md"
---

# Task 04: Prove end-to-end behavior and document it

## Acceptance

- Desktop E2E covers default-off compatibility, persisted enablement, empty-column collapse, search
  stability, drag reveal, cancellation, successful drop, and manual-hidden precedence.
- Mobile E2E covers focused-workflow isolation, a 44 CSS px toggle surface, move-target recovery, and
  absence of document overflow.
- Public tasks/workflows documentation and the feature-spec index explain the new preference and its
  distinction from manual hiding.

## Likely files

- `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-auto-hide-empty-columns.spec.ts`
- `apps/web/e2e/pages/kanban-page.ts` if a shared locator is justified
- `docs/public/tasks-and-workflows.md`
- `docs/specs/INDEX.md`
- this plan and its task files for final status and command evidence

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/kanban/auto-hide-empty-columns.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/kanban/mobile-auto-hide-empty-columns.spec.ts)
make fmt
make typecheck
make test
make lint
```

## Risks

- E2E must wait on causal task/settings updates rather than unconditional sleeps.
- The tests must prove manual and automatic hiding are separate, not merely that a column vanished.
- User-facing documentation must not imply that manually hidden steps become move targets.

## Results

- Added `auto-hide-empty-columns.spec.ts`; Chromium passed 1/1 and exercises the complete desktop
  acceptance path through the real settings UI and pointer drag surface.
- Added `mobile-auto-hide-empty-columns.spec.ts`; Mobile Chrome passed 1/1 and verifies workflow
  isolation, settled 44 CSS px geometry, recovered drop targets, cancellation cleanup, and no
  document-level overflow.
- Added a stable mobile drop-target test id and the auto-hide field to the typed E2E settings helper.
- Both runs used the production E2E build, real backend, isolated SQLite database, and no raw sleeps.
