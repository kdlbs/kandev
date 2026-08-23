---
id: "02-checkout-relation-and-action-semantics"
title: "Classify checkout drift and Git actions"
status: done
wave: 2
depends_on: ["01-upstream-status-contract"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 02: Classify Checkout Drift and Git Actions

Create one tested relation model for the selected PR, local checkout, and upstream, then replace
base-ahead Push semantics with upstream-relative semantics.

## Context

- `usePRCommits` already returns ordered provider commits plus `loading` and `error`, but
  `useChangesPanelPRData` discards the latter two states.
- `useSessionGit` exposes only base-relative `ahead`/`behind`, sets `canPush = ahead > 0`, and gives the
  same values to multi-repository Push controls.
- `VcsSplitButton` also requires an `origin/<current-branch>` upstream, which excludes the dedicated
  contribution remote by design.
- Task 01 makes local/upstream SHA and count evidence available in the frontend store.

## TDD sequence

1. Add a table-driven pure-function test covering `not_applicable`, `aligned`, `local_ahead`,
   `provider_ahead`, `diverged`, and `unknown`.
2. Include negative cases: same message/different SHA does not match; loading/error does not mean an
   empty or rewritten PR; an upstream head that differs from provider head does not prove local-ahead;
   base `ahead` alone never proves a Push is available when an upstream exists.
3. Add session summary/fan-out tests for upstream Push/Pull counts and no-upstream fallback.
4. Implement the classifier, selected-PR relation hook, and SessionGit semantics.

## Relation rules

Use full commit identity only:

- No selected linked PR: `not_applicable`.
- Provider request loading, failed, empty unexpectedly, or required Git SHA absent: `unknown`.
- `localHead === providerHead`: `aligned`.
- Provider commit list contains `localHead`: `provider_ahead`.
- `upstreamHead === providerHead && remoteAhead > 0 && remoteBehind === 0`: `local_ahead`.
- Complete provider data with none of the above: `diverged`.

For a provider-ahead relation, Pull is allowed and Push is not. For local-ahead, Push is allowed for
`remoteAhead` commits. For diverged, both remote mutations are blocked. For unknown/non-PR flows, use
upstream counts when an upstream exists and retain the existing first-push fallback when it does not.

## Implementation

- Add a pure module beside session Git domain code with explicit input/output types; do not couple it
  to React or rendered commit types.
- Add a selected-PR hook that combines `useReviewPRSelection`, `usePRCommits`, repository identity, and
  the active repository's Git status. Preserve provider `loading` and `error` in its result.
- Extend `SessionGit` with `headCommit`, `remoteHeadCommit`, `remoteAhead`, `remoteBehind`, `pushAhead`,
  and `pullBehind`. Keep `ahead`/`behind` base-relative.
- Update `deriveMultiRepoSummary` and `useRemoteOpsFanOut` gates so per-repository Push uses upstream
  evidence. A repository with a configured upstream must never use base `ahead` as its Push count.
- Keep change-request creation behavior for a branch without an upstream: base commits can still make
  Create PR the primary action and the initial push can set an upstream.
- Feed the relation result into Changes-panel data. Task 03 owns rendering and control copy.

## Acceptance

- The six old SHAs plus fifteen rewritten provider SHAs classify as `diverged`, not twenty-one
  unpushed/pushed commits.
- Patch-equivalent commits with different SHAs remain distinct.
- A normal maintainer commit on top of the current provider head classifies as `local_ahead` and Push
  reports one upstream-relative commit even if the branch is many commits ahead of `main`.
- A provider fast-forward containing local HEAD classifies as `provider_ahead` and offers Pull.
- Loading and provider errors remain observable and never claim a rewrite.

## Files likely touched

- `apps/web/hooks/domains/session/remote-contribution-relation.ts` (new)
- `apps/web/hooks/domains/session/remote-contribution-relation.test.ts` (new)
- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts` (new)
- `apps/web/hooks/domains/github/use-pr-commits.ts`
- `apps/web/hooks/domains/session/use-session-git.ts`
- `apps/web/hooks/domains/session/use-session-git-summary.ts`
- existing SessionGit and summary tests
- `apps/web/components/task/changes-panel-data.tsx`

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-session-git-summary.test.ts hooks/domains/session/use-session.test.ts
cd apps/web && pnpm run typecheck
```

## Dependencies and parallelism

Depends on Task 01. Run sequentially; Task 03 consumes the classifier and action policy.

## Output contract

Record classifier truth-table evidence, upstream/no-upstream action results, red/green commands, and
any compatibility choices. Update this task and the plan checkbox when complete.

## Completion evidence

- The pure classifier covers not-applicable, aligned, local-ahead, provider-ahead, diverged, and
  unknown states, including rewritten SHAs, loading/error state, missing Git evidence, and upstream
  versus base-count fallbacks.
- `SessionGit` now carries upstream-relative Push/Pull counts and observed heads without changing the
  existing base-relative divergence fields. Fan-out Push gating uses the upstream count when one is
  configured.
- Focused relation, summary, and session Git tests passed. Web typecheck and lint passed.
