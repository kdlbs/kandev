---
id: "01-plus-menu"
title: "Move source attachment to +"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-005
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-005.3
system_design:
  - ../../specs/tasks/system-design/current-workspace-sources.md
---

# Task 01: Move source attachment to +

## Summary and scope

Move the existing action and opener ref into +, including the no-upload case. Preserve overflow Open workspace folder and existing creation/upload actions. Reuse the existing dialog and phone drawer; localize the menu label and visible reasons.

Acceptance: keyboard and touch open the form from + and return focus there; no duplicate attachment item remains under overflow; existing file and upload actions work.

Out of scope: root expansion, native-session replacement, active-turn admission, host-to-remote mounts/sync, and unrelated refactors.

## ASCII UI preview

Relevant views copied from the [combined preview](plan.md#ascii-ui-preview). Required order and capability semantics must match rendered UI; spacing is illustrative.

### UI-01: Files toolbar, desktop

Current source and supplied screenshot place attachment under overflow. Move it to +.

```text
Before: Files   /workspace/...       [+] [...] [Search]
                                          Add repositories
                                          Open workspace folder

After:  Files   /workspace/...       [+] [...] [Search]
                                    | New file
                                    | Upload files
                                    | Upload folder
                                    | ---------------------------
                                    | Add repositories or folders
                                         [...] Open workspace folder
```

Keep all existing creation/upload actions. If uploads are unsupported, omit those items, but + remains a menu when attachment is available.
Maps to AC-005.1/.2/.3 (all AC abbreviations in previews use the full REQ prefix in frontmatter).

### UI-04: Phone

```text
Files   /workspace/...   [+] [...]
                         | New file
                         | Upload files
                         | Upload folder
                         | Add repositories or folders

+----------------------------------+
| Add repositories or folders  [X] | fixed header
| Local / research                 |
|----------------------------------|
| [+ Repository v] [+ Folder]      | one scroll body
| [payments-api v]                 |
| Branch [main v]                  |
| [/home/me/reference] [Browse]    |
|                                  |
| (*) Current folder               |
|     Short paths; more entries.   |
| ( ) Inside ./kandev/             |
|     Grouped; longer paths.       |
|                                  |
| Result and unchanged CWD         |
| Folder links edit original files.|
| Access rules still apply.        |
|----------------------------------|
| [Cancel]          [Add sources]  | fixed safe-area footer
+----------------------------------+
```

Use the existing full-height source drawer for the form, not a compressed desktop dialog. Dynamic viewport height, vertical-only content, wrapping paths, visible disabled reasons, >=44px touch targets, and no page overflow are required. Return focus to + after close. Remote phones use UI-03 capabilities in this same composition.
Maps to AC-005.3 and AC-006.1/.5/.6/.7. The menu/dialog transition must not steal focus or close the new surface.

## Verification

Use TDD for changed behavior. Commands run from repository root; install workspace dependencies from apps if absent. E2E requires built assets and the existing guarded runner. Record actual results and environment blockers; do not mark unavailable provider checks passed.

```bash
(cd apps/web && pnpm exec vitest run components/task/file-browser-toolbar.test.tsx)
make build-web
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/add-workspace-sources.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/components/task/files-panel.tsx`
- `apps/web/components/task/task-files-panel.tsx`
- `apps/web/components/task/file-browser-toolbar.test.tsx`
- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`

Also update affected locale catalogs and focused tests beside changed code. Read scoped AGENTS.md before implementation. Use docs-maintainer for public documentation changes.

## Dependencies

None. Read the requirements, current-workspace design, and earlier placement package. Do not clear its expansion gate.

## Risks

See the package risks; verify ownership and effective root before filesystem changes. No automatic rebind is permitted for unchanged-CWD additions.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/tasks/requirements/attach-workspace-sources.md), [design](../../specs/tasks/system-design/current-workspace-sources.md), existing attachment tests, and the assigned previews.

## Results

Implemented. Source attachment is available from the Files `+` menu as **Add repositories or folders**. The overflow menu retains **Open workspace folder**, and the existing new-file and upload actions remain in the `+` menu.

Verification passed:

- Focused toolbar and placement tests passed, including 13 tests in the final toolbar and placement run.
- Desktop and mobile attachment E2E flows passed.
- Changed attachment E2E files passed ESLint and the source localization checks passed.
