---
status: current
system: tasks
requirements:
  - REQ-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001
---

# GitHub issue link mutation design

## Ownership and scope

Tasks owns the durable metadata association and its preservation contract. This
focused companion supplies the previously missing issue-mutation design for the
[existing external-link requirement](../requirements/link-existing-task-github-issue.md).
It covers GitHub issue link/relink/unlink only. Provider credentials and remote
reads remain Integration-owned; repository attachment remains Workspace-owned.
Other external references, PR associations, watches and their intake/deduplication
retain their current implementations.

Reuse the [field and explicit merge contract](task-field-updates.md), including
its writer inventory, owner rules, supported ordinary value dialects, snapshot
exclusions and postcommit observation limits. The previous merge delivery excluded
the GitHub replacement adapter deliberately; this companion corrects that caller
through issue-only intent without changing either ordinary operation's contract.

## Reviewed baseline boundaries and consumers

Inventory is based on `ca1492fafa5bd86bdc02a5fff476b4428c558de8`; backend paths below
are relative to `apps/backend/`.

| Boundary | Current role and bounded change |
| --- | --- |
| `internal/github/service_task_issue.go` | `LinkTaskIssue`, `LinkTaskIssueForUser`, `linkTaskIssueResolved`, `UnlinkTaskIssue`; currently copy early metadata and set/delete five keys. Keep early task reads for existence, ownership and response title; stop using them as mutation input. |
| `github.TaskIssueStore` | Task/repository reads also serve PR validation, CI options and workspace-group watch redirection. Replace its issue-write method with the required typed issue-only method; preserve its other methods. |
| `internal/backendapp/adapters.go` | Sole production `githubTaskIssueStoreAdapter` currently sends the whole map to `Service.UpdateTask`. Forward the new method to task service, retaining `wrapGitHubTaskIssueStoreError` and both wrapped error identities. |
| `internal/task/service` | Ordinary `UpdateTask` is replacement/pending-title patch; `UpdateTaskMetadata` is explicit merge. Add a dedicated issue-only service method with existing write authorization and postcommit task observation/publication. |
| `internal/task/repository/interface.go` | Required `TaskRepository` method, implemented by SQLite/PG repository. Embedded wrappers forward it; standalone fakes need compile-only adaptation, with no optional snapshot fallback. |
| `internal/task/repository/sqlite/task_metadata_merge.go` | Existing transaction options, writer reservation, task-scoped PG advisory lock and locked raw metadata reader provide the storage pattern. Reuse small locking/reader helpers without generalizing ordinary merges or introducing a mutation framework. |
| `github.Controller.RegisterHTTPRoutes` | Existing PUT/DELETE `/api/v1/github/tasks/:taskId/issue` and GET `/api/v1/github/task-issues?workspace_id=...`. Keep controller DTOs, error classification, and `LinkTaskIssueForUser` credential resolution. |
| Web API and dialog | `apps/web/lib/api/domains/github-api.ts` link/unlink helpers; `components/task/task-github-issue-dialog.tsx` submits, reports errors and closes on success. `components/github/my-github/quick-task-launcher.tsx` links after task creation; failed linking does not undo creation. No client change. |
| Publication and rendering | `PublishTaskUpdated` feeds the existing gateway and `lib/ws/handlers/tasks.ts`/`task-merge.ts`; `lib/kanban/map-task.ts` and metadata utilities project issue fields. The workspace issue lookup parses canonical URL/number and tolerates legacy `issue_repo: owner/repo`. No new event or frontend state shape. |
| Issue-watch creation | `internal/orchestrator/event_handlers_github.go` creates initial metadata with issue URL/number/repo slug and author/watch/profile records. Preserve this legacy intake shape; explicit unlink removes only five link keys. |

`fakeTaskIssueStore` and `multiTaskIssueStore` are the existing nonproduction
implementations of the GitHub interface. Adapt them for their existing isolated
service/controller/PR tests. Permanent concurrency evidence must bypass fake
business behavior and use the real adapter, task service and database.

## Typed issue-only seam

Add `models.TaskGitHubIssueLink` in a small companion file, with `URL string`,
`Number int`, `Owner string`, and `Repo string`. It is internal data, not a new
wire model. A nonnil link is the complete validated issue identity from the
existing fetched `github.Issue`; a nil link means explicit removal of the whole
five-key association. `github_issue_linked` is derived as true for a link.
There is no arbitrary set/remove map, key mask, revision, or per-key caller API.

