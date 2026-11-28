---
id: "01-host-folder-policy"
title: "Host folder policy and executor transitions"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-006
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-006.1
  - AC-TASKS-MIXED-REPOSITORIES-006.2
  - AC-TASKS-MIXED-REPOSITORIES-006.3
  - AC-TASKS-MIXED-REPOSITORIES-006.4
  - AC-TASKS-MIXED-REPOSITORIES-006.7
  - AC-TASKS-MIXED-REPOSITORIES-006.9
  - AC-TASKS-MIXED-REPOSITORIES-006.10
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 01: Host folder policy and executor transitions

## Summary and scope

Implement the shared source policy, accurate folder-menu status and visible folder-only adjustment across desktop and phone. Keep branch checkout semantics separate from host-folder eligibility.

- Coordinate folder add/removal and set expansion with draft executor/profile selection.
- Track transient automatic-switch provenance and clear it on manual choice; restore eligible Worktree when a repo returns.
- Preserve current source rows/branches on incompatible manual executor switches; enforce submit-handler and button gating.
- Update bottom hints for Local, Worktree, mixed and empty; keep compact typography and touch-visible reasons.
- Include editable subtask/preset/last-used compatibility without overriding locked context or fresh-dialog policy.
- Update public folder/executor instructions and all five locale catalogs.

## Out of scope

No new executor capabilities, remote host-folder copying, pushing local changes,
running-task redesign, commits, publishing, or delegation.

## Acceptance

- The assigned ACs pass through real creation callers, not only isolated helpers.
- The assigned previews match desktop and phone rendering, including recovery paths.
- Existing source identity, branch choices, locked context, and cleanup guarantees remain intact.

## ASCII UI preview

See the [complete preview](plan.md#ascii-ui-preview). The relevant views follow.

### UI-01: Worktree plus folder (desktop)

Entry: Worktree selected, open Add. Local Folder is enabled.

```text
[Repo: api | from: main v | x]   [+ Add v]
                                +----------------------+
                                | Repository         > |
                                | Local Folder       > |
                                | Repository Set     > |
                                +----------------------+

After choosing a folder:
[Repo: api | from: main v | x]
[Folder: ~/design-assets | x]    [+ Add]
[Prompt...]
[Agent v] [Workflow v] [Executor: Worktree v]
A worktree will be created from main.
The selected folder will be used directly.       [Start task]
```

Maps to 006.1-006.3, 006.9. Worktree stays selected because a repository exists.
Choosing Local through the existing executor selector changes the bottom helper
to direct checkout/folder use. No second executor selector is added.

### UI-02: Folder-only adjustment and explicit override

Entry: empty draft with Worktree selected; browse a folder and commit it.

```text
[Folder: ~/research | x] [+ Add]
Switched to Local for folder-only work.
[Prompt...]
[Agent v] [Workflow v] [Executor: Local v]
The selected folder will be used directly.       [Start task]
```

Maps to 006.1, 006.3, 006.7. Browse/cancel does not switch. Adding a repository
restores the previous Worktree choice only if the user has not since made an
explicit executor choice. Removing all items restores the longer Add label and
bottom-only scratch explanation from UI-03 of the predecessor plan. An unavailable
Local profile retains input and shows recovery; it does not claim a successful switch.

### UI-04: Retained incompatible selections

Entry: switch a mixed host workspace to SSH, or retain an unpushed branch.

```text
[Repo: api | Clone from remote | feature/local-only v | x]
This branch is not available remotely. [Choose remote branch]
[Folder: ~/design-assets | x]  [+ Add]
Local folders require a Local or Worktree executor.
[Executor: SSH v]                         [Start: disabled]
```

Maps to 006.6-006.8. Errors are per row. Switching back to a supported host executor
restores valid checkout behavior. Removing incompatible rows or selecting a remote
branch enables Start when all other validation passes. Origin changes show a
refresh-required row error; access failures show Retry/settings without losing the
selection. These are resolved error states, never indefinite executor waiting text.

### UI-05: Phone source navigation and recovery

Entry: phone New Task, then Add under the chosen executor.

```text
+--------------------------------------+
| [Repo: api                        x] |
| [Clone from remote | main v]         |
| [+ Add]                             |
| [Prompt...                        ] |
| [Agent v] [Workflow v] [SSH v]       |
| Clone remote contents only.         |
| Local uncommitted/unpushed work     |
| is not included.                    |
| [Start task]                        |
+--------------------------------------+

+--------------------------------------+
| Add to workspace                 x  |
| Repository                       >  |
| Local Folder             [disabled] |
| Requires Local or Worktree.         |
| Repository Set                   >  |
+--------------------------------------+

+--------------------------------------+
| < Back / Repository                 |
| Local | GitHub | ...                |
| [Search...                        ] |
|--------------------------------------|
| api                                |
| Clone from remote                  |
| github.com/acme/api                 |
|                                    |
| offline-project         [disabled] |
| No usable remote origin.           |
+--------------------------------------+
```

Maps to 006.9 plus 006.1-006.8. With Worktree, the same folder option is enabled
and navigates within this sheet to the folder picker. Reuse MobilePickerSheet:
inset bottom drawer, fixed navigation/search, one vertical results scroller,
dynamic viewport sizing and safe-area clearance. Back replaces the sheet body;
selection returns focus to Add. No stacked popovers. Recovery is visible beside
rows, not tooltip-only. At least 44px touch targets; no page horizontal overflow.
Desktop menus use the surrounding 12px text and normal compact controls. Copy in
these sketches is illustrative except approved labels; all copy uses five locales.

## Verification

Use TDD for changed logic. Commands run from the repository root; install workspace
dependencies once if missing. New test files in the plan are deliverables of this
work order. Record actual results; do not reuse predecessor counts.

```bash
(cd apps/web && pnpm exec vitest run components/task-create-dialog-executor-source-policy.test.ts components/task-create-dialog-executor-source-menu.test.tsx components/task-create-dialog-options.test.tsx components/task/use-subtask-submit.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- e2e/tests/task/create-task-executor-sources.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/task/mobile-create-task-executor-sources.spec.ts)
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog-computed.ts`
- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/task-create-dialog-mixed-repository-actions.ts`
- `apps/web/components/task-create-dialog-mixed-repository-chips.tsx`
- `apps/web/components/task-create-dialog-mobile-mixed-repository-surface.tsx`
- `apps/web/components/task-create-dialog-workspace-source-menu.tsx`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-repositories-state.ts`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/src/locales/`
- `docs/public/`

## Dependencies

None.

## Risks

Automatic transitions must not override a later manual choice or confuse Worktree with in-place checkout behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md), requirement 006.
- [Design](../../specs/tasks/system-design/mixed-repository-selection.md), executor-aware source policy.
- Existing workspace-contents creation and executor workspace-source tests.
- Plan test matrix and shared desktop/mobile previews.

## Results

Implemented the shared executor source policy, folder-only executor transition,
provenance restoration, submission blocking, desktop/mobile source menu wiring,
localized helper copy, and public folder/executor instructions.

Verification passed on 2026-09-15:

- Web task-create suite: 55 files and 706 tests.
- Web typecheck, lint, i18n checks, and production E2E build.
- Desktop workspace-content and mixed-repository flows: 3/3 and 2/2.
- Mobile workspace-content and mixed-repository flows: 1/1 and 1/1.
- Desktop and mobile mixed-subtask flows: 1/1 each.
- Public-doc validation and specification validation are included in the final gate.
