---
id: "06-e2e-disposition"
title: "E2E coverage for the disposition control (withdrawn)"
status: withdrawn
wave: 5
depends_on: ["05-frontend-disposition-control"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 06: E2E coverage for the disposition control — WITHDRAWN 2026-08-15

This task was built and then cut from scope alongside
[task-04](task-04-disposition-endpoint.md) and
[task-05](task-05-frontend-disposition-control.md). See task 04 for the reason
and the spec's Amendment history for the contract change.

Both specs it produced — `apps/web/e2e/tests/pr/pr-disposition.spec.ts` and its
mobile sibling — are deleted by
[task-07-narrow-to-upstream-scope](task-07-narrow-to-upstream-scope.md). **No
replacement spec is added.** The narrowed feature has no user-visible surface
(spec AC-30b), so there is nothing on screen to assert; E2E is not required for
this change.