Required signatures:

```go
// TaskIssueStore and task Service respectively:
UpdateTaskGitHubIssue(context.Context, string, *models.TaskGitHubIssueLink) (*models.Task, error)
// TaskRepository returns the locked candidate for the existing reload fallback:
UpdateTaskGitHubIssue(context.Context, string, *models.TaskGitHubIssueLink) (*models.Task, error)
```

GitHub constructs only the typed identity; it never forwards `task.Metadata`.
The canonical storage method alone defines the five stored key names. No owner
policy filtering is needed for unrelated keys because the caller cannot supply
them. Do not invent additional provider/repository validation or alter the
existing no-attached-repository behavior while narrowing this seam.

## Flow, security and observations

1. Keep current early task lookup, workspace ownership, personal-read resolution,
   input parsing, issue fetch and attached-repository validation. In the user path
   retain `ensureRepositoryInWorkspaceScope` and `resolvePersonalReadClient`.
2. After those stages succeed, call the typed store method with
   `context.WithoutCancel(ctx)`, exactly where the current GitHub write detaches
   cancellation. Unlink retains its early existence read and the same detached
   write boundary. It requires no provider fetch or token. Cancellation before
   admission still propagates through the existing reads/fetch/validation.
3. The task service checks `authz.ScopeTaskWrite` through `authorizeTaskScope`,
   preserves the existing early existence/error boundary, and delegates to the
   required canonical repository method. Do not use `Service.UpdateTask`, its
   whole metadata replacement, or generic merge as an unlink substitute.
4. After commit, reuse `reloadTaskAfterMutation`, the ordinary update's
   `ListTaskRepositories` observation (log/retain candidate on failure), and
   `PublishTaskUpdated`. Preserve the existing ordinary-update fallback on
   postcommit row-read failure rather than importing the merge service's different
   suppression rule; the committed candidate must come from the locked current
   row for that fallback. Implementation must retain that observation
   behavior without manufacturing scalar/transition effects.
5. The link response keeps its existing fetched issue fields and early task title;
   DELETE keeps `{ "unlinked": true }`. Events may observe a later commit. There
   is no promise that the link response title is the title at commit, or that
   events across independent services have global order.

The adapter must retain error wrapping for storage/service task-not-found. Viewer
denials and foreign task nondisclosure remain under existing task authorization;
do not add new error/routing/auth behavior. No direct repository write from
GitHub may bypass task-service publication.

## Canonical persistence

Use a dedicated `task_github_issue.go` repository companion, with no schema
change. Read raw current metadata inside the writer transaction and write only
`metadata` and `updated_at`. Return a locked-current task candidate for existing
fallback observation, never the GitHub preflight snapshot. Use the separate raw
reader first, then the existing task scanner after the metadata UPDATE and before
commit, inside this same transaction. This avoids the legacy scanner's SQL NULL
limitation before normalization. Returned metadata follows the existing task
model/DTO decoding semantics; no DTO number representation change is introduced.

- Begin using `hierarchyTxOptions()` (READ COMMITTED on PG). SQLite reserves its
  writer before any metadata read through the existing zero-row writer statement.
  Preserve normalization of a context-cancelled busy error to its context cause.
- On PG take the existing task-scoped metadata advisory namespace
  `task-metadata-merge:` before reading, then lock the physical task row with
  `SELECT ... FOR UPDATE`. The physical row lock, not the advisory alone, is what
  interoperates with ordinary fields, titles, native scalar/metadata owners and
  independent connections. No workspace/step locks are acquired after this row
  lock; no hierarchy or workflow mutation is attempted.
- Reuse `currentMetadataForMerge`/`decodeMetadataMergeObject` or the smallest
  shared equivalent. Use `map[string]json.RawMessage` for unrelated values, with
  no `float64` round-trip. SQL NULL, empty metadata and JSON null normalize to an
  object as in the current merge reader. Malformed/nonobject current documents
  fail closed with no partial write. Top-level serialization can normalize
  whitespace/key order; unrelated raw numbers, nulls and nested values remain
  semantically unchanged, including number precision.
