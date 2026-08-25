---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001
created: 2026-08-12
owners:
  - tbd
---
# GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Constraints

- Files SHALL stay at most 600 lines and functions at most 100 lines, both
  `warn` with `skipBlankLines` and `skipComments` (`apps/web/eslint.config.mjs`),
  under `eslint --max-warnings 0`. `task-item.tsx` is ~660 raw lines today and
  already carries one `max-lines-per-function` disable on `TaskItem`; the
  restructure SHALL NOT grow the file's warning count, and extracting the
  combined badge pair into its own small component is the expected shape.
- **`tasks-page-client.tsx` has the least headroom of the three, and it is the one
  file every builder must edit** (AC12 adds an import plus a hook call). Measured
  by stripping blank and comment-only lines, it sits at roughly **597 of the 600
  allowed** — slightly tighter than `task-item.tsx` at ~591, despite being the
  smaller file by raw count (642 vs 660). Treat that figure as approximate: it was
  derived by hand rather than from an `eslint` run, so the true count may fall
  either side of the limit. Two lines of margin is not a margin.
  The builder SHALL therefore check `max-lines` on this file explicitly, before
  assuming a two-line edit is free. If the edit does push it over, the remedy is an
  extraction out of `tasks-page-client.tsx` — see the fence below, which permits a
  sibling under `app/tasks/` for exactly this reason. Do not resolve it by adding an
  `eslint-disable`: the file carries none today, and silencing a size warning to
  land a two-line hook call trades a real signal for convenience.
- Production React changes are confined to `components/task/task-item.tsx`,
  `app/tasks/rich-task-list-row.tsx` and `app/tasks/tasks-page-client.tsx`,
  **plus at most one new sibling component file** — under `components/task/` for
  the badge-pair extraction, or under `app/tasks/` if `tasks-page-client.tsx`
  needs to shed lines per the bullet above. At most one new file in total, and the
  builder picks which of the two purposes it serves; if both pressures turn out to
  be real, that is a genuine second finding and routes back here rather than being
  absorbed by a second new file. All in-line shapes are permitted too and the
  choice is the builder's; a new file inherits every constraint the fence carries,
  in particular AC7 (it SHALL NOT import `useWorkspaceMRs`) and AC18.
  The fence exists to stop the change spreading into other surfaces, not to force
  an extraction into an already-oversized file.
  Test files, E2E specs and page objects are required edits outside that fence
  and are not restricted by it.
- The Playwright page object work SHALL extend `e2e/pages/app-sidebar-page.ts`
  with a row-and-badge accessor rather than adding a parallel sidebar page
  object; `sidebar-tasks-page.ts` already scopes `sidebar-task-item` and is the
  shape to follow.
- Commands that SHALL pass: `cd apps/web && pnpm run typecheck`,
  `pnpm --filter @kandev/web lint`, `cd apps/web && pnpm run i18n:check`,
  the vitest suites for the touched files, and the three new E2E specs.

## Related

- [gitlab-integration](../requirements/gitlab-integration.md) — the umbrella feature;
  its badge scope (Kanban card only) is what this card extends.
- [gitlab-mr-status-chip](../requirements/gitlab-mr-status-chip.md) — the sibling
  parity card; source of the hydration-ownership and colour-freeze exclusions
  this spec inherits.
- `apps/web/CLAUDE.md`, "GitHub PR status UI" — the single-derivation invariant
  the MR side mirrors.
