---
id: "05-publish-auto-run-guidance"
title: "Publish Auto-run guidance"
status: done
wave: 5
depends_on: ["04-prove-auto-run-flows"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-run.md"
---

# Task 05: Publish Auto-run guidance

## Intent

Replace obsolete queue-control terminology with the proven behavior and mark
the durable feature package shipped.

## Acceptance

1. Public coordination and session guidance explains one-message-per-turn
   Auto-run, finish-current OFF, immediate Cancel, targeted Send Now resume,
   and lifecycle/clarification deferral without advertising header Run Next,
   bulk Send Now, or Skip to next.
2. `message-queue-run.md` and `docs/specs/INDEX.md` are marked `shipped`; Send
   Now, pin, reorder, and management specs remain consistent with the final
   surface and persisted policy.
3. The new ADR remains indexed and public-doc validation passes. Terminology
   search leaves old labels only in deliberate historical decision context or
   backward-compatible protocol discussion.

## Verification

```bash
rg -n 'Run next|Run queue|Skip to next|Auto-run|Send Now' docs/public/coordination.md docs/public/sessions-and-review.md docs/specs/ui/message-queue-*.md docs/decisions/2026-08-16-server-owned-queue-auto-run.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`
- `docs/specs/ui/requirements/message-queue-run.md`
- `docs/specs/ui/requirements/message-queue-send-now.md`
- `docs/specs/ui/requirements/message-queue-pin.md`
- `docs/specs/ui/requirements/message-queue-reorder.md`
- `docs/specs/ui/requirements/message-queue-management.md`
- `docs/specs/INDEX.md`
- `docs/decisions/2026-08-16-server-owned-queue-auto-run.md`
- `docs/decisions/INDEX.md`

## Dependencies

Task 04. Public claims and shipped status require passing browser evidence.

## Parallelism

Sequential. This task publishes behavior proven by Task 04.

## Inputs

- Completed Task 03 copy and Task 04 behavior evidence.
- Spec `What`, `State and Concurrency`, and `Scenarios`.
- `/docs-maintainer` content classification and validator rules.

## Risks

- Do not describe ON as bypassing clarification or workflow safeguards.
- Keep row Send Now distinct from removed bulk header Send Now.
- Keep protocol compatibility details out of ordinary how-to guidance unless
  the page already documents the API.
- Mark the draft shipped only after implementation and E2E evidence exist.

## Output contract

Report public pages and their content type, internal spec status changes, exact
validator results, terminology audit, files changed, and residual risks. Set
this task to `done`, record results below, and synchronize `plan.md`.

## Results

- Updated `docs/public/coordination.md` and
  `docs/public/sessions-and-review.md`. Both remain task-oriented how-to guides:
  they now explain one-turn-at-a-time Auto-run, finish-current OFF, immediate
  Cancel with a parked OFF backlog, targeted Send Now resume, and lifecycle or
  clarification deferral.
- Marked `docs/specs/ui/requirements/message-queue-run.md` and its index entry `shipped`.
  The accepted ADR remains indexed; adjacent queue specs retain row Send Now,
  pin, reorder, management, and backward-compatible protocol boundaries.
- Terminology audit found obsolete labels only in deliberate historical
  rationale or protocol-compatibility discussion. First-party public guidance
  contains no Run next, header bulk Send Now, or Skip to next instruction.
- `node --test scripts/validate-public-docs.test.mjs` passed 61/61 tests;
  `node scripts/validate-public-docs.mjs` validated 41 published pages;
  `git diff --check` passed.
