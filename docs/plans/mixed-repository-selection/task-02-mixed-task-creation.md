---
id: "02-mixed-task-creation"
title: "Mixed New Task selection"
status: completed
wave: 2
depends_on: ['01-provider-readiness']
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-001
  - REQ-TASKS-MIXED-REPOSITORIES-002
  - REQ-TASKS-MIXED-REPOSITORIES-003
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-001.1
  - AC-TASKS-MIXED-REPOSITORIES-001.2
  - AC-TASKS-MIXED-REPOSITORIES-001.3
  - AC-TASKS-MIXED-REPOSITORIES-001.4
  - AC-TASKS-MIXED-REPOSITORIES-001.5
  - AC-TASKS-MIXED-REPOSITORIES-002.1
  - AC-TASKS-MIXED-REPOSITORIES-002.2
  - AC-TASKS-MIXED-REPOSITORIES-002.3
  - AC-TASKS-MIXED-REPOSITORIES-002.4
  - AC-TASKS-MIXED-REPOSITORIES-002.5
  - AC-TASKS-MIXED-REPOSITORIES-002.6
  - AC-TASKS-MIXED-REPOSITORIES-002.7
  - AC-TASKS-MIXED-REPOSITORIES-003.3
  - AC-TASKS-MIXED-REPOSITORIES-003.4
  - AC-TASKS-MIXED-REPOSITORIES-003.5
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 02: Mixed New Task selection

## Summary

Deliver the mixed New Task path from source picker to backend attachments on desktop and phone. Remove the global source switch after the new state and serializer are wired.

## In scope

- Add the ordered union and reducer, stable keys, per-kind mapping, and untouched-only hydration.
- Build tabs above search and per-row source chips, keeping URL inspection, PR metadata, local creation, branch policies, and executor validation.
- Implement phone management, Add/Back/branch subviews, empty folder state, and failure recovery.
- Add mixed backend/transport regression tests and desktop/phone E2E. Migrate existing remote and plugin mode-switch tests and their helpers.
- Translate all new copy in five catalogs and generate Traditional Chinese variants.

## Out of scope

Changing executor capabilities, clone semantics, server trust, set persistence, or Quick Chat.

## Acceptance

- One task persists every mixed row in order with its independent base, checkout, and provider identity. Inspection failure preserves the draft and existing no-write guarantees.
- UI-01, UI-02, and UI-03 are functional, including eligible named tabs, empty input, public URL entry, touch geometry, and connection retry.
- Legacy presets and existing local/remote task-create tests pass through boundary adapters without source-mode controls or lost branch data.

## ASCII UI preview

### UI-01: Desktop mixed draft and Add repository

Entry: New Task, two selected repositories, Add repository open.

```text
[ kandev | Local  | from: origin/main v | x ]
[ plugin | GitHub | from: main v        | x ]  [ + Add ] [ Sets v ]

+---------------------------------------------------------+
| Local | GitHub | Bitbucket | Azure DevOps                |
|         ======                                          |
| Search GitHub repositories...                       [R] |
+---------------------------------------------------------+
| acme/plugin                                             |
| acme/design-system                                      |
| acme/api                                                |
+---------------------------------------------------------+
| Paste repository URL                                    |
+---------------------------------------------------------+
```

Tabs precede search. Names remain visible, including when the strip scrolls.
The header and URL action are fixed. Only results scroll vertically.
`R` illustrates the existing refresh icon. Providers shown are examples of
eligible integrations, not a fixed provider allowlist.
Maps to AC-TASKS-MIXED-REPOSITORIES-001.1 through 001.3 and 002.1 through 002.7.

### UI-02: Empty and failure states

Entry: New Task after removing the final repository.

```text
Repositories
No repositories attached.  [ + Add ] [ Sets v ]
Starting folder (optional): [ Choose folder ]
                                          [ Start task ]
```

The folder action appears only with its existing local-executor eligibility.
A removed last row is not restored by asynchronous defaults.

Entry: a selected remote provider loses its connection.

```text
[ plugin | GitHub | from: main v | x ]
Connection unavailable. [ Retry ] [ Integration settings ]
                                         [ Start: disabled ]
```

Its browse tab is absent. Local and other eligible tabs remain usable.
A list-only request failure instead keeps the eligible tab and shows Retry
inside its results. Initial status loading shows Local plus a loading notice.
An eligible empty source shows `No repositories found`, with its tab retained.
Maps to AC-TASKS-MIXED-REPOSITORIES-001.4, 001.5, 002.2, 002.3, 002.6, and 003.5.

### UI-03: Phone repository management

Entry: New Task, then tap Repositories (2).

```text
+----------------------------------+
| New task                         |
| [ Repositories (2) v ]           |
| Describe your task...            |
+----------------------------------+

+----------------------------------+
|              ----                |
| Repositories              Done   |
+----------------------------------+
| kandev     Local           [ x ] |
| from: origin/main v              |
|                                  |
| acme/plugin  GitHub        [ x ] |
| from: main v                     |
+----------------------------------+
| [ + Add repository ] [ Sets v ] |
+----------------------------------+
```

