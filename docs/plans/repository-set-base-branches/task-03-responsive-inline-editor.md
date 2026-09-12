---
id: "03-responsive-inline-editor"
title: "Build the responsive inline editor"
status: done
wave: 3
depends_on:
  - "02-apply-saved-bases"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-002
  - REQ-WORKSPACES-REPOSITORY-SETS-003
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-002.2
  - AC-WORKSPACES-REPOSITORY-SETS-002.3
  - AC-WORKSPACES-REPOSITORY-SETS-002.4
  - AC-WORKSPACES-REPOSITORY-SETS-002.5
  - AC-WORKSPACES-REPOSITORY-SETS-002.6
  - AC-WORKSPACES-REPOSITORY-SETS-002.7
  - AC-WORKSPACES-REPOSITORY-SETS-002.8
  - AC-WORKSPACES-REPOSITORY-SETS-002.9
  - AC-WORKSPACES-REPOSITORY-SETS-003.9
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 03: Build The Responsive Inline Editor

## Summary

Build the approved selected-member editor with one base selector per row. Use a
desktop dialog and a full-height phone drawer with shared form state.

## In scope

- Show searchable and ordered selected-member rows.
- Add repositories through a searchable picker.
- Add per-row default and saved-base selectors.
- Reuse New Task branch search, refresh, option mapping, and origin badges.
- Gate branch loading on selector-open state.
- Keep unavailable saved values visible.
- Add Reset bases and responsive action regions.
- Add localized copy in every supported catalog.

## Out of scope

- Backend transport changes.
- Task payload construction.
- Playwright coverage.

## Acceptance

- Opening a large set starts no branch request until a selector opens.
- Branch rows match New Task names, search results, ordering, and origin badges.
- Desktop and phone surfaces expose the same member and base actions.
- The phone editor has one scroll owner, safe-area actions, and touch targets of
  at least 44 CSS pixels.

## Verification

Run these commands from `apps/web`.

```bash
pnpm exec vitest run app/settings/workspace/workspace-repository-set-editor.test.tsx app/settings/workspace/workspace-repository-sets-section.test.tsx components/task-create-dialog-selectors.test.tsx components/combobox.test.tsx components/task-create-dialog-repository-sets-save.test.tsx
pnpm run typecheck
pnpm run i18n:check
```

## Files likely touched

- `apps/web/app/settings/workspace/workspace-repository-set-editor.tsx`
- `apps/web/app/settings/workspace/workspace-repository-sets-section.tsx`
- `apps/web/app/settings/workspace/use-workspace-repository-sets.ts`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-branch-options.tsx`
- `apps/web/components/combobox.tsx`
- `apps/web/components/settings/repository-branch-policy-fields.tsx`
- `apps/web/src/locales/en/workspaces.json`
- `apps/web/src/locales/pt-pt/workspaces.json`
- `apps/web/src/locales/zh-cn/workspaces.json`
- `apps/web/src/locales/zh-hk/workspaces.json`
- `apps/web/src/locales/zh-tw/workspaces.json`
- Related component tests

## Dependencies

- Task 02 supplies the frontend member and task-draft shapes.

## Risks

- A hook per visible row can cause eager Git traffic.
- A settings-only option mapper can drift from New Task labels and filtering.
- Nested selectors can create more than one scroll owner on phones.
- Saved unavailable values need a stable fallback option.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-REPOSITORY-SETS-002`
- Mobile design contract in the repository-set system design
- New Task `Pill`, `branchOptionValue`, `branchToOption`, and `sortBranches`
- Repository branch-policy dialog and drawer patterns

## Results

- Scroll follow-up: the desktop wheel regression reproduced a branch list that
  stayed at scrollTop zero. The editor now supplies its outer surface as the
  shared picker portal container and allows popup overflow outside the form.
  The member hint is shorter and remains visible on desktop and mobile.
  Added desktop wheel and mobile touch regressions with 40 branches.
  Validation passed: 47 focused component tests, 2 desktop and 3 mobile E2E
  tests, TypeScript checking, changed-file ESLint, i18n checks, and spec lint.

- UX follow-up: use the shared 28px desktop control sizing and responsive
  touch targets for add, base, reset, and row actions. Keep the existing
  full-height phone drawer and shared state. Portal the repository picker out
  of the scrolling form and align it to the trigger's trailing edge.
  Pad the desktop footer and override the shared bottom drawer's 80vh cap so
  the phone editor retains its full-height contract.
  Verification: 25 focused component tests and both settings Playwright specs
  pass (2 desktop and 2 mobile tests). The browser checks cover 28px desktop
  controls, repository-option hit targets, phone touch sizes, and drawer height.
- Replaced the checkbox editor with ordered selected-member rows, searchable
  add and filter controls, move/remove actions, per-member base selectors, and
  Reset bases.
- Reused New Task branch options and loading behavior, including unavailable
  saved values and open-triggered branch requests.
- Added the bounded desktop dialog and full-height phone drawer with one scroll
  owner, safe-area footer, and touch-sized actions.
- Added complete localized copy across English, Portuguese, Simplified Chinese,
  Traditional Chinese, and the pseudo catalog.
- Verification: the work-order Vitest command passed 5 files and 46 tests;
  typecheck, web lint, i18n check, and i18n ratchet pass.
