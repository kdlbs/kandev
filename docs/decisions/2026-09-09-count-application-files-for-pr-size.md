# ADR-2026-09-09-count-application-files-for-pr-size: Count Application Files for Pull Request Size

**Status:** accepted
**Date:** 2026-09-09
**Area:** workflow, infra

## Context

Maintainers want to review small pull requests before large pull requests. The repository needs one stable size measure for this review order.

Changed-line totals do not match the desired review groups. For example, pull request `#3538` changes more code lines than `#3533`.

Changed application-file totals match all seven confirmed examples:

| Group | Pull requests | Counted files |
| --- | --- | --- |
| Small | `#3526`, `#3524` | 2, 7 |
| Medium | `#3534`, `#3532`, `#3538` | 15, 17, 41 |
| Big | `#3533`, `#3536` | 80, 96 |

Documentation and repository tooling can add many files without increasing application-code review effort. Tests and translation catalogs do increase review effort.

## Decision

The pull request size is the number of changed application files. Additions, deletions, and changed-line totals do not affect this value.

The count includes application source, test source, and web translation catalogs under `apps/`. The count excludes these file classes:

- Documentation and agent guidance.
- GitHub workflows and repository scripts.
- Generated files and dependency files.
- Linter code, linter settings, and other tool settings.
- A file with a known image, font, or binary asset extension, or a file under one of these asset directories: `apps/backend/internal/notifications/providers/assets/`, `apps/desktop/src-tauri/icons/`, `apps/web/lib/assets/`, `apps/web/public/`, and `apps/web/src/assets/`.

The size groups use these fixed limits:

- `small`: 0 through 10 counted files.
- `medium`: 11 through 50 counted files.
- `big`: 51 or more counted files.

Each pull request has exactly one of these labels. The workflow recalculates the label when the pull request diff changes.

## Consequences

The labels show review breadth instead of changed-line volume. A large change in one file can remain `small`.

A pull request that changes only documentation or tooling receives the `small` label. Test-heavy changes can move a pull request to a larger group.

The workflow owns an explicit file filter. New application languages or new tooling directories can require a filter update.

## Alternatives Considered

### Count additions and deletions

This measure makes large test files dominate the result. It does not preserve the confirmed example groups.

### Count all changed files

This measure includes specifications, plans, workflows, and tooling. These files do not represent the requested application-code review scope.

### Use weighted file and line scores

This measure can model review effort in more detail. It adds policy complexity that the confirmed examples do not require.