- Set all five fixed keys for a nonnil link, or delete all five for nil. Marshal
  and issue one metadata/timestamp UPDATE in the same transaction. Pending-title
  rows use this locked document directly, never SQLite `json_patch` over the
  whole document: that would delete omitted nulls and can leave removed keys.
  PG likewise performs explicit removal, never nondeleting concatenation for unlink.
- Check affected rows, commit atomically, and return the committed candidate
  for fallback. Rollback on read/encode/write/commit failure. Preserve
  `errors.Is` for missing task and cancellation. No entry dispatch, runner sync,
  transition ledger, association replacement, title write or launch work.

## Writer compatibility and value limits

| Other write | Relationship to issue-only mutation |
| --- | --- |
| Ordinary merge of unrelated keys; scalar update; typed update omitting metadata | Both intents survive either order; issue mutation changes no scalar. Native task row serialization protects current reads. |
| Human title and generated-title claim/set CAS | Preserve pending/owner present or absent and latest title. Later sanctioned title updates remain authoritative. Issue mutation never gains title authority. |
| Deferred launch, step-handoff carry, provenance, Office carrier, workspace recovery/launch-error owners | Preserve current raw values/absence when outside the five-key domain, and preserve their later scoped writes. Do not replay stripped or removed owner records from an early snapshot. |
| Materialized workspace mode/group and other workspace metadata | Untouched by issue intent; preserve the complete current workspace value. No workspace normalization or reparenting work is triggered. |
| Link/relink versus link/unlink | Whole domain last successful commit wins. Explicit unlink is deletion, including pending-title rows. Never mixed identity keys. |
| Ordinary supplied metadata or intentional full snapshot | Directional exclusion remains: a later replacement/snapshot may overwrite issue keys according to its contract. A later issue mutation preserves its current unrelated values. No retroactive protection against arbitrary future snapshots. |
| Other providers, issue watches, tables/associations | Existing shapes/intake/authorization unchanged. Unlink retains unrelated watch/profile/author data. No cross-table transaction. |

The ordinary merge's SQLite recursive pending null/nested semantics and PG
shallow semantics remain unchanged. Issue operations supply only complete fixed
scalar identities/removal, so their unrelated raw values do not enter those
ordinary merge expressions. This is preservation of omission, not a new uniform
value dialect or nested-key concurrency guarantee.

## Failure and verification mapping

Existing external read errors and repository mismatch precede storage and leave
the row untouched. A missing task discovered at mutation is still typed missing
task. Native lock waits/cancellation and injected real trigger rejection must
show rollback of metadata and timestamp, with no mutation success event. A
postcommit failure is not rollback; pin existing fallback/observation separately.

| Criteria | Evidence in the single work order |
| --- | --- |
| `.1`, `.2` | Permanent existing-API backendapp RED, both delayed link/unlink versus accepted merge orders, sequential/scalar/title controls, independent tasks/workspaces. |
| `.3` | Same-domain link/link and link/unlink whole-identity order; legacy watch unlink and relink; all five keys asserted in real persisted rows. |
| `.4` | Raw-number/null/nested/pending-owner/server-owned/workspace controls, unchanged populated scalar/associated state and effect tables. |
| `.5` | Actual registered PUT/DELETE and task PATCH/preference routes, DB/DTO/normal event checks; auth/provider/validation/fetch/error/cancellation/fallback controls; actual PG physical wait/current result and SQLite busy/rollback. |

All criteria use prefix `AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001`.
See [the plan](../../../plans/preserve-github-issue-metadata/plan.md) and
[one sequential work order](../../../plans/preserve-github-issue-metadata/task-01-atomic-issue-mutation.md).

## Documentation and mobile audit

Backend state/data only: existing desktop/mobile dialogs, DTOs, events, layout,
touch/scroll/navigation and copy remain identical. Registered protocol tests are
the causal end-to-end proof; no visual/browser/build/E2E work is required absent
ROOT's causal amendment. Public docs audit covered `integrations.md`,
`tasks-and-workflows.md`, `websocket-api.md`, root README and screenshots. During
implementation, the smallest useful clarification is one paragraph next to
partial-update semantics in `docs/public/websocket-api.md`, describing existing
issue PUT/DELETE preservation and retaining replacement/snapshot exclusions.
No new public page, UI copy or screenshot. The design and shared contract contain
the full rationale; the `/record` audit adds no independent ADR or framework.
