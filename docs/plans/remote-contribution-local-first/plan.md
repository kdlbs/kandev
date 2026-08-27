---
spec: docs/specs/tasks/system-design/remote-contribution-tasks.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Local-First Remote Contributions

## Overview

Keep a diverged task checkout as the user's working version. Replace the current blocking warning
with explicit version choices. Add exact-lease backend operations before the desktop and mobile UI can
offer remote replacement or local adoption.

The plan implements
[ADR-2026-08-12](../../decisions/2026-08-12-local-first-contribution-replacement.md).

## Invariants

- The task checkout is the working version. The provider branch is the published version.
- Drift detection does not change either version.
- Remote replacement targets one contribution repository and uses an exact provider-head lease.
- Adopting the provider version requires a clean working tree and creates a recovery branch first.
- Kandev exposes the managed replacement actions only through session-scoped user UI actions.
- Normal aligned, local-ahead, and provider-ahead behavior does not change.
- The first UI implementation covers GitHub PR drift. The backend operations stay provider-neutral.

## Backend

### Explicit contribution operations

Add `ReplaceRemoteContribution` and `UseRemoteContribution` to
`apps/backend/internal/agentctl/server/process/`. Keep them separate from `GitOperator.Push` so a
generic force request cannot become an implicit contribution replacement.

`ReplaceRemoteContribution` must:

1. Require a valid remote-contribution binding and a full expected commit ID.
2. Target the binding's contribution remote and exact head ref.
3. Invoke `git push` with
   `--force-with-lease=refs/heads/<head>:<expected-remote-head>`.
4. Return the normal Git operation result without retrying a lease mismatch.

`UseRemoteContribution` must:

1. Reject staged, unstaged, and untracked file changes.
2. Fetch the bound contribution head without changing the current branch.
3. Require the fetched commit ID to equal `expected_remote_head`.
4. Create a unique local recovery branch at the current task HEAD.
5. Reset the task branch to the fetched provider head and retain its upstream.
6. Return the recovery branch name in the Git operation result.

Use the existing classified Git subprocess path. Validate commit IDs and generated ref names before
they reach command arguments.

### Agentctl and runtime contracts

Add these internal agentctl endpoints:

- `POST /api/v1/git/contribution/replace`
- `POST /api/v1/git/contribution/use`

Each request carries `expected_remote_head` and the existing optional `repo` scope. Extend the runtime
agentctl client with typed methods and round-trip tests.

Add the user-facing WebSocket actions `worktree.replace_contribution` and
`worktree.use_contribution`. Each payload carries `session_id`, `expected_remote_head`, and one optional
repository scope. Register the actions only in `GitHandlers`. Do not add MCP tools or automatic call
sites.

The existing gateway session authorization remains the first request guard. A destructive action must
never fan out across repositories.

## Frontend

### Relation and action model

Update `remote-contribution-relation.ts` so `diverged` means a local-first presentation instead of a
blocked workflow. Preserve normal Push and Pull capability fields. Add explicit replacement and
provider-adoption capability fields that require complete provider evidence and an exact provider
head.

Replace `remoteMutationBlocked` and the `history_changed` disabled reason with an action model that
distinguishes:

- normal Push
- provider-ahead Pull
- diverged remote replacement
- diverged provider adoption
- unavailable evidence

Add typed callbacks in `use-git-operations.ts` for both new WebSocket actions. The callbacks must keep
an explicit empty repository scope and must not use the multi-repository fan-out wrapper.

### Shared copy and result handling

Add localized copy to the `task` namespace in English, Portuguese, Simplified Chinese, and pseudo.
Use these English concepts consistently:

- status title: **PR branch changed**
- status body: **Your task version is unchanged. When you are ready, choose which version to use.**
- primary destructive action: **Replace PR branch**
- local destructive action: **Use PR version**
- secondary disclosure: **PR #<number> version**

The replacement confirmation must identify the repository and provider head. It must state that the
task version replaces the published branch. The provider-adoption confirmation must state that Kandev
creates a recovery branch and requires a clean working tree.

After provider adoption, show the recovery branch name in success feedback. Map stable backend error
conditions to translated instructions. Lease mismatches must tell the user that the PR changed again.

### Desktop Changes and VCS surfaces

For a diverged relation:

- Show one yellow warning icon in the Changes toolbar and no repeated warning banner in the body.
- Keep the local task history as the primary commit list.
- Keep the provider history collapsed behind **PR #<number> version**.
- Keep local edit and commit actions enabled.
- Replace disabled Push and force-push controls with **Replace PR branch**.
- Add **Use PR version** as the secondary version choice.
- Keep generic Pull disabled.
- Show each desktop action explanation in the shared immediate tooltip component.

Changes relation and PR-file inputs must include only pull requests whose `repository_id` and
normalized `head_branch` match a live checkout status. A historical PR may remain in Review, but it
must not supply provider commits, files, links, or drift state to Changes.

Apply the same action model to `ChangesPanelHeader`, `VcsSplitButton`, and
`VcsMultiRepoMenu`. Scope every destructive action to the selected contribution repository.

### Mobile design contract

- **Desktop outcome:** The user can inspect both versions and select which version wins.
- **Mobile entry point:** The existing Git actions trigger in the task top bar.
- **Nearest exemplar:** `session-mobile-top-bar-git-controls.tsx` supplies the compact menu entry.
  `mobile-menu-sheet.tsx` supplies the inset, safe-area-aware bottom surface.
