---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-JIRA-PROJECT-SELECTION-001
---

# Jira Project Selection System Design

## Purpose and boundaries

Integration owns Jira project inventory and its projection into the existing
selector. The correction belongs at `CloudClient.ListProjects` in
`apps/backend/internal/jira/cloud_client.go`. It requires no new architecture,
storage, API fields, authentication policy, or ADR.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-JIRA-PROJECT-SELECTION-001` | [Provider contracts](#provider-contracts), [Request and projection flow](#request-and-projection-flow), [Failure and security](#failure-and-security), [Verification boundary](#verification-boundary) |

## Provider contracts

The [Jira Server 9.12 reference](https://docs.atlassian.com/software/jira/docs/api/REST/9.12.0/#api/2/project)
documents `GET /rest/api/2/project` returning a JSON array of visible projects.
Its project resource does not document `/project/search`.
The [modern Data Center project reference](https://developer.atlassian.com/server/jira/platform/rest/v11000/api-group-project/)
also defines the array operation. The existing comment claiming Server 8+
supports `/project/search` must be removed.
The [Cloud reference](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-projects/#api-rest-api-3-project-search-get)
defines project search with a paged `values` envelope.

| Client configuration | Request | Response handling |
| --- | --- | --- |
| `InstanceTypeServer`, existing PAT | `GET /rest/api/2/project`, no Cloud pagination query | Decode a bare array into `[]JiraProject` |
| `InstanceTypeCloud`, existing REST auth | `GET /rest/api/3/project/search?maxResults=200` | Keep current `values` decoding and projection |
| Empty instance type | Existing constructor defaults to Cloud | Same Cloud operation and envelope |
| OAuth through `DefaultClientFactory` | Existing `MCPClient` | Unchanged; outside this REST correction |

Select the Server path from the existing normalized `instanceType` before
the Cloud operation. Decode the Server response using the existing public
`JiraProject` fields (`id`, `key`, `name`). Preserve response order and additional
upstream-field tolerance. An explicit `[]` is a successful empty list.
Keep the current Cloud operation intact. Do not infer instance type from an
error or response shape, probe endpoints, or add fallback calls.

Update the method comment to describe both provider shapes accurately without
claiming a universal 200-project limit. Cloud still requests one page of 200;
the shared transport still bounds response bodies to its existing size limit.
Neither limit is repaired in this package.

## Request and projection flow

`httpListProjects` obtains the configured workspace client via
`Service.clientFor`, calls `Client.ListProjects`, and returns
`{"projects": [...]}`. The WebSocket path uses `Service.ListProjects` and the
same client contract. `DefaultClientFactory` uses `CloudClient` for Server PAT
and other existing REST auth configurations, and `MCPClient` for OAuth.
These boundaries remain unchanged.

The new Server branch calls the existing `do` helper, retaining request context,
authorization, URL composition (`siteURL + path`), error handling, redirect
policy, and transport limits. A base URL such as `https://host/jira` must result
in `/jira/rest/api/2/project`; the branch must not discard the context path.

On the web, `listJiraProjects({ workspaceId })` feeds `loadUserProjects` in
`apps/web/app/jira/jira-page-client.tsx`. That function awaits project listing
and the existing ticket search, then intersects inventory keys with returned
ticket project keys. `ProjectPill` in
`apps/web/components/jira/my-jira/filter-pills.tsx` consumes that list.
No change to eligibility, default views, saved filters, or frontend code is needed.

Desktop and phone use this shared data path. The correction changes neither
rendered composition nor viewport/touch behavior. Mobile-parity assessment:
backend protocol coverage applies to both surfaces; no new browser, screenshot,
OS, markup, copy, or mobile test gate is introduced.

## Failure and security

Return `do` errors unchanged. Non-success HTTP responses retain `APIError`
status and diagnostic; JSON decoding errors remain errors. Never substitute an
empty list for a failed request. Existing handlers retain their mappings.
My Jira currently catches project-load failure, leaving the picker empty while
other fields remain usable; this package does not redesign that presentation.

Keep existing workspace authorization and PAT Bearer headers. Tests use only
synthetic tokens and local HTTP fixtures; no production credentials or data.
No persistence migration, new telemetry, auth/OAuth policy, or retry framework
is required.

## Verification boundary

Independently author `cloud_client_projects_test.go` using `httptest` and
`NewCloudClient`. Fixtures encode official provider contracts, never derive
expected URLs or response shapes from the implementation. They must reject an
unexpected method/path/query. The Server fixture returns 404 for the old search
path and succeeds only for the documented array path. Assert the full project
identities, existing auth headers, context path, and meaningful error status/body.

Use a failing Server contract regression before production edits, plus passing
Cloud and direct documented Server transport/schema controls. Add empty/error
controls for both provider shapes and explicit Cloud/default-mode coverage.
Run the owning Jira package and scoped Go lint only, with the bounded commands
in the [work order](../../../plans/jira-server-project-selection/task-01-load-server-projects.md).
These are real-client HTTP contract tests, not evidence of a live Jira deployment.
