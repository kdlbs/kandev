---
created: 2026-09-01
status: complete
requirements:
  - REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001
system_design:
  - ../../specs/integrations/system-design/watcher-remote-base-branches.md
legacy_specs:
  - ../watcher-repository-binding/plan.md
---

# Implementation Plan: Watcher Remote Base Branches

## Overview

Reuse the searchable New Task branch-picker stack in the shared watcher field,
preserve the supported `origin` remote identity, then prove the qualified value
persists in Jira on desktop and phone. The backend already lists remote
branches, accepts `origin/<branch>`, persists the value, and materializes it
through the worktree refresh path, so implementation remains a frontend picker
and regression change.

## Scope

### In scope

- Show local and qualified `origin` refs as distinct watcher choices.
- Provide branch search, refresh, preferred ordering, and local or remote
  badges through the existing shared picker behavior.
- Preserve the selected qualified ref through watcher create, reload, and task
  repository projection.
- Cover the shared behavior with unit, desktop, and mobile evidence.

### Out of scope

- New API or database fields.
- A per-watcher fetch setting.
- Changes to repository refresh or fallback behavior.
- Redesigning the repository selector or the surrounding watcher dialogs.

## Technical approach

Replace the base-branch `PickSelect` in `WatcherRepositoryFields` with the
existing full-width `BranchSelector`. Keep the reusable selector and branch
option projection in neutral `components/branch-selector.tsx` and
`components/branch-picker-options.tsx` modules; task-create filenames may only
provide compatibility re-exports. Reuse `sortBranches`, `branchToOption`, and
`scoreBranch` from that shared branch-picker stack so search behavior, qualified
values, ordering, and badges stay consistent. Filter named remotes to the
supported `origin` contract before projection. Keep the repository-default
sentinel as a plain leading option, preserve a stored value with a fallback
option when it is absent from the current branch response, and deduplicate only
exact projected refs.

Wire `useBranches.refresh` to the selector's existing refresh action. Reuse the
combobox's bounded popover and add a narrowly scoped option-row class or prop so
coarse-pointer rows meet the mobile touch target without changing unrelated
comboboxes. Keep the repository selector on its current `Select`.

Add stable selectors or page-object methods only where the Jira Playwright flow
needs them. Extend the existing desktop Jira settings coverage and add a mobile
Jira watcher spec. Both flows use the current dialog and the reused searchable
branch picker; no surrounding dialog composition change is planned.

## Tests

- `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.1`, `.3`, and `.4`: focused
  component tests in `apps/web/components/watcher-repository-fields.test.tsx`
  prove local, qualified origin, provider-only, default, unsupported-remote, and
  duplicate refs; search filtering; badges; and refresh behavior.
- The existing branch-policy picker test remains the closest reuse regression
  and proves the same shared selector stack outside task creation.
- Existing backend tests continue to prove `origin/main` validation and remote
  worktree refresh behavior.

## E2E tests

- `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.2`: extend
  `apps/web/e2e/tests/integrations/jira-settings.spec.ts` to save and reload a
  Jira watcher bound to `origin/main`.
- `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.5`: add
  `apps/web/e2e/tests/integrations/mobile-jira-watcher-branches.spec.ts` for the
  same saved qualified-ref outcome through touch interaction, including dialog
  and popover containment, touch-sized rows, internal scrolling, and absence of
  document horizontal overflow.

## Work orders

- [x] [Task 01: Reuse searchable branch picker](task-01-expose-qualified-remote-refs.md)
- [x] [Task 02: Prove watcher branch selection](task-02-prove-watcher-branch-selection.md)
- [x] [Task 03: Document watcher branch selection](task-03-document-watcher-branch-selection.md)

## Verification results

Passed focused component verification (5 files, 51 tests), including the
shared branch-picker module-boundary regression, targeted TypeScript and ESLint
checks, rebuilt desktop Jira watcher E2E (1 passed), rebuilt mobile Jira
watcher E2E (1 passed), and public-document validation (61 tests and 41
published pages). The watcher projection now omits named remotes outside the
supported `origin` contract, associates the base-branch label with its
trigger, and cleans up temporary Jira watchers in all E2E outcomes. A
repository-wide E2E sleep audit still reports unrelated pre-existing
violations outside the changed files.

## Risks

- Provider-backed branch records may omit a remote name. The projection keeps
  their current short value instead of inventing `origin/`.
- The shared field affects Jira, Linear, Sentry, and GitLab watchers. Unit
  coverage must preserve default and local behavior for every consumer.
- `BranchSelector` is reusable, but its compact New Task `Pill` trigger is not
  appropriate for a labeled watcher form field. Reusing the option model and
  full-width selector avoids coupling watcher layout to task-chip geometry.
- The Jira E2E fixture must expose both the seeded local branch and its
  `origin/` tracking ref before the selector assertion.
