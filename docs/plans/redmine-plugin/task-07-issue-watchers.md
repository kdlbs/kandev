---
id: "07-issue-watchers"
title: "Issue watchers"
status: not started
wave: 3
depends_on: ["03-connection-secrets-health", "04-projects-field-mapping", "05-issue-read-write-attachments"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 07: Issue watchers

## Intent

Implement structured-filter issue watches that create plugin-owned Kandev tasks for
newly matching issues, with dedup and a per-watch inflight-task throttle that actually
enforces (not just persists).

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: watch CRUD, poller loop, dedup and
throttle enforcement, `PluginOwnedTaskTrees` wiring for cleanup.

## Dependencies

Tasks 03, 04, 05.

## Acceptance

1. A watch with a structured filter creates one Kandev task per newly matching issue.
2. A second poll of an already-seen issue creates no second task, deduplicated by
   `(issue_watch_id, issue_id)`.
3. A watch with `maxInflightTasks: 1` refuses to create a second task while the first
   is still open — the throttle key the poller checks against is *exactly* the same
   constant the plugin writes into the created task's metadata (this is the precise
   bug Sentry's native integration originally shipped with: a mismatched or missing
   metadata key that made the persisted cap silently never apply).
4. Watcher-created tasks are created with `plugin:<id>` provenance so they are covered
   by `PluginOwnedTaskTrees`; deleting the watch or the connection cascades-deletes
   them via that host RPC rather than leaving orphaned tasks.
5. Removing or disabling a watch stops the poller for it without affecting other
   watches on the same connection.

## Verification

```sh
go test ./internal/watch/... -race -run TestThrottleCapEnforced
go test ./internal/watch/... -race
```

## Risks

The throttle-enforcement bug (see Acceptance #3) is the single highest-value thing to
get right in this task — write the enforcement test before the implementation, not
after, so a metadata-key mismatch fails loudly instead of shipping silently.
