---
id: "04-comparison-target-ui-and-docs"
title: "Present comparison targets on desktop and mobile"
status: done
wave: 4
depends_on: ["03-pr-reconciliation-and-lifecycle"]
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 04: Desktop/mobile presentation and public docs

Expose the authoritative target and fail-closed state in the existing responsive Changes and task-list
surfaces. Keep the current branch picker as the manual repository-local override.

## Red tests first

Add failing component/store tests for:

- rendering `upstream/widget:main` rather than a bare `main` when an explicit target is active;
- the same shared target content in the desktop branch hover and mobile touch drawer;
- an unavailable target showing one translated Changes error and a task-card unavailable indicator instead
  of numeric additions/deletions;
- target state scoped to the correct repository in multi-repo/multi-branch tasks;
- selecting attached-repository `main` clearing an active upstream `main` target rather than no-oping;
- successful selection invalidating commits and cumulative-diff caches;
- failed selection preserving the explicit target and showing the existing error toast; and
- long owner/repository/branch labels truncating without horizontal viewport overflow while retaining an
  accessible full label.

## Implementation

- Extend web HTTP/WS/status-summary types with optional comparison target/error fields. Add one typed parser
  for `TaskRepository.metadata.comparison_target`; do not scatter unchecked metadata casts.
- Thread comparison state through `useChangesPanelData`, branch rows, Changes header, and task-list summary.
- Reuse the existing desktop hover card and `useTouchDrawer` mobile composition. Add no new navigation or
  desktop-only action. Preserve one mobile scroll owner, existing touch targets, safe-area padding, and
  document containment.
- Update `BaseBranchPicker` so current-value equality includes repository identity. Its options remain attached
  repository branches; a selection calls the existing update route, which now atomically clears the provider
  target.
- Map stable backend error codes to concise recovery copy. Do not render raw Git/provider error strings.
- Add all new user-facing copy in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`; use the repository's Hant
  generation command for Traditional Chinese and declare genuine verbatim values only when required.
- Update these reference pages in the same implementation PR:
  - `docs/public/git-operations.md`: repository-qualified target, no origin/push rewrite, failure behavior;
  - `docs/public/sessions-and-review.md`: displayed target and manual override;
  - `docs/public/coordination.md`: `update_repository_base_branch_kandev` clears provider-derived comparison
    context but does not retarget the provider PR.

## Acceptance

- Desktop and mobile show the same target identity, failure meaning, and manual recovery action.
- The sidebar never presents known-wrong totals after explicit-target failure.
- All copy is translated and public reference docs match the implemented behavior.

## Likely files

- `apps/web/lib/types/http-workspace-sources.ts`
- `apps/web/lib/types/http*.ts`
- `apps/web/components/task/base-branch-picker.tsx`
- `apps/web/components/task/base-branch-picker.test.tsx`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/lib/ws/handlers/*` and focused tests as required by the event shape
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- `docs/public/git-operations.md`
- `docs/public/sessions-and-review.md`
- `docs/public/coordination.md`

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/base-branch-picker.test.tsx components/task/task-item.test.tsx
cd apps && pnpm --filter @kandev/web typecheck
cd apps/web && pnpm run i18n:zh-hant
cd apps/web && pnpm run i18n:check
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run `cd apps && pnpm install --frozen-lockfile` first in a fresh worktree.

## Parallelism

`sequential`. Consumes Task 03's final protocol and is the UI contract validated by Task 05.

## Output contract

Record red/green commands, desktop/mobile composition choice, translations, public-doc classification
(`git-operations`, `sessions-and-review`, and `coordination` are reference), and screenshots if useful. Update
task/plan status only after focused validation passes.

## Completion record

- Green: focused Changes/task tests (83 tests), web typecheck/lint, `i18n:check`, and public-doc validators.
- Desktop hover and the existing mobile touch drawer share the same translated comparison-target display;
  unavailable targets show bounded recovery copy and suppress task-card totals.
- Reference documentation was updated in `git-operations`, `sessions-and-review`, and `coordination`.
