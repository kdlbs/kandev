---
id: "02-frontend-types-api"
title: "Frontend: JiraStatus type + status API client"
status: done
wave: 2
depends_on: ["01-backend-status-endpoint"]
plan: "plan.md"
spec: "../../specs/jira-status-filter/spec.md"
---

# Task 02: Frontend JiraStatus type + API client

Expose the new backend endpoint to the SPA.

## Acceptance
- `apps/web/lib/types/jira.ts` exports `interface JiraStatus { id: string; name: string; statusCategory: JiraStatusCategory }`.
- `apps/web/lib/api/domains/jira-api.ts` exports `listJiraProjectStatuses(projectKey, options?)` returning `{ statuses: JiraStatus[] }`, URL-encoding the key, mirroring `listJiraProjects`.

## Verification
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched
- `apps/web/lib/types/jira.ts`
- `apps/web/lib/api/domains/jira-api.ts`

## Inputs
- Spec: Data model, API surface.
- Plan: Frontend > Types, API client.
- Pattern: existing `listJiraProjects` / `getJiraTicket` in the same file.

## Dependencies
Task 01 (endpoint contract). Type work can start in parallel but verify against the real endpoint shape.

## Output contract
Report: summary, files changed, checks run, blockers, risks; set `status: done` + tick plan checkbox.
