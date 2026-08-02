---
id: "04-frontend-picker-dialog"
title: "Frontend: multi-repo picker and dialog wiring"
status: pending
wave: 2
depends_on: ["03-frontend-types-form"]
plan: "plan.md"
spec: "../../specs/linear-watcher-multiple-repositories/spec.md"
---

# Task 04: Frontend — multi-repo picker and dialog wiring

## Acceptance

- New component `apps/web/components/watcher-repository-multi-fields.tsx` renders, per bound repository, a row with the repository label, a base-branch `Select` (branches fetched per row via `useBranches`, branch names deduped, `(repository default branch)` sentinel mapping to `""`), and a remove button; plus an "Add repository" control listing only repositories not already bound, which appends `{ repositoryId, baseBranch: "" }`. Empty selection renders just the add control with a "(no repository)" explanation.
- The Linear watcher dialog (`linear-issue-watch-dialog.tsx`) replaces the `WatcherRepositoryFields` block with the new component wired to `form.repositories`; the dialog description copy mentions one or more repositories; workspace switches clear the repository list.
- **Every user-facing literal in the new file and every changed dialog line goes through `t()`** (new `linear` namespace in `apps/web/src/locales/en/linear.json` + pseudo catalog synced via `pnpm run i18n:pseudo`).
- Component test covers: add appends; add-dropdown excludes already-bound repos; branch change updates only its row; remove deletes the row; empty state renders the add control.

## Verification

```bash
cd apps && pnpm --filter @kandev/web typecheck
cd apps && pnpm --filter @kandev/web test -- components/watcher-repository-multi-fields.test.tsx
cd apps/web && pnpm run i18n:ratchet && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/watcher-repository-multi-fields.tsx` (new; t()-localized)
- `apps/web/components/watcher-repository-multi-fields.test.tsx` (new)
- `apps/web/components/linear/linear-issue-watch-dialog.tsx` (AutomationFields block, DialogDescription, workspace-switch clearing)
- `apps/web/src/locales/en/linear.json`, `apps/web/src/locales/pseudo/linear.json` (new catalogs)

## Dependencies

Task 03 (form state). Consumed by task 05 (E2E drives this UI).

## Inputs

- Spec: `What`, scenarios 4–5, `Out of scope` (no table change).
- Plan: `Design > Frontend picker`, `Frontend dialog`.
- Existing patterns: `watcher-repository-fields.tsx` (hook usage + branch dedupe), `sentry-issue-watch-multiselect.tsx` `ProjectMultiSelect` (dropdown add pattern), `sentry-issue-watch-filter-fields.tsx` (field wrapper with description), `linear-issue-watch-fields.tsx` (chips/badges), i18n docs `docs/i18n.md` (TL;DR for adding a string).

## Risks

- Hooks in a loop: the per-row `useBranches` fetch must live in a stable sub-component (`RepoBindingRow`), not in a `.map` over bindings.
- Radix `SelectItem` cannot take `value=""` — the default-branch sentinel must map to `""` at the boundary (existing `DEFAULT_BRANCH` helpers).
- The i18n ratchet judges changed lines in `linear-issue-watch-dialog.tsx` too — any copy edited there must use `t()` with catalog keys.
- Keep `watcher-repository-fields.tsx` untouched (Jira/Sentry still use it).

## Output contract

Report component/dialog changes, the i18n keys added, and exact test/ratchet results; mark this task `done` and update its checkbox in `plan.md`.
