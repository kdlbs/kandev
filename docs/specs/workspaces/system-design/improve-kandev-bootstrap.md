---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
created: 2026-11-19
updated: 2026-11-19
owners:
  - Carlos Florencio
---

# Improve Kandev bootstrap lifecycle

This design extends the [Improve Kandev system design](improve-kandev.md).
The HTTP request acknowledges preparation without waiting for GitHub or Git.
The job calls the existing setup to resolve the workspace, register the
repository, initialize both workflows, and allocate its diagnostic directory.
[Decision](../../../decisions/2026-11-19-improve-kandev-background-bootstrap.md)

## HTTP contract

All routes below use `/api/v1/system/improve-kandev` and its existing
authentication. The `workspace_id` and `create_workspace` fields keep their
current meaning.

| Request | Response |
| --- | --- |
| `POST /bootstrap` with `async: true` | `202` with `{"job_id":"<id>"}`. |
| `POST /bootstrap` without the opt-in | The existing synchronous response. |
| `GET /bootstrap/:id` while pending | `202` with the same job ID. |
| `GET /bootstrap/:id` after completion | `200` with the existing `BootstrapResponse`, or an error with the existing safe message. |
| `GET /bootstrap/:id` for a missing or inaccessible job | `404`. |

Send `Cache-Control: no-store` for async admission and status. The browser
uses the same two response shapes for both requests.

## Backend

Keep a single bounded job map in `internal/improvekandev`. Admit only
authenticated callers. Bind each job to its initiating user, tenant, and
authentication mode.
Before returning a ready result, use the request identity to check access to
the result workspace. Each attempt gets its own diagnostic directory so
concurrent drafts can attach different archives.

Run the existing setup under a backend-owned context with the caller identity.
Use a six-minute deadline to cover the existing five-minute clone budget.
The backend cancels and joins the jobs on shutdown or database restore.
Remove expired results on request. If the map fills, evict the oldest
finished result or return `503` if all slots are active.

Reuse existing workspace, repository, and workflow resolution. For a new
managed clone, reuse the staging pattern in `repoclone/checkout_options.go`.
Clone into an owned sibling directory and rename it after Git succeeds.
Use the existing clone credentials and path lock. Canceling Git can leave a
`.git` directory without a usable commit. Staging keeps that partial checkout
off the final path. Remove the directory owned by the attempt on normal exit.
After a backend restart, a new attempt reads the resources already saved.

## Dialog

`useBootstrapKandev` opts in and polls the status once per second after
`202`. A completed response goes through the current repository and workflow
hydration. The dialog keeps the submit guard active until hydration succeeds.
Its existing progress message remains visible during preparation.

The existing error banner offers **Retry** in the same form. Retry preserves
the draft and starts a fresh request. Close or workspace change stops its
polling. Ignore any late result from a prior attempt. The shared form owns
the desktop and phone layout, with a touch-sized Retry control on phones.

The HTTP contract and dialog cover `AC-WORKSPACES-IMPROVE-KANDEV-001.14`,
`.15`, and `.18`. The job and checkout lifetimes cover `.16` and `.17`.
[Fix plan](../../../plans/improve-kandev-bootstrap-timeout/plan.md)
