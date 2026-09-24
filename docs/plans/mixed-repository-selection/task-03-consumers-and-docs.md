---
id: "03-consumers-and-docs"
title: "Shared consumers and documentation"
status: completed
wave: 3
depends_on: ['02-mixed-task-creation']
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-001
  - REQ-TASKS-MIXED-REPOSITORIES-003
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-001.2
  - AC-TASKS-MIXED-REPOSITORIES-001.4
  - AC-TASKS-MIXED-REPOSITORIES-003.1
  - AC-TASKS-MIXED-REPOSITORIES-003.2
  - AC-TASKS-MIXED-REPOSITORIES-003.3
  - AC-TASKS-MIXED-REPOSITORIES-003.4
  - AC-TASKS-MIXED-REPOSITORIES-003.5
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 03: Shared consumers and documentation

## Summary

Use the same mixed selection contract for New Subtask and repository Sets. Update public task instructions and reconcile the set specification after the obsolete modes disappear.

## In scope

- Migrate New Subtask state and submit hooks without changing inherited-source restrictions.
- Apply Sets additively to mixed or empty drafts and preserve saved bases, duplicate handling, and registered-only save behavior.
- Update desktop/phone Set E2E and add mixed subtask E2E.
- Update current set requirements/design to remove obsolete mode exclusions. Add this plan link without rewriting historical work-order results.
- Update tasks-and-workflows.md and relevant integration instructions. Search README and screenshot references for obsolete source modes.

## Out of scope

Arbitrary URL storage in Sets, workspace schema changes, Quick Chat redesign, and production provider releases.

## Acceptance

- Editable subtasks submit mixed rows and restore presets. Inherited or locked sources preserve current restrictions.
- Sets append without replacing remote rows or their bases. Save-as-set explains unregistered exclusions and keeps the existing storage contract.
- Desktop and phone tests prove these flows. Public docs explain eligible tabs, host-local paths, empty tasks, URL entry, and provider adoption requirements.

## ASCII UI preview

### UI-01: Desktop mixed draft and Add repository

Excerpt from the [full preview](plan.md#ascii-ui-preview).
Maps to AC-TASKS-MIXED-REPOSITORIES-003.1 and 003.2.

```text
[ local-repo | Local | develop v | x ]
[ plugin | GitHub | main v | x ]  [ + Add ] [ Sets v ]
                                               |
                                    +---------------------+
                                    | Platform (3 repos)  |
                                    | Save selection...   |
                                    +---------------------+
```

Applying Platform appends missing registered members. Existing chips stay put.
Save selection explains any unregistered-source exclusions before saving.

### UI-03: Phone repository management

```text
+----------------------------------+
| Repositories              Done   |
| local-repo   develop       [ x ] |
| plugin       main          [ x ] |
| [ + Add repository ] [ Sets v ] |
+----------------------------------+
```

New Subtask uses this same sheet when source editing is allowed. Sets opens
inside the shared mobile surface. Use one scrolling list and safe-area actions.
Maps to AC-TASKS-MIXED-REPOSITORIES-003.3 and 003.4. See the complete geometry
and navigation contract in UI-03 of the plan.


## Verification

Run from the repository root. New test files named here must be created in this work order.
Use TDD and preserve red/green evidence. Install workspace dependencies once as described in the plan.

```bash
(cd apps/web && pnpm exec vitest run components/task/use-subtask-submit.test.ts components/task-create-dialog-repository-sets.test.ts components/task-create-dialog-repository-sets-control.test.tsx components/task-create-dialog-repository-sets-save.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium -- tests/task/create-subtask-mixed-repositories.spec.ts tests/task/create-task-repository-sets.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/task/mobile-create-subtask-mixed-repositories.spec.ts tests/task/mobile-create-task-repository-sets.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
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

- `apps/web/components/task/new-subtask-form-state.ts`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/components/task-create-dialog-repository-sets.ts`
- `apps/web/components/task-create-dialog-repository-sets-apply.ts`
- `apps/web/components/task-create-dialog-repository-sets-save.tsx`
- `apps/web/components/task-create-dialog-repository-sets-control.tsx`
- `apps/web/e2e/tests/task/`
- `apps/web/src/locales/`
- `docs/specs/workspaces/requirements/repository-sets.md`
- `docs/specs/workspaces/system-design/repository-sets.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/integrations.md`

## Dependencies

Task 02.

## Risks

Set members are registered repository IDs. Do not convert save-as-set into a hidden registration step, and do not claim that arbitrary remote URLs are saved.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md).
- [System design](../../specs/tasks/system-design/mixed-repository-selection.md).
- [Plan and test mapping](plan.md).
- Scoped AGENTS.md, TDD, mobile-parity, and E2E guidance.

## Results

Completed on 2026-09-13.

- New Subtask reuses the mixed selection state and submission path while keeping
  inherited and locked source restrictions.
- Sets apply registered workspace repositories additively, preserve existing
  rows and saved bases, remain available for empty drafts, and explain excluded
  local or unregistered remote rows when saving.
- Desktop and phone E2E cover mixed subtasks, Sets, mobile navigation, saved
  branches, and last-row removal. The affected desktop suite passed 23/23 and
  the phone suite passed 35/35.
- Public task, integration, plugin, and repository-set documentation was updated
  and passed public-doc validation, specification validation, and `git diff
  --check`.
