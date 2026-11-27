---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-001
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-002
---

# Linear Watch Project System Design

## Purpose and boundaries

The integration system owns the Linear project filter, the team project
inventory, and the project fields on `LinearIssue`. The orchestrator owns
prompt interpolation. The watch filter is stored as JSON in the existing
`filter_json` column, so the new `projectIds` field needs no migration and old
rows decode with an empty list.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-001.1` | Search filter |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-001.2` | Search filter; Validation |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-001.3` | Project inventory; Watch dialog |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-002.1` | Issue projection; Prompt interpolation |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-002.2` | Project descriptions |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-002.3` | Prompt interpolation |
| `AC-INTEGRATIONS-LINEAR-PROJECTS-002.4` | Prompt interpolation; Watch dialog |

## Search filter

`SearchFilter.ProjectIDs` (`projectIds` on the wire) maps in
`buildIssueFilter` to `project: { id: { in: [...] } }` on Linear's
`IssueFilter`. Linear combines top-level filter fields with AND and the `in`
comparator with OR, so a watch matches any selected project and all other
filters. An empty list adds no condition.

## Validation

`normalizeFilter` trims project IDs and drops blank entries, using the same
helper as state and label IDs. `filterIsEmpty` counts a non-empty project list,
and the create and update validation messages list `projectIds`.

## Issue projection

`issueFragment` adds `project { id name url description }`. `Project.description`
is Linear's short summary and maps to `LinearIssue.ProjectSummary`; `id`,
`name`, and `url` map to `ProjectID`, `ProjectName`, and `ProjectURL`. A null
project leaves all four empty. The fragment does not request the project's
markdown `content`, so browse search and watches that do not need it keep
their current response size.

## Project descriptions

`CheckIssueWatch` collects unseen issues as today. When the watch prompt
contains `linear.ProjectDescriptionPlaceholder`, it calls
`Client.GetProjectContents` once with the distinct project IDs of those
issues and copies each project's markdown `content` into
`LinearIssue.ProjectDescription` before sorting and publishing. The lookup
uses `projects(filter: { id: { in } }, includeArchived: true)` in batches of
25 IDs. This limits the number of project bodies per request, not their byte
size. Project content has no size limit, so even one body can exceed the
client's 4 MB response limit. An oversized response fails decoding and the
poll; repeated polls can keep failing until upstream content or the watch
prompt changes. A project that Linear does not
return renders an empty description. A lookup error fails the poll before any
issue is published; dedup rows are reserved only after publication, so the
next poll retries the same issues.

## Project inventory

`Client.ListProjects(teamKey)` queries `projects` with
`accessibleTeams.some.key.eq` and `orderBy: updatedAt`, 250 per page. It shares
`collectPages` with `ListUsers`: every page keeps the same filter and
credential, a missing completion flag or an empty or repeated continuation
cursor fails the whole call, and no partial list is returned.
`GET /api/v1/linear/projects?team_key=` returns `{ projects: [{ id, name }] }`
through the workspace-scoped client, like the labels and users endpoints. The
E2E mock client serves projects seeded through `POST /api/v1/linear/mock/projects`.

## Watch dialog

`useTeamsAndStates` loads projects with states, labels, and users for the
selected team and caches them per workspace and team, because two workspaces
can connect different Linear organizations with the same team key.
`switchWatchWorkspace` clears the previous workspace's state, label, project,
and creator IDs together with the shared workflow and repository fields. `FormState.projectIds` is read from and
written to the filter, so saving an edited watch sends the stored project IDs
back. `ProjectMultiSelect` renders toggle chips under Labels. With no team,
only saved selected IDs render as removable "Unavailable project" chips; an
empty selection keeps only the no-team helper. The hook never requests project
inventory without a team. Selected IDs missing from a selected team's list
also remain removable.
On coarse pointers each chip button has a 44 px minimum hit area. The team selector clears `projectIds` with states and
labels. The watch table summary adds `project:N selected`. Desktop and phone
share the same dialog composition; chips wrap on narrow screens.

`linearIssueWatchPlaceholders` adds the four project placeholders so the prompt
editor offers them. New copy is localized in every shipped locale.

## Prompt interpolation

`interpolateLinearPrompt` adds the four project placeholders to its single
`strings.Replacer`, so substituted values are never re-expanded and templates
without the new placeholders render unchanged. The description is truncated
to `linearProjectDescriptionMaxRunes` (4,000) runes, counted as Unicode code
points, and then followed by a fixed truncation notice.

Provider placeholders are interpolated at task creation by
`LinearWatcherSource.BuildTaskRequest`. The content-lookup gate reads the
literal watch prompt. Saved-prompt references expand later through
`applyDirectPromptReferenceContext` / `expandPromptReferencesWithContext`,
which append context without another provider-placeholder pass. Put project
placeholders directly in the watch prompt; this preserves the existing
cross-provider order.

## Security and compatibility

Project text is owner-authored Linear content with the same trust level as
the issue description, which watch prompts already include. No credential,
permission, or workspace-scope rule changes. Existing watches have no
`projectIds`, send the same search conditions, and make no additional Linear
requests. The prompt captures the project description at poll time; later
edits do not reach running tasks.

## Verification boundary

Real `GraphQLClient` requests against `httptest.Server` cover the issue
projection, filter mapping, project pagination, and description batching.
Service tests cover validation and the description lookup gate and failure.
Orchestrator tests cover rendering, empty projects, and truncation. Web unit
tests cover the form round-trip; Playwright covers project selection and
edit preservation and project-only unavailable-chip removal on desktop and
phone. Mock UI flows verify form persistence, not provider matching:
`MockClient.SearchIssues` ignores filters. GraphQL request tests verify the
real client construction against an isolated server. Follow-up hosted
verification results are retained with the PR review evidence; they are pending
until those exact-source runs complete.
