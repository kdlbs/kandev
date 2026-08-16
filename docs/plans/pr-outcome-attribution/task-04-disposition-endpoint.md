---
id: "04-disposition-endpoint"
title: "Disposition endpoint (withdrawn)"
status: withdrawn
wave: 3
depends_on: ["01-schema-and-activation"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 04: Disposition endpoint — WITHDRAWN 2026-08-15

This task was built and then cut from scope. The reviewer of PR #2614 decided a
Kandev-recorded closure reason does not belong in core; a plugin owns the reason
taxonomy, the replacement links, its own storage, and its own UI. The spec was
narrowed accordingly on 2026-08-15 (see its Amendment history, and AC-40 /
AC-41 / AC-42).

The code this task produced is removed by
[task-07-narrow-to-upstream-scope](task-07-narrow-to-upstream-scope.md). The
detailed design that used to live here is deliberately not retained: it
describes a contract that no longer exists, and leaving it in the plan invites a
future reader to rebuild it.

Nothing from this task survives into the narrowed feature.
