---
id: "01-expose-qualified-remote-refs"
title: "Reuse searchable branch picker"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.1
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.3
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.4
system_design:
  - ../../specs/integrations/system-design/watcher-remote-base-branches.md
---

# Task 01: Reuse Searchable Branch Picker

## Summary

Replace the watcher base-branch select with the shared searchable branch
picker used by New Task and repository branch-policy settings. Keep the
reusable selector and branch option projection in neutral component modules,
with task-create modules retaining only compatibility re-exports where needed.
Keep local `main` and supported remote `origin/main` distinct while preserving
default, loading, disabled, stored-value, and provider-backed behavior.

## In scope

- Add failing component coverage for qualified refs, search, badges, refresh,
  exact deduplication, and selector states.
- Reuse neutral `BranchSelector`, `sortBranches`, and `branchToOption` modules
  in `WatcherRepositoryFields` and repository branch-policy consumers.
- Keep the repository-default sentinel first and retain a missing stored value
  as a fallback option.
- Wire `useBranches.refresh` to the picker without changing selection.
- Apply scoped coarse-pointer sizing to the trigger and branch rows.

## Out of scope

- Backend changes.
- New user-facing copy or translations.
- Changing the repository picker or surrounding watcher dialog layout.
- Importing the compact New Task chip trigger into the watcher form.

## Acceptance

- Local and qualified remote refs with the same short name are separate values.
- Only exact duplicate projected refs collapse.
- Search, refresh, preferred ordering, and local or supported remote badges match
  the existing shared branch-picker behavior.
- Default, stored-value, and short-name fallback behavior remains unchanged.

## Verification

```bash
cd apps/web && pnpm test -- --run components/watcher-repository-fields.test.tsx components/settings/repository-branch-policy-fields.test.tsx components/task-create-dialog-repo-chips.test.tsx
```

## Files likely touched

- `apps/web/components/watcher-repository-fields.tsx`
- `apps/web/components/watcher-repository-fields.test.tsx`
- `apps/web/components/branch-selector.tsx`
- `apps/web/components/branch-picker-options.tsx`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/combobox.tsx`

## Dependencies

None.

## Risks

- The shared component serves four integrations, so a value-format regression
  would affect more than Jira.

## Parallelism

`sequential`

## Inputs

- Watcher remote base-branch requirement and system design.
- Existing New Task and branch-policy picker implementations as the established
  reusable pattern.

## Results

Reused the searchable `BranchSelector` option projection for watcher base
branches. The selector and projection now live in neutral modules so watcher
and repository settings consumers do not depend on task-create-owned files;
task-create compatibility re-exports preserve existing internal imports.
Qualified `origin` refs remain distinct from local refs, unsupported named
remotes are omitted because the backend launch contract supports `origin`,
exact projected duplicates collapse, stored values remain selectable when a
refresh does not return them, and refresh plus coarse-pointer sizing are wired
through the shared selector. The base-branch label is associated with the
combobox trigger, and stored fallback values use the standard local or remote
badge renderer.

The focused verification command passed 3 files and 26 tests. The expanded
branch-picker verification passed 5 files and 51 tests, including a module
boundary regression test.
