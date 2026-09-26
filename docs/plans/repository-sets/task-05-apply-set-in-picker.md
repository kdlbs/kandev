---
id: "05-apply-set-in-picker"
title: "Apply a repository set in the picker"
status: done
wave: 5
depends_on: ["04-boot-and-web-data-layer"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 05: Apply A Repository Set In The Picker

## Acceptance

- A **Sets** control sits next to **add repository** in the shared repository row, listing the
  workspace's sets with their repository counts. It appears in the task-create dialog and the
  new-subtask form, is absent in Quick Chat, is absent when the workspace has no sets or when Remote
  URL / No repository mode is on, and is disabled with a visible reason (not a hover-only tooltip)
  when the chosen executor forbids multi-repository tasks.
- Choosing a set adds one row per member in set order with an empty branch, consumes a single blank
  placeholder row instead of leaving it, never discards or reorders rows the user configured, skips
  members already present, skips members missing from the live repository list while reporting how
  many were skipped and why, and produces no change when the same set is applied twice.
- On phone widths the set list is a bottom sheet sized for touch rather than a hover dropdown, and
  all copy exists in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw` with no em dashes.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run components/task-create-dialog-repository-sets.test.ts components/task-create-dialog-repo-chips.test.tsx components/task-create-dialog-repositories-state.test.ts
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task-create-dialog-repository-sets.ts` (new, pure `applyRepositorySet`) and
  its `.test.ts`
- `apps/web/components/task-create-dialog-repository-sets-control.tsx` (new) and its `.test.tsx`
- `apps/web/components/task-create-dialog-repo-chips.tsx` (`RepoChipsRow:65`, `ModeBody:171`)
- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx` (trailing area `:134-143`)
- `apps/web/components/task-create-dialog-repositories-state.ts` (taken-key loop, mirroring
  `addRemoteRepo` `:58-68`)
- `apps/web/components/task-create-dialog-prop-builders.ts`,
  `task-create-dialog-setup.ts`, `task-create-dialog-types.ts`
- `apps/web/components/task/new-subtask-form-parts.tsx` (`:240`, only if props must be threaded)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/`

## Dependencies

Task 04 for `useRepositorySets` and the store slice.

## Inputs

- Spec: Applying a set, Failure modes, Persistence guarantees (applying writes nothing).
- Place the control in `RepoChipsRow`, which both task-create and new-subtask render, while Quick
  Chat renders `WorkspaceRepoChips` directly - that placement alone gives the intended surfaces.
- `buildRepositoriesPayload` (`task-create-dialog-helpers.ts:279`) already handles N rows, so no
  submit or payload change is needed.
- Gate on `userSettingsLoaded`: `useRepositoryAutoSelectEffect`
  (`task-create-dialog-repository-autopick.ts:23`, `defer` branch `:94`) can otherwise overwrite an
  applied set.
- Run `/mobile-parity` for the phone pattern; reuse existing chip test ids
  (`repo-chip`, `repo-chip-trigger`, `add-repository`) and add stable ids for the new control.

## Risks

- Row-key collision: `useRepositoriesState`'s counter starts at zero (`:16`), so injecting rows
  through `setRepositories` can duplicate a later `addRepository()` key. Pin this in a test.
- `canReplaceEmptyRepositoryPlaceholder` (`repository-autopick.ts:148`) only replaces when exactly
  one blank row exists, so a two-member set survives, but a one-member set applied during autopick
  can be replaced. Cover both sizes.
- The dialog modules sit near their line caps; new logic goes in the new files.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
