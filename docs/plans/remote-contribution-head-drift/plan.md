---
spec: docs/specs/tasks/system-design/remote-contribution-tasks.md
created: 2026-08-10
status: approved
---

# Implementation Plan: Remote Contribution Head Drift

## Overview

Prevent a contribution branch update or force-push from making the Changes panel present two different
Git histories as one. The reported panel combined six commits from the task's original checkout with
fifteen commits from the PR's current provider history. Exact-SHA deduplication correctly found no
matches after the rewrite, but the fallback interpretation was wrong: every unmatched local SHA was
rendered as an unpushed commit with a green arrow. The rows were old contributor commits, not commits
authored by Kandev.

The fix preserves the checkout and classifies its relationship to the current provider history. Safe
aligned, local-ahead, and provider-ahead histories retain normal Push/Pull behavior. Diverged histories
show an inline warning and two separately titled commit lists. No observation automatically resets,
rebases, merges, or deletes local work.

This plan implements the amended
[Remote Contribution Tasks spec](../../specs/tasks/system-design/remote-contribution-tasks.md) and
[ADR-2026-08-10](../../decisions/2026-08-10-remote-contribution-head-drift.md).

## Implementation status

- [x] Task 01 — publish and retain complete upstream Git status evidence.
- [x] Task 02 — derive provider/local history relation and correct Push/Pull semantics.
- [x] Task 03 — render diverged histories and enforce the shared desktop/mobile action policy.
- [x] Task 04 — prove the force-push regression and responsive behavior end to end.

## Invariants

- Provider commits define the current PR contents; local Git defines the checkout contents.
- Different commit SHAs are not considered the same because their message, author, timestamps, file
  totals, or patches look similar.
- `ahead`/`behind` remain base-branch-relative. Push and Pull use upstream-relative evidence.
- Drift detection never mutates the checkout.
- Diverged history disables Push, force-push, and generic Pull on desktop and mobile.
- Provider loading or failure is not silently treated as an empty provider history or a confirmed
  rewrite.
- Ordinary repositories and tasks without a linked PR retain their current behavior.

## Data contract

### Git status evidence

Extend `GitStatusUpdate` with `remote_head_commit`, the commit resolved by
`@{upstream}^{commit}` during the same observation that computes `remote_ahead` and
`remote_behind`. Project `head_commit`, `base_commit`, `remote_head_commit`, and both remote counts
through lifecycle events, the WebSocket type, the handler, and `GitStatusEntry`. Carry the complete
upstream snapshot forward only when a transient secondary command fails and local HEAD is unchanged.

The existing meanings stay explicit:

- `ahead` / `behind`: checkout versus target base branch.
- `remote_ahead` / `remote_behind`: checkout versus its configured upstream.
- `head_commit`: checkout HEAD.
- `remote_head_commit`: locally observed upstream tip.

### Provider/local relation

Add a pure classifier with inputs for the selected linked PR, ordered provider commits and their
loading/error state, local HEAD, upstream HEAD, and upstream divergence counts. It returns one of:

- `not_applicable`: no selected linked PR.
- `aligned`: local and provider heads match.
- `local_ahead`: upstream head matches provider head and Git reports only local upstream-ahead commits.
- `provider_ahead`: the current provider commit list contains local HEAD.
- `diverged`: provider data is complete but neither safe ancestry relationship is proven.
- `unknown`: required provider or Git evidence is unavailable.

The classifier also returns explicit capabilities (`canPush`, `canPull`, `remoteMutationBlocked`) and
presentation mode (`unified` or `separate`). It does not compare commit patches or metadata. Keep it
provider-shaped so GitLab MR commit data can adopt the same classifier; the first Changes-panel wiring
uses the existing GitHub selected-PR commit source because that is the surface involved in this bug.

## Backend and protocol

- Add `RemoteHeadCommit` to `apps/backend/internal/agentctl/types/streams/git.go` and lifecycle Git
  status data.
- In `workspace_git_status.go`, resolve the sanitized upstream commit and counts as one secondary
  observation. Preserve the prior complete upstream snapshot on transient failure when local HEAD is
  unchanged; clear it when no upstream exists.
- Extend existing status tests for no upstream, aligned upstream, local ahead, remote ahead, divergence,
  and carry-forward. Extend lifecycle event projection tests to pin JSON field names.