Add navigates inside the same drawer:

```text
+----------------------------------+
| < Back          Add repository   |
| Local | GitHub | Bitbucket | ... |
|         ======                   |
| Search GitHub repositories...    |
+----------------------------------+
| acme/plugin                      |
| acme/design-system               |
| acme/api                         |
+----------------------------------+
| Paste repository URL             |
+----------------------------------+
```

Use the MobilePickerSheet interaction: one drawer, fixed header, one vertical
scroll body, dynamic viewport height, and safe-area footer. Branch and URL views
replace the body and return through Back. Tab scrolling stays inside the strip.
Touch actions measure at least 44px. Desktop actions retain their normal 28px size.
Maps to AC-TASKS-MIXED-REPOSITORIES-003.3 and 003.4.

Tab position, shared selected state, source labels, navigation, and scroll
ownership are structural requirements. Example names, spacing, and icons are
illustrative. All product text must use the five locale catalogs.


## Verification

Run from the repository root. New test files named here must be created in this work order.
Use TDD and preserve red/green evidence. Install workspace dependencies once as described in the plan.

```bash
(cd apps/web && pnpm exec vitest run components/task-create-dialog-repository-selection.test.ts components/task-create-dialog-repository-picker.test.tsx components/task-create-dialog-helpers.multi-repo.test.ts components/task-create-dialog-submit.test.tsx components/task-create-dialog-repository-autopick.test.ts components/task-create-dialog-multi-repo-guard.test.ts components/task-create-dialog-remote-repo-provider-tabs.test.tsx)
(cd apps/backend && go test ./internal/task/service -run 'TestCreateTask.*(Mixed|Plugin|Repository)')
(cd apps/backend && go test ./internal/task/handlers -run 'Test.*(Mixed|Repository)')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium -- tests/task/create-task-mixed-repositories.spec.ts tests/task/create-task-remote-repo.spec.ts tests/task/plugin-repository-task-create.spec.ts tests/task/create-task-new-local-repository.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/task/mobile-create-task-mixed-repositories.spec.ts tests/task/mobile-create-task-remote-repo.spec.ts tests/task/mobile-plugin-repository-task-create.spec.ts tests/task/mobile-create-task-new-local-repository.spec.ts)
```

Run ESLint on the changed web source and test files. This exact command includes
tracked changes and new files. Record its result and the file list in Results.

```bash
python3 - <<'CHECK_LINT'
import subprocess
from pathlib import Path
changed = subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '-z']).split(b'\0')
new = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z']).split(b'\0')
paths = sorted({p.decode() for p in changed + new if p})
files = [p for p in paths if p.startswith('apps/web/') and p.endswith(('.ts', '.tsx')) and Path(p).is_file()]
print('\n'.join(files))
if files:
    subprocess.run(['pnpm', 'exec', 'eslint', *[p[len('apps/web/'):] for p in files]], cwd='apps/web', check=True)
CHECK_LINT
```
Do not use all-worker overrides. Run desktop and phone projects sequentially.

## Files likely touched

- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-repositories-state.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-repo-chips.tsx`
- `apps/web/components/task-create-dialog-source-mode.tsx`
- `apps/web/components/task-create-dialog-remote-repo-provider-tabs.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-repository-autopick.ts`
- `apps/web/components/task-create-dialog-multi-repo-guard.ts`
- `apps/backend/internal/task/service/repository_selection_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/task_ws_handlers_test.go`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/task/`

## Dependencies

Task 01.

## Risks

Legacy source flags are used by presets and hydration. Removing them before boundary normalization can silently drop input. Run any additional migrated suites discovered by the source-mode inventory.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md).
- [System design](../../specs/tasks/system-design/mixed-repository-selection.md).
- [Plan and test mapping](plan.md).
- Scoped AGENTS.md, TDD, mobile-parity, and E2E guidance.

## Results

Completed on 2026-09-13.

- New Task uses one ordered local/remote selection contract and serializes each
  row with its own source identity and branch choices.
- The task-wide Repo, Remote, and None switch is removed. Provider tabs remain
  source-scoped and eligible, with URL entry and list-failure recovery retained.
- Empty drafts, local folder selection, branch policies, executor limits,
  provider inspection failures, local repository creation, and five-locale copy
  remain covered.
- The shared mobile sheet supports selected-row management, Add/Back/Done
  navigation, branch views, Sets, focus return, and 44px touch targets.
- The affected unit suite passed 17 files and 163 tests. The desktop affected
  E2E suite passed 23/23 and the phone suite passed 35/35. Web lint, typecheck,
  i18n checks, and the E2E build passed.
- Changed backend task service and handler tests passed through
  `go test -tags fts5 ./internal/task/...`.
