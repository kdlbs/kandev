---
created: 2026-09-03
status: completed
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
legacy_specs: []
---

# Implementation Plan: Commit Provenance Refresh Stability

## Overview

Keep confirmed pull-request commit provenance stable while the provider commit resource refreshes for
the same contribution. Implement the correction in one vertical work order because the cache contract,
safe relation classification, Changes projection, and regression evidence are one coupled result.

## Root cause

`usePRCommits` keys provider evidence by workspace, repository, pull request, and
`last_synced_at`. When that version changes, `pr-commits-resource.ts` creates a pending entry whose
commit list is empty and prunes the previous version. `useRemoteContributionRelation` passes that empty
list to the Changes projection. `mergeCommits` can then recognize only local Git history, so commits
whose provider membership was already confirmed temporarily render with the emerald unpushed marker.
The accepted provider response restores the SHA matches and the neutral pushed marker.

Task switching exposes this transition because task restoration resubscribes Changes consumers and can
refresh the task pull-request snapshot. The supplied trace shows repeated `session.git.commits` reads.
The later Git status update reports `changed=false`. Thus, provider-enrichment state causes the marker
transition. A checkout change does not cause it.

## Scope

### In scope

- Retain the most recent successful provider commit list across evidence-version changes for the same
  workspace, repository, and pull request.
- Keep retained commits display-only while the current version is pending or failed.
- After a successful current-version response, replace the retained provenance.
- Preserve identity isolation, stale-result rejection, retry behavior, deduplication, and bounded cache
  eviction.
- Add a fail-before-fix hook regression and task-switch browser evidence.

### Out of scope

- Persist provider snapshots across page reloads or application restarts.
- Change local Git ahead/behind computation, push routing, commit marker styling, or copy.
- Change Changes-panel layout, touch behavior, scrolling, navigation, or mobile composition.
- Reduce the duplicate Git-status subscriptions visible in the supplied trace. They are diagnostic
  noise and do not cause the incorrect marker state.
- Treat retained provider commits as current evidence for Push, Pull, or contribution replacement.

## Technical approach

### Provider commit resource

`PRCommitsState` and `apps/web/hooks/domains/github/pr-commits-resource.ts` separate the current
authoritative commit snapshot from the last successful display snapshot. When a new `sourceKey` version
starts, use the existing `identityFor` key to find retained display evidence. This key contains the
`workspaceId`, owner, repository, and pull request number. Do not retain across a different identity.

Pending and failed current versions expose retained commits only through the display projection. Their
authoritative commits, provider head, and completeness remain empty or false, with the current loading
or error state intact. Successful current evidence atomically replaces both projections. Preserve the
existing source-key generation guard and prune superseded entries only after their display evidence has
been transferred or intentionally discarded.

### Relation and Changes projections

`apps/web/hooks/domains/github/use-pr-commits.ts` and
`apps/web/hooks/domains/session/use-remote-contribution-relation.ts` expose both projections.
`classifyRemoteContribution` receives only authoritative current evidence.
`useChangesPanelPRData` in `apps/web/components/task/changes-panel-data.tsx` receives retained display
commits. This explicit split prevents a stale commit list from enabling a remote action.

The existing `mergeCommits` and `CommitStatusMarker` behavior remains unchanged. Matching display SHAs
continue to show as pushed. If an accepted refresh removes a SHA, its presentation changes with that
snapshot.

### Desktop and mobile contract

Desktop and phone Changes surfaces already share `useChangesPanelData`, `mergeCommits`, and
`CommitRow`. This correction changes only their shared state projection. The nearest shipped mobile
surface is `apps/web/components/task/task-layout.tsx`. Existing Changes provenance coverage is in
`apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`. There is no new entry point, hierarchy,
overlay, scroll owner, safe-area rule, touch target, or viewport-dependent interaction. Focused
hook/component coverage plus the existing shared-path browser scenario satisfies mobile parity. A
mobile-specific Playwright case is not required unless implementation changes responsive composition.

## Tests

- `apps/web/hooks/domains/github/use-pr-commits.test.ts`: add
  `retains resolved display commits while a same-PR sync-version refresh is pending`. This is the
  required fail-before-fix regression: current code returns an empty commit list after rerendering with
  a new refresh key.
- In the same file, prove a failed same-identity refresh retains display commits, a successful refresh
  replaces them, and a different pull request never receives them.
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx`: prove that retained display
  commits reach Changes while current evidence classifies as `unknown`. Prove that remote actions remain
  disabled.
- `apps/web/components/task/changes-panel-remote.test.ts`: prove that retained display evidence keeps a
  matching local commit pushed. It becomes unpushed only after an accepted current list omits it.

## E2E tests

- Extend `apps/web/e2e/tests/pr/pr-switcher-changes.spec.ts` for
  `AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.7`. Seed a provider commit whose SHA matches checkout history.
  Then switch to another task and create a new evidence version. Force both provider attempts to fail
  before the switch back. Assert that the retained commit row still has pushed provenance. The hook
  regression holds a request unresolved and covers the pending interval without an arbitrary timeout.

## Work orders

- [x] [Task 01: Stabilize commit provenance across refreshes](task-01-stabilize-commit-provenance.md)

## Verification results

- The provider commit resource retains the last successful display list for the selected contribution
  identity. A separate authoritative list stays empty during a pending or failed refresh.
- The relation classifier uses only authoritative current evidence. Therefore, retained display data
  cannot enable Push, Pull, or contribution replacement.
- Focused tests pass with 30 assertions across the resource, relation hook, and Changes projection.
- The production-build Chromium task-switch scenario passes with a forced failed refresh.
- TypeScript typecheck, frontend lint, specification lint, and `git diff --check` pass.

## Risks

- Retaining by source key instead of stable contribution identity can preserve the flicker or leak
  another pull request's history.
- Passing retained commits into relation classification can incorrectly enable a remote mutation after
  the provider head changes.
- Pruning the prior version before transferring its successful display snapshot can reintroduce the
  empty interval.
- A final-state browser test alone can miss a short pending transition. The hook regression holds the
  request unresolved, and the browser scenario covers retention after both refresh attempts fail.
