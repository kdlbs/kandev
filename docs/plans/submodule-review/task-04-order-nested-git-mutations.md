---
id: "04-order-nested-git-mutations"
title: "Order nested Git mutations"
status: done
wave: 2
depends_on: ["02-aggregate-submodule-git-data"]
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 04: Order nested Git mutations

## Acceptance

- File derivation and controls retain a real empty root scope alongside named children and route duplicate relative paths by explicit repository identity.
- Dependency-sensitive multi-scope operations execute deepest repository paths first, preserve sibling parallelism and partial results, and do not attempt an ancestor after its child fails.
- Commit-all stages and commits changed child scopes before parents so parent commits record new gitlinks; explicit single-scope and sibling-only operations remain unchanged.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-session-git-grouping.test.ts hooks/domains/session/use-session-git-repository-order.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/hooks/domains/session/use-session-git.ts`
- `apps/web/hooks/domains/session/use-session-git-grouping.test.ts`
- `apps/web/hooks/domains/session/use-session-git-repository-order.test.ts` (new)
- `apps/web/hooks/use-git-operations.ts` only if the existing single-scope primitive cannot express stage-plus-commit per wave

## Dependencies

Task 02.

## Parallelism

`parallel-safe` with Task 03 after Task 02: the owned operation/dispatch files are disjoint from Review presentation and locale files.

## Inputs

- Spec stage/discard/commit behavior and child-failure scenario.
- ADR deepest-first mutation invariant.
- Existing `fanOutAcrossRepos`, `aggregatePerRepoResults`, `pendingKey`, and per-repository stage/commit primitives.

## Output contract

Report wave ordering, failure propagation, files changed, exact tests and counts, blockers/risks, and synchronized task/plan status.

## Results

Implemented deepest-first repository-scope waves for stage, unstage, discard, and commit fan-out. Siblings remain independent, failed children block only their ancestors, and UI stage/commit controls disable while another operation is in flight to prevent overlapping mutations.

Verification:

- `use-session-git-repository-order.test.ts` and the surrounding focused Review/Git Vitest suite — 118 passed.
- `pnpm run typecheck` and `pnpm run lint` — passed.
