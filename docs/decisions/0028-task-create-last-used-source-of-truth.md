# 0028: Backend-Owned Task-Create Last-Used Preferences

**Status:** accepted
**Date:** 2026-06-29
**Area:** backend, frontend

## Context

The task-create dialog restores the user's last repository, branch, agent profile, and executor profile. Those values were partly kept in browser localStorage and partly in backend user settings, which allowed stale localStorage to win while backend settings were still loading. Auto-picked defaults also were not saved unless the user manually changed a selector.

## Decision

Backend user settings are the durable source of truth for `task_create_last_used`. Successful task creation records the final values used by the backend. Browser localStorage remains a cache and pending-write bridge, but dialog auto-selection must wait for backend settings to load before trusting cached values.

Task-create preference writes use targeted JSON updates under `users.settings.task_create_last_used`, and broad user-settings writes preserve the current task-create preference instead of rewriting it from a stale settings blob.

## Consequences

The next task-create dialog reflects the choices that actually created the previous task, including auto-picked defaults. Settings writes no longer erase newer task-create preferences when they race with task creation. The frontend may show an empty/loading selector state slightly longer while waiting for user settings, but avoids restoring stale localStorage values.

## Alternatives Considered

1. **Persist inside frontend auto-pick effects.** Rejected because auto-pick runs during data-loading and compatibility settling; saving there would make transient fallbacks durable.
2. **Keep localStorage as the primary source.** Rejected because it cannot be read by the backend, can be stale across database resets or workspace changes, and does not publish updates to other clients.
3. **Use the existing broad user-settings PATCH only.** Rejected because the read/merge/write path can clobber newer task-create values when unrelated settings writes overlap.