- Do not fetch or mutate a remote as part of status polling. The provider comparison supplies current
  remote-change evidence without adding network Git work to the polling loop.

## Frontend state and behavior

- Retain the existing `head_commit`, `base_commit`, `remote_ahead`, and `remote_behind` fields that the
  WebSocket currently drops, and add `remote_head_commit`.
- Keep `SessionGit.ahead`/`behind` as base divergence for compatibility. Add clearly named upstream
  values such as `pushAhead` and `pullBehind`; `canPush` uses `pushAhead`, with the existing base-ahead
  fallback only when no upstream exists and a new change request still needs its first push.
- Update multi-repository summaries and fan-out gates to use each repository's upstream counts.
- Preserve the `loading` and `error` values from `usePRCommits`. A selected-PR relation hook combines
  those provider facts with the active repository's Git status and exposes one policy to the Changes
  panel, desktop VCS split button, and mobile Git menu.

## Changes panel

- Keep `mergeCommits` for `aligned`, `local_ahead`, `provider_ahead`, `unknown`, and non-PR flows. It
  continues exact-SHA deduplication.
- For `diverged`, bypass the flat merge. Render an inline warning followed by “Current PR commits” and
  “Local checkout commits”. Provider rows use provider details and no local mutation menu. Local rows
  retain local detail/recovery actions, but use a neutral local-checkout marker and accessible label;
  they are not described as unpushed solely because they are absent from the PR.
- The commit total and group headings must not imply that the union is the current PR history.
- Disable Changes-panel Push controls in diverged state and show the reason in accessible text.
- Surface provider commit loading failures without classifying the history as diverged.
- Add all copy to the `task` translation namespace in English, Portuguese, Simplified Chinese, and
  pseudo-locale files.

## Desktop and mobile controls

- Update `VcsSplitButton` to use upstream Push/Pull counts. Any configured upstream is valid; do not
  require `origin/<current-branch>`, because contribution tasks intentionally track a contribution
  remote.
- Keep base divergence pills and rebase/merge context base-relative.
- In diverged state, disable Push, force-push, and generic Pull with the same reason used by the Changes
  panel. Local Commit and explicit local recovery operations remain available.
- Apply the same relation and disabled state to `GitActionsDropdown` in the mobile top bar. The shared
  Changes panel warning remains inside its existing vertical scroll owner; do not add a nested drawer or
  horizontal scroller. The existing 44px mobile Git trigger remains unchanged.

## Testing strategy

Every implementation task follows red-green-refactor.

- Backend unit tests pin upstream head/count observation and carry-forward behavior.
- Frontend wire tests prove no status evidence is dropped.
- Pure classifier table tests cover aligned, local-ahead, provider-ahead, rewritten, loading, error,
  absent PR, and missing-upstream inputs.
- Component tests prove base counts never masquerade as Push counts, contribution remotes qualify as
  upstreams, diverged commit lists do not merge, and desktop/mobile remote actions are disabled.
- Desktop E2E recreates the bug with old local SHAs and a provider commit list containing rewritten
  SHAs, then asserts separate headings, the warning, accurate counts, and no green unpushed arrows.
- Mobile E2E opens the same task's Changes panel and Git menu at Pixel 5 dimensions, verifies the same
  warning/action policy, and checks viewport containment.

## Implementation waves

```text
Wave 1:
- [x] [Task 01: Upstream status contract](task-01-upstream-status-contract.md)

Wave 2:
- [x] [Task 02: Checkout relation and action semantics](task-02-checkout-relation-and-action-semantics.md)

Wave 3:
- [x] [Task 03: Diverged Changes UI and mobile parity](task-03-diverged-changes-ui-and-mobile-parity.md)

Wave 4:
- [x] [Task 04: Responsive force-push regression E2E](task-04-responsive-force-push-regression-e2e.md)
```

The tasks are sequential. Each consumes the contract established by the previous task, and Tasks 02–03
both touch shared Git UI state where parallel edits would conflict.

## Final verification

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web test
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:raw --project=chromium tests/git/git-changes-panel.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts
git diff --check
```

## Documentation impact

The behavioral spec and ADR are the durable documentation for this fix. No public CLI, configuration,
installation, executor, or API contract changes, so `docs/public/**` does not require an update.

## Open questions

None. A guided “reconcile checkout” action is intentionally out of scope; it requires a separate
product decision about reset, backup, rebase, and conflict handling.
