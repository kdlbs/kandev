---
id: "06-task-linking-bidirectional-sync"
title: "Task linking and bidirectional sync"
status: completed
wave: 3
depends_on: ["03-connection-secrets-health", "04-projects-field-mapping", "05-issue-read-write-attachments"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 06: Task linking and bidirectional sync

## Intent

Wire task linking through the shared host Link dialog (`registerTaskAction`,
`host.openTaskLinkDialog`), then implement the inbound cursor-based sync loop and the
opt-in outbound write-back with echo suppression.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: link action registration, sync
loop, write-back logic, echo-suppression state.

## Dependencies

Tasks 03, 04, 05.

## Acceptance

1. Linking a task to a Redmine issue goes through `host.openTaskLinkDialog`, not a
   plugin-drawn modal; the task carries issue id/url afterward via `Tasks().Update`.
2. The sync poll sends `updated_on=><cursor>` and `status_id=*`; the cursor advances to
   the newest observed `updated_on` minus a 1-second overlap; restarting the plugin
   resumes from the persisted cursor, not from zero.
3. A linked issue's status change to a mapped, different workflow step moves the
   Kandev task on the next poll, verified specifically for a **closed** status.
4. With `syncTitleDescription` enabled, a Redmine-side subject/description change
   updates the Kandev task's title/description on the next poll; with it disabled,
   neither changes.
5. With `autoStatusWriteback` enabled, moving a linked task to a mapped workflow step
   issues `PUT /issues/:id.json` with the mapped status within one event-loop turn;
   with it disabled, no PUT is issued and a manual "Set Redmine status" action still
   works.
6. Echo suppression: a write-back PUT followed by the next inbound poll does not
   re-apply the change or bounce the task back — zero additional workflow transitions
   after one round trip.
7. Unlinking removes the link and stops syncing that task.

## Verification

```sh
go test ./internal/sync/... ./internal/tasklink/... -race
```

## Risks

Echo storms (R1 from the original native plan) and cursor gaps at second-granularity
boundaries (R2) carry over unchanged to the plugin — the mitigations (recorded
`last_pushed_*` fields, 1-second cursor overlap) are the same design, just
plugin-owned state instead of a SQL table.