- **Hierarchy:** Keep Commit first. Show **Replace PR branch** as the primary drift action. Show
  **Use PR version** and **PR #<number> version** as secondary actions.
- **Presentation:** Use the existing responsive Git menu for entry. Use an inset bottom drawer for the
  destructive confirmation on phone. Keep the desktop confirmation as a dialog.
- **Rationale:** The confirmation is temporary and action-focused. It does not need a new route or a
  full-height surface.
- **Geometry:** Keep one internal scroll owner. Clear the bottom safe area. Give action rows and
  confirmation buttons a 44px touch target. Keep document horizontal overflow at zero.
- **Shared logic:** Reuse the relation, operation callbacks, confirmation state, and result handling.
  Keep only the dialog or drawer composition viewport-specific.
- **Mobile proof:** Replace the PR branch from the Pixel 5 Git menu and verify the exact remote result.

## Tests

- **Exact leased replacement:** Add a process test with a real bare source repository. Prove that the
  expected head succeeds and a moved head fails without ref mutation.
- **Provider adoption:** Add a process test that proves the recovery branch preserves old HEAD and the
  task branch moves to the expected provider head. Cover dirty worktrees and stale fetches.
- **Contract forwarding:** Extend agentctl API, runtime client, and WebSocket handler tests for the new
  action names, repository scope, expected head, response fields, and missing-field errors.
- **Relation policy:** Update the pure classifier tests for local-first divergence, unavailable evidence,
  provider-ahead Pull, and local-ahead Push.
- **Branch-scoped selection:** Prove that a merged historical PR cannot override the open PR matching
  the checked-out repository and branch, including when Review still selects the historical PR.
- **Frontend callbacks:** Prove both actions send one repository scope and the exact provider head.
- **Desktop components:** Prove that the compact state replaces the blocking warning. Prove that local
  actions remain available and destructive actions require confirmation.
- **Mobile components:** Prove that the Git menu exposes all three drift actions. Prove that the phone
  confirmation uses a drawer and keeps all touch actions enabled.
- **Localization:** Run the i18n catalog and changed-line ratchet checks.

## E2E Tests

### Replace a changed PR branch on desktop

- **Scenario:** A provider rewrites the PR after task creation. The user chooses the task version.
- **File:** `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- **What to verify:** The toolbar warning appears without a repeated body banner, provider history
  stays collapsed, confirmation names the effect, and the remote ref changes to local HEAD through an
  exact lease.

### Adopt the current PR version on desktop

- **Scenario:** A provider rewrites the PR after task creation. The user chooses the PR version.
- **File:** `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- **What to verify:** The task branch moves to the provider head, the recovery branch retains old HEAD,
  and the drift status clears after provider refresh.

### Replace a changed PR branch on mobile

- **Scenario:** A phone user selects **Replace PR branch** from the Git actions menu.
- **File:** `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`
- **What to verify:** The bottom drawer explains the replacement, the action succeeds, the remote ref
  changes, touch targets remain usable, and the document has no horizontal overflow.

Use real local Git refs for provider-head fixtures. Do not use synthetic commit IDs for leased actions.
Update the mock provider state after a successful push so the UI can converge to the aligned state.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Exact-lease contribution operations](task-01-exact-lease-contribution-operations.md)

Wave 2:

- [x] [Task 02: Local-first relation and shared actions](task-02-local-first-relation-and-shared-actions.md)

Wave 3 (parallel candidates after Task 02, user authorization required):

- [x] [Task 03: Desktop local-first contribution UI](task-03-desktop-local-first-contribution-ui.md)
- [x] [Task 04: Mobile contribution version choices](task-04-mobile-contribution-version-choices.md)

Wave 4:

- [x] [Task 05: Desktop and mobile drift E2E](task-05-desktop-mobile-drift-e2e.md)

Wave 5:

- [x] [Task 06: Branch-scoped drift UX correction](task-06-branch-scoped-drift-ux-correction.md)

The default execution order is sequential in the primary conversation. The wave labels do not
authorize subagents.

## Verification Results

Completed 2026-08-12. Each task records its exact commands and results.

- Backend focused verification passed 1,753 tests across the process, agentctl API, runtime client, and
  WebSocket handler packages.
- Frontend focused relation, operation, desktop, VCS, and mobile tests passed, including the shared
  26-test desktop/mobile action suite. `pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed.
- Desktop and Pixel 5 drift scenarios each passed once with retries disabled. Their provider histories
  use real local Git commits and verify the confirmation boundary and responsive interaction contract.
- Destructive ref mutation and provider adoption are covered by the real-Git process tests. The current
  REST E2E helper cannot create the server-owned contribution binding, so browser execution stops before
  mutation; the limitation is recorded in Task 05.
- The branch-scoped follow-up passed 66 focused Vitest assertions, full web lint and typecheck, i18n
  catalog and ratchet checks, all 21 desktop Changes E2E scenarios, and the Pixel 5 drift scenario with
  retries disabled. It proves that a merged historical PR does not drive the live checkout relation.

## Documentation Impact

The task behavior spec and the new ADR contain the user contract and safety boundary. This change does
not affect public CLI commands, configuration keys, installation, deployment, or executor setup.
`docs/public/**` does not require an update.

## Risks

- A bad lease implementation can overwrite provider commits that arrived after confirmation.
- A provider-adoption operation can lose uncommitted files unless the clean-tree guard includes staged,
  unstaged, and untracked files.
- If a desktop or mobile entry point keeps the old force-push path, generic Push controls can bypass the
  intended copy.
- Provider mocks can hide lease races unless E2E uses real remote refs.
