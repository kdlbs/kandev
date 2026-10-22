---
id: "01-preserve-current-step"
title: "Preserve current step after a superseded cross-step move"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
acceptance_criteria:
  - AC-TASKS-KANBAN-TASK-REORDERING-001.13
  - AC-TASKS-KANBAN-TASK-REORDERING-001.28
---
# Task 01: Preserve current step

## Summary

Bound rollback to the pending move's own optimistic stage. Preserve accepted
current projections and backend-owned arrival position.

## In scope

Real-provider causal TDD, narrow production correction, browser entry-point
proof, internal contract clarification and recorded checks.

## Out of scope

See [plan scope](plan.md#scope). No delegation or publication.

## Acceptance

- Newer accepted task projections survive failed old moves, including equal
  targets and away/back; departed/replaced contexts are never recreated.
- Ordinary owned failure reverts only the stage; success/no-op, other tasks,
  request payload and captured error identity remain compatible.
- Journals and all owned fixtures/handles close on success and failure.

## ASCII UI preview

### UI-01: Cross-step move after a newer task event

Desktop entry: drag a card body onto another column. Phone entry: visible card
More options > Move to > destination, using the existing inset menu and focused
column navigator. Layout, scroll owners, safe areas and touch targets stay as
shipped (`MobileColumnTabs`, card menu and `SwipeableColumns`).

```text
Desktop: [Source] [Optimistic] [Authoritative: Remote title]
Phone:   [Workflow / Authoritative v]
         [Remote title] [More options]
         [Existing move error]
```

Before: rejecting the old request returns the card to Source.
After: the accepted authoritative projection remains, including when its stage
is Optimistic. Without a newer projection, failure returns the card to Source.
The structure is existing UI; spacing is illustrative. Covers AC .13 and .28.

Full context: [plan](plan.md#ascii-ui-preview).

## Verification

Run from repository root with Node 24 on PATH. Use the documented CI 8 GiB cap
for frontend checks if needed; do not broaden worker/shard limits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-swimlane-kanban-dnd.ts hooks/domains/kanban/use-cross-step-move.test.tsx hooks/domains/kanban/use-cross-step-move.test.ts hooks/domains/kanban/use-swimlane-kanban-dnd.test.ts e2e/helpers/cross-step-rollback.ts e2e/tests/kanban/cross-step-rollback.spec.ts e2e/tests/kanban/mobile-cross-step-rollback.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
NODE_OPTIONS=--max-old-space-size=8192 GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/kanban/cross-step-rollback.spec.ts -- --retries=0
NODE_OPTIONS=--max-old-space-size=8192 GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/kanban/mobile-cross-step-rollback.spec.ts -- --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/cross-step-move-rollback-ownership
git status --short -- docs/plans/cross-step-move-rollback-ownership
```

Also run the repository PR-documentation coverage preflight over actual changed
paths and a normal commit with active pre-commit/commit-msg hooks. Record complete
per-check outcomes, actual diff, clean HEAD and resource closure for the parent.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-swimlane-kanban-dnd.ts`
- `apps/web/hooks/domains/kanban/use-cross-step-move.test.tsx`
- Existing `use-cross-step-move.test.ts` and `use-swimlane-kanban-dnd.test.ts` fixtures
- `docs/public/tasks-and-workflows.md`
- `apps/web/e2e/helpers/cross-step-rollback.ts`
- `apps/web/e2e/tests/kanban/cross-step-rollback.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-cross-step-rollback.spec.ts`
- Existing owning Tasks requirement/design pair and this package.

## Dependencies

None. Preserved parent receipts remain outside the checkout.

## Risks

Existing bounded journal invalidation fails closed. Partial unrelated fields
survive an owned stage rollback. This is client projection repair.

## Parallelism

`sequential`

## Inputs

[Requirement](../../specs/tasks/requirements/kanban-task-reordering.md),
[design](../../specs/tasks/system-design/kanban-task-reordering.md), and
parent receipts under `loop-receipts/attempt26-source/`.

## Results

- Frozen workspace install: passed with Node 24.18.0.
- Immutable-base causal RED: 2 intended stage failures, 1 passing ordinary
  failure control, 17 unselected cases; no setup/import failures. The source was
  restored byte-for-byte in `finally` (SHA-256
  `103532b5a51228b6bed4778e0801bd148a754e55ffb6146e59ff9aa696ab817a`).
- Partial title/position-only event RED: 1 intended behavioral failure before
  narrowing ownership to stage writes. Final causal GREEN: 20/20, including
  equal-value events without timestamp advancement, away/back, independent
  moves, deletion, membership loss/reinsertion, workflow/workspace replacement,
  journal overflow, unmount, captured request/error callback and no-ops.
- Full `hooks/domains/kanban` Vitest directory: 42 files, 384 tests passed after
  the final production edit. Existing fake-store tests were upgraded to the real
  app store; their missing-capability failures were setup failures, not RED.
- Final desktop browser: 3/3 passed (different/equal authoritative target through
  real pointer drag, plus injected callback-failure fixture cleanup). Final phone
  browser: 2/2 passed through card-menu taps and the existing focused-column
  navigator. Error dismissal leaves the authoritative card in the viewport.
  Phone screenshots inspected; no document horizontal overflow observed.
- Managed host production build passed with Node's documented 8 GiB cap,
  `GOMAXPROCS=2`, `GOFLAGS=-p=2`, one worker, one shard and retries zero. Later
  browser runs reused that exact unchanged production source/build.
- Disposable fixture cleanup passed on success and injected callback failure:
  owned workspace, executor and files removed; worker seed, disposable source
  and task checkout HEAD/full raw porcelain status preserved. No shared reset,
  operator instance, provider traffic or credentials were used.
- Final typecheck (including rich-output catalog) and focused ESLint passed with
  no warnings. Full i18n check and new-code ratchet passed; no product copy or
  catalog keys changed.
- Public docs: 63 validator tests and all 54 published pages passed. Added a
  short recovery explanation to the existing Tasks and Workflows guide.
- Documentation catalog: 396 decisions and 1707 specifications validated.
  Full specification lint passed. Existing near-limit spec prose was shortened
  without changing its contracts. PR-documentation coverage preflight passed
  over all actual changed paths and the single linked work order.
- `git diff --check`, package diff and status checks passed. The final normal
  commit must have active pre-commit and commit-msg hooks; its exact receipt and
  complete base-to-head diff are retained in the external checkpoint packet.
- Refreshed 69 open PRs and actual paginated target patches: #1512 remains at
  `a8655ad92faa050a68140a904500fc99ebc0a04b` (9 pages, 895 files), with an
  unconditional original-snapshot rollback in its Query producer. #4013 and
  #3440 have no matching target files. This is a bounded duplicate assessment,
  not a correctness claim about those PRs.
- Original proof base remains `0424e7885814b0e4a7c8950e05f957fe6c6aa5a7`.
  Current main `8ba5b6909139ec2e9e14b0e2eb2f96a4e342e88b` adds the inspected
  disjoint Jira repair. No branch rewrite or combined-delivery claim was made.

Initial message-expectation, fake-store, fixture-cleanup helper and doc-size
failures were corrected and followed by the passing checks above. An initial
public-doc test invocation from `apps/` failed on its root-relative fixture;
the final documented root invocation passed. No deadline was used as evidence.

This proves managed Chromium and phone-emulated interactions. It does not claim
physical-device, OS-shell, operator-incident reproduction or zero-flake coverage.
No push or PR was attempted; parent independent validation and a separate
same-session delivery grant remain required.
