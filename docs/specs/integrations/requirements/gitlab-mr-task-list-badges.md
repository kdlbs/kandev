---
status: draft
system: integrations
created: 2026-08-12
owners:
  - tbd
---
# GitLab MR Badge on the Sidebar and Tasks-List Rows Requirements

## Overview

A task whose work lives on GitHub shows a pull-request badge on all three task-row surfaces: the Kanban card, the app sidebar's task list, and the `/tasks` page rows. A task whose work lives on GitLab shows a merge-request badge on exactly one of them, the Kanban card. On the other two a GitLab-only task looks like a task with no remote contribution at all.

## Requirements

### REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001: GitLab MR Badge on the Sidebar and Tasks-List Rows

**Intent:** A task whose work lives on GitHub shows a pull-request badge on all three task-row surfaces: the Kanban card, the app sidebar's task list, and the `/tasks` page rows. A task whose work lives on GitLab shows a merge-request badge on exactly one of them, the Kanban card. On the other two a GitLab-only task looks like a task with no remote contribution at all.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.1:** A task with at least one linked GitLab merge request SHALL render the existing `MRTaskIcon` badge on the app sidebar's task row and on the `/tasks` page's rich task row, in addition to the Kanban card it already renders on.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.2:** The badge SHALL be the existing `MRTaskIcon` component, rendered unmodified. No new glyph, no new colour rule, no new status derivation, no new `data-testid`, and no new user-facing copy are introduced by this feature.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.3:** On every surface that shows both, the pull-request badge SHALL precede the merge-request badge in DOM order.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.4:** The `/tasks` page SHALL become an owner of GitLab MR hydration for the active workspace, because it is the surface whose own row renders the badge. The sidebar SHALL NOT become one; see Hydration ownership.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.5:** No file under `apps/web/components/github/` changes. `apps/web/components/gitlab/mr-task-icon.tsx` does not change either. `PRTaskIcon`'s and `MRTaskIcon`'s rendered output SHALL be identical before and after this feature.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.6:** **No linked MRs.** `useTaskMRs` returns `EMPTY_MRS`, `MRTaskIcon` returns `null`, and the row renders no MR element. The MR badge SHALL contribute **no element of its own** to the row in this case: no wrapper `<span>`, no placeholder, and no *conditional* spacing that exists only to reserve room for the absent badge. This is the observable form of the requirement, and AC6 is its test. (An earlier draft said "byte-identical layout", which named no baseline and no mechanism; no snapshot baseline is introduced by this feature.)
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.7:** **No `taskId`.** The sidebar's `TaskItem` accepts `taskId?: string`. When it is absent the MR branch renders nothing and SHALL NOT call `MRTaskIcon` with an empty-string or `undefined` id.
- **AC-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001.8:** **No active workspace** (`workspaces.activeId === null`). `useTaskMRs` returns `EMPTY_MRS`; no badge, no error, no fetch.

## System design

The migrated technical source is split into [part 1](../system-design/gitlab-mr-task-list-badges-01.md), [part 2](../system-design/gitlab-mr-task-list-badges-02.md), [part 3](../system-design/gitlab-mr-task-list-badges-03.md).
