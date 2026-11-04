# ADR-2026-11-03-clarification-answer-draft-ownership: Clarification answer draft ownership

**Status:** accepted
**Date:** 2026-11-03
**Area:** frontend

## Context

[Issue #5013](https://github.com/kdlbs/kandev/issues/5013) reports lost question
answers after task navigation. `ClarificationDraftProvider` owns its cache
inside `TaskLayout` and clears that cache when the task ID changes. Route
unmounts also discard it. Responsive remount retention cannot preserve answers
across either boundary.

Answer drafts contain user input that the user has not submitted. Navigation
retention must not create durable response state or cross an account boundary.
The task system owns the clarification lifecycle and its answer identity.

## Decision

The browser application store owns a bounded in-memory cache for active
clarification answer drafts. Cache identity includes task, source session,
pending bundle, and form mode. Task-page providers project this cache into
the existing form. They retain local ownership of panel sizing.

One store-lifetime observer clears drafts on auth mode, authentication, user,
role, or status changes. It replaces the cache generation even when no task
page is mounted. Old callbacks cannot populate the replacement generation.

The cache retains up to eight recently accessed or edited bundles. It never
persists to browser storage or sends an answer. Current clarification authority
and the existing response path decide whether a restored answer can be submitted.

## Consequences

Task switching and task-page unmounts preserve unfinished answers within the
same application instance. Store isolation and auth invalidation also apply
to pages that do not currently render a question form.

Reload and logout discard drafts. A ninth retained bundle evicts the least
recently used draft. Cross-tab, cross-device, and durable draft recovery require
a separate product contract. This decision does not change historical
late-answer admission or other question hosts.

The [requirements](../specs/tasks/requirements/clarification-response-reliability.md)
define `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002`. The
[system design](../specs/tasks/system-design/clarification-response-reliability.md#unsubmitted-drafts-across-navigation)
defines cache integration. The
[implementation plan](../plans/clarification-answer-drafts/plan.md) records delivery.

## Alternatives Considered

- **Keep the cache in the task layout:** Responsive remounts retain drafts,
  but task changes and route unmounts still discard them.
- **Keep task layouts mounted:** This couples answer retention to heavyweight
  workbench lifetime and does not cover leaving the task route.
- **Use an unscoped module map:** This survives navigation but lacks application
  ownership and auth invalidation while forms are absent.
- **Persist drafts in browser or backend storage:** This adds retention,
  authorization, synchronization, and reload behavior beyond the reported issue.
- **Retain answers and panel geometry together:** This extends sizing preferences
  across tasks and risks resetting workbench focus during ownership changes.
