---
id: "01-recover-provider-commit-evidence"
title: "Recover provider commit evidence"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 01: Recover Provider Commit Evidence

Share identical provider commit reads and retry one failed read. Keep the final error as internal
safety evidence.

## Inputs

- Spec sections: What, Failure modes, and Scenarios.
- Plan sections: Shared provider evidence resource and Tests.
- Decision: `docs/decisions/2026-08-13-provider-history-changes-enrichment.md`.
- Existing pattern: `usePRCommits` request ownership and source-key guards.

## Acceptance

1. Two concurrent consumers with the same evidence key send one WebSocket request.
2. The resource retries one failed read and publishes a successful second result.
3. Two failed attempts publish one final error and no provider commits.
4. A pull request or sync-version change prevents an older result from replacing the active result.
5. Manual refresh starts one shared fresh request.
6. Superseded cache entries do not accumulate without a bound.

## Files Likely Touched

- `apps/web/hooks/domains/github/use-pr-commits.ts`
- `apps/web/hooks/domains/github/use-pr-commits.test.ts`
- `apps/web/hooks/domains/github/pr-commits-resource.ts` (new, if extraction keeps the hook small)

## TDD Sequence

1. Add failing tests for shared in-flight reads and one retry.
2. Add failing tests for final failure, manual refresh, stale results, and bounded cleanup.
3. Implement the smallest shared resource that passes the tests.
4. Refactor request ownership without changing the hook contract.

## Verification

Fresh worktree bootstrap:

```bash
cd apps && pnpm install --frozen-lockfile
```

Focused tests:

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-pr-commits.test.ts
```

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes this task's evidence and refresh contract.

## Risks

- Do not reuse a success across different workspace, repository, pull request, or sync versions.
- Do not let a retry timer outlive its resource owner.
- Do not turn a cached failure into permanent state.

## Output Contract

Report the resource key, retry policy, eviction rule, files changed, and exact test results. Update this
task and `plan.md` in the same conversation.

## Results

- Added a module-level resource keyed by workspace, owner, repository, pull request, and provider sync
  version. Identical consumers share one in-flight request and successful evidence cache entry.
- A failed read retries once after a bounded delay. Two failures publish the final error internally;
  stale identities are ignored, superseded entries are evicted, and the cache is bounded at 24 entries.
- Manual refresh starts one shared fresh request for the evidence key.
- Focused command passed 10 tests:
  `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-pr-commits.test.ts`
