---
created: 2026-11-08
status: complete
requirements:
  - REQ-WORKSPACES-DELETION-001
system_design:
  - ../../specs/workspaces/system-design/deletion.md
legacy_specs: []
---

# Implementation Plan: Workspace deletion permission errors

## Overview

Correct the HTTP and WebSocket projection of workspace deletion failures for
[issue #5171](https://github.com/kdlbs/kandev/issues/5171). Readable collaborators
must receive a permission error, while hidden workspaces remain indistinguishable
from missing workspaces. Genuine storage failures retain internal-error responses.

One sequential work order adds causal regression coverage and qualifies the two
existing deletion mappers. Service authorization and deletion remain unchanged.
The work order completed on 2026-11-08; see Verification results.

## Authority and traceability

Workspaces owns the deletion response contract, independently of the caller's
transport. The existing active
[deletion requirement](../../specs/workspaces/requirements/deletion.md) remains
the source of truth. Its existing IDs cover confirmation and successful cleanup.
New criteria 001.9 through 001.11 make the missing response guarantees explicit.
This is an amendment to the existing capability, without a separate repair spec.

| Requirement / AC | Evidence owned by this package |
| --- | --- |
| `REQ-WORKSPACES-DELETION-001`, `AC-WORKSPACES-DELETION-001.9` | Real readable collaborator/viewer denial, both transports, no mutation, cleanup, or deletion events |
| `AC-WORKSPACES-DELETION-001.10` | Nonmember, foreign-organization, and missing workspace parity without name disclosure |
| `AC-WORKSPACES-DELETION-001.11` | Real cascade abort and rollback, including misleading error text; focused wrapped-error classification |
| `AC-WORKSPACES-DELETION-001.2` | Existing exact-name guard and request validation controls |
| `AC-WORKSPACES-DELETION-001.3`, `.4`, `.5` | Authorized success and existing service cascade/event/resource regressions |

The [deletion design](../../specs/workspaces/system-design/deletion.md#generic-deletion-error-projection)
declares this requirement and specifies the local transport contract. The
[active auth requirement](../../specs/auth/requirements/auth.md), especially
`AC-AUTH-AUTH-001.7`, supplies the privacy constraint. The
[roles and scopes document](../../specs/auth/requirements/roles-and-scopes.md)
supplies management-scope and readable-denial policy. Its frontmatter is still
`draft`, although the role resolver shipped in merged
[PR #2919](https://github.com/kdlbs/kandev/pull/2919). Do not promote that document
or migrate its broader contract in this repair.

The [accepted auth ADR](../../decisions/2026-07-24-opt-in-authentication.md)
retains service-level scoping. No new architectural or access-policy decision
needs an ADR. The separate draft
[cascade boundary design](../../specs/tasks/system-design/archive-cascade-boundary-contracts.md)
describes a future Store API and is not an implementation prerequisite here.

## Investigation and confidence

### Observed facts

- This task's initial clean checkout and live `main` both resolved to
  `9aba7b2edd020f6ba0f92c2bfe318429723ea2ed` on 2026-11-08.
- The canonical issue reports stock v0.99.1, correct confirmation, an empty
  workspace, and readable collaborators through bearer and password-login access.
  The live issue had no comments or attachments.
- `DeleteWorkspaceWithConfirmName` loads the workspace, checks management scope,
  compares the name, then enters deletion. `requireWorkspaceManage` returns
  `ErrForbidden` for a readable non-manager and typed workspace absence for
  unreadable callers. These checks are already correct.
- `handleHTTPDeleteWorkspaceError` and `wsDeleteWorkspaceError` omit permission
  denial, so it reaches their internal-error branches.
- Both mappers call `isNotFound`, whose legacy substring fallback can mistake
  a storage error containing `not found` for workspace absence. The real
  workspace repository already returns a wrapped `ErrWorkspaceNotFound` sentinel.

A disposable Go overlay exercised registered Gin routes and the WS dispatcher,
the real task service and authz resolver, and temporary SQLite databases. It
confirmed both defects without product or permanent-test changes. A database
trigger raised a real cascade error rather than substituting a handler result.

| Case | Current HTTP | Current WS | Mutation evidence |
| --- | --- | --- | --- |
| Readable collaborator, correct name, empty workspace | `500` | `INTERNAL_ERROR` | Workspace retained, no published event |
| Readable collaborator with a workflow | `500` | `INTERNAL_ERROR` | Workspace and workflow retained |
| Readable collaborator, wrong nonempty name; viewer | `500` | `INTERNAL_ERROR` | Denial precedes name comparison, no event |
| Owner; admin who is also workspace owner | `200` | Success response | Workspace removed, `workspace.deleted` published |
| Authorized caller, wrong or empty name | `400` | `VALIDATION_ERROR` | Workspace retained, no event |
| Nonmember with correct or wrong nonempty name; missing workspace | `404` | `NOT_FOUND` | No name disclosure or deletion event |
| Nonmember with empty name | `400` | `VALIDATION_ERROR` | Payload validation remains first |
| Storage trigger abort, ordinary message | `500` | `INTERNAL_ERROR` | Workspace and workflow retained after rollback |
| Storage trigger abort containing `not found` | `404` | `NOT_FOUND` | Rows retained, but the error category is wrong |

The expanded diagnostic passed all 28 observations of current behavior. The
initial run also passed eight existing deletion-handler tests. Changing only the
temporary expectations to the desired responses provides a causal RED probe;
its exact outcome is recorded under Verification results.

**Confidence for #5171: high.** The reported HTTP defect and the parallel WS
defect reproduce through current registered handlers. The storage-text edge is
also verified and falls within the requested internal-failure safeguard.
There is no unresolved root-cause hypothesis that blocks this independent fix.

Limits: identities entered through `authn.WithIdentity`, after authentication.
This was an in-process transport/service/SQLite probe, without a live deployment,
network WebSocket upgrade, password login, bearer verification, browser, or
runtime cleanup worker. The two credential paths remain issue evidence, not a
claimed local end-to-end reproduction.

## Settled intent and scope

The assumption check found no material product choice requiring a question.
Management remains scope-based; an org admin receives no private-workspace bypass.
Payload validation remains first, then reach/manage authorization, then name
comparison. No issue suggestion changes these boundaries.

### In scope

- Correct both generic deletion error mappers using established sentinels and codes.
- Use fixed safe permission messages without wrapped implementation details.
- Recognize typed workspace absence locally, preserving hidden/missing parity.
- Prove role denial, ordering, preservation, genuine storage failure, and success.
- Add only the missing criteria and design detail to the existing deletion pair.

### Out of scope

- Scope grants, roles, authentication, authorization service changes, or new settings.
- Global changes to `errors.go:isNotFound` or unrelated endpoint error mapping.
- Office deletion, cascade redesign, transaction changes, side-table cleanup,
  resource reclamation, new events, or the future lifecycle Store migration.
- Frontend, navigation, control visibility, rendering, product copy, or locale changes.
- Executor selection/replacement, task environment reset, or shared-secret policy.
- Issues #5172, #5204, #5205 and deferred #5191, #5186, #5203, #5169.
- Implementation, permanent tests, publication, task transitions, or delegation
  during this planning turn.

## Technical approach

Change only `handleHTTPDeleteWorkspaceError` and `wsDeleteWorkspaceError` in
`apps/backend/internal/task/handlers/workspace_handlers.go`. Keep the existing
confirmation branch first. Qualify workspace absence with
`errors.Is(err, repoerrors.ErrWorkspaceNotFound)`, then recognize
`service.IsForbidden(err)` before the internal fallback. Keep HTTP `404` and WS
`NOT_FOUND` messages unchanged. The permission message on both transports uses
`service.ErrForbidden.Error()`, with HTTP `403` and `ws.ErrorCodeForbidden`.
Do not forward a wrapped error's text.

This requires one repository-error import, without a generic mapping framework.
Update the two legacy missing-workspace test fixtures to use the real typed
sentinel. Add focused error-projection cases for wrapped sentinels and unrelated
errors. Keep those tests secondary to the actual role-denial integration.

Do not edit `service_resources.go`, `service_errors_authz.go`, authorization
resolution, or repository cascade methods. An API error is projected after the
service decision; no permission or mutation policy changes.

### Transport and identity compatibility

| Surface / identity | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| HTTP + authenticated readable collaborator/viewer | `403`, fixed existing permission message | Registered route, real membership/resolver and SQLite |
| WS + same resolved identity | `FORBIDDEN`, same safe message | Registered `workspace.delete` dispatcher; envelope and correlation asserted |
| Bearer or maintained login session | Same result after identity resolution | Canonical issue covers both; local probe begins after authentication |
| Unreadable or foreign organization; missing ID | Fixed `404` / `NOT_FOUND` | Real resolver/absence tests; never name comparison output |
| Real manager, including owner/admin-owner | Existing success or confirmation validation | Positive deletion and typed name-race controls |
| Synthetic identity or internal context | Existing unscoped policy | Existing service behavior; compatibility control, no new credential path |
| Office route selected by the existing web action | Existing Office behavior | Separate route, no coverage claim or dependency |
| Malformed payload / empty name; WS empty ID | Existing validation response | No repository or cleanup access |

## Tests and acceptance evidence

Add `workspace_delete_permissions_integration_test.go` beside the handlers.
Reuse `workflowHandlerTemplate.Open`, `newTestLogger`, registered route/dispatcher
patterns from `workspace_field_updates_integration_test.go`, and real SQLite
membership rows. Build a small dedicated deletion fixture; do not reuse that
settings fixture's concurrency gates or its partial service repository wiring.

| Proposed test | Criteria / causal assertion |
| --- | --- |
| `TestWorkspaceDeleteAuthorizationIntegration` | AC 001.9/.10: collaborator and viewer, correct/wrong names, empty and dependent workspaces, inherited unit membership, nonmember/admin nonmember, foreign org, missing ID |
| `TestWorkspaceDeleteValidationIntegration` | AC 001.2/.9/.10: payload/empty-name/empty-WS-ID validation before repository reads; manage/reach before nonempty name comparison |
| `TestWorkspaceDeleteSuccessIntegration` | AC 001.2/.3/.4/.5: owner, admin-owner, synthetic/internal success controls and ordinary event publication |
| `TestWorkspaceDeleteStorageFailureIntegration` | AC 001.11: actual SQLite cascade abort with ordinary or `not found` text, fixed internal response and transaction rollback |
| `TestWorkspaceDeleteErrorProjection` in `workspace_delete_errors_test.go` | AC 001.9/.10/.11: wrapped permission/absence/confirmation sentinels, fixed safe messages, and unrelated errors containing permission/absence words |

For denied dependent fixtures, retain a workflow, task, session and workspace
membership. Observe the real rows after the response. Wrap the real task
repository's `ListTasksByWorkspace` to observe entry into `deleteWorkspace`.
Install recording `SetCanvasCleanup` and `SetWorkspaceSecretDeleter` dependencies.
Assert zero calls and no new cleanup jobs on denial. An owner success control
must activate those recorders. This avoids assertions against unused nil
dependencies. Capture publication synchronously at the event bus seam, or drain
asynchronous observations deterministically. Never use sleeps as evidence of absence.

The existing `TestService_DeleteWorkspace*` tests provide confirmation-race,
cleanup, rollback and child-event compatibility checks. Do not append to the
already oversized `service_resources_test.go` or change its implementation.

## End-to-end and mobile evidence

The bounded acceptance proof starts at registered HTTP/WS handlers and ends at
real service authorization and SQLite state. It deliberately excludes credential
verification, actual sockets and runtime resources. No artificial browser test
is needed for this transport-only correction.

`useWorkspaceDeleteDraft` already preserves the dialog/name and route on failure;
`fetchJson` rejects non-success responses. Manage-scope gating prevents the usual
collaborator UI action on both desktop and phone. No rendered, interaction,
navigation, or client-state change is planned, so `/mobile-parity` requires no
new preview, screenshot, or mobile Playwright case here. UI scope expansion needs
a separate design assessment.

## Work orders

- [x] [Task 01: Project workspace deletion errors](task-01-project-delete-errors.md)
  — `done`, wave 1, no prerequisite, sequential.

Run the work order's RED test command before any mapper edit. Then make the
bounded correction, run GREEN, and run its exact compatibility and lint commands.
No external PR must merge before this order can begin.

## Dependencies, overlap and issue assignment

The PR check inspected all 146 open PR bodies and the 100 most recent merged
PRs on 2026-11-08. It also inspected the canonical issue timeline and searches
for #5171 and semantic overlap. There was no linked or explicit open fix PR.

- [PR #5179](https://github.com/kdlbs/kandev/pull/5179), head
  `92c8f001a15f4ce2330998da625f94126e4be6e9`, handles retained executor bindings
  and workflow handler denials. It does not touch generic workspace deletion.
  Keep #5172/#5204/#5205 out of this package.
- [PR #5184](https://github.com/kdlbs/kandev/pull/5184), head
  `29989fc62d8f55d18ab254a80433960b9d4ced1d`, changes executor-profile sections
  of `service_resources.go`. Its inspected patch leaves deletion untouched.
  Shared-secret policy is a separate task, without a delivery dependency here.
- PRs #4936, #4773, #3660 and #4735 have adjacent deletion/lifecycle concerns.
  Their file lists do not change these mappers or the owning deletion pair.
  Existing lifecycle events and their cleanup consumers remain intact.
- Merged PRs #4392 and #2455 cover Settings catalogue preservation and generic
  versus Office route selection. Their behavior remains unchanged. The existing
  catalogue-preservation plan is historical context, without reopened work.

`gh api user` confirmed `carlosflorencio`. After diagnosis, a fresh issue and PR
check still showed OPEN, unassigned and uncovered. Assignment succeeded at
2026-11-08 20:59 UTC and a live read confirmed `carlosflorencio` as the sole
assignee. No issue/PR comment, closure, or other issue assignment occurred.
Recheck main, ownership and relevant PR heads before later implementation.

## Documentation assessment

Internal docs updated: the existing workspace deletion requirement/design and
this two-file package. `/docs-maintainer` found the existing reference pages
`docs/public/team-access.md` and `docs/public/websocket-api.md` already describe
the management boundary, `403`, and standard `FORBIDDEN`/`NOT_FOUND` codes.
Root README, `docs/screenshots.md` and `docs/public/tasks-and-workflows.md` need
no new operation, setting, label, procedure, or image. No public-doc change is
needed. Backend API messages reuse existing English diagnostics; there is no
new frontend copy or localization key.

## Verification results

Implementation checks: **complete** on 2026-11-08. Production changes are
limited to the two mappers in
`apps/backend/internal/task/handlers/workspace_handlers.go`; permanent tests are
`workspace_handlers_test.go`, `workspace_delete_permissions_integration_test.go`,
and `workspace_delete_errors_test.go`. No other production file changed.

| Implementation check | Exact result |
| --- | --- |
| Causal RED overlay | Reverted only the two mapper branches through a temporary Go overlay. Exit 1 with 13 expected subtest failures: four error-projection cases, six HTTP/WS permission denials, the wrong-name permission-order case, and both storage-text cases. Success, hidden/missing parity, admin, foreign-org, validation, and rollback controls passed. |
| Targeted GREEN gate | Exit 0. The five named tests passed for both transports, including real membership/resolver denials, retained dependent rows, zero cleanup calls, unchanged resource-cleanup job rows, and typed storage rollback. |
| Existing deletion handler tests | Exit 0. `TestHTTPDeleteWorkspace*` and `TestWSDeleteWorkspace*` passed with typed absence fixtures. |
| Existing service deletion tests | Exit 0. `TestService_DeleteWorkspace*` passed with unchanged cascade, cleanup, rollback, and event behavior. |
| Race variants of all three gates | Exit 0. No race reports. |
| Full `internal/task/handlers` package | Exit 0 in 76.9s. |
| Full `internal/task/service` package | Exit 0 in 103.8s. |
| `go build -trimpath ./...` from `apps/backend` | Exit 0. |
| `bash scripts/lint-go-changed` on the four changed Go files | Exit 0, 0 issues. |
| `gofmt` on the changed Go files | Exit 0, no remaining differences. |
| `python3 scripts/list-docs.py validate` | Exit 0. Validated 439 decisions and 1,946 specifications. |
| `python3 scripts/lint-spec-files.test.py` | Exit 0. All 36 linter tests passed. |
| `python3 scripts/lint-spec-files.py --all` | Exit 0. All specification files passed. |
| PR-documentation `validateCoverage` on all changed docs and Go files | `covered`, zero errors. |
| `git diff --check -- apps/backend/internal/task/handlers docs/specs/workspaces docs/plans/workspace-delete-permission-errors` | Exit 0, no whitespace errors. |

The following planning checks completed on 2026-11-08 against the recorded base.

| Check | Exact result and limit |
| --- | --- |
| Initial disposable overlay and existing handler controls | Exit 0. All 26 diagnostic observations and eight existing deletion-handler tests passed. |
| Expanded disposable overlay | Exit 0. All 28 observations of current behavior passed, including the real misleading-text storage failure. |
| Temporary desired-response RED overlay | Expected exit 1. Ten response assertions failed for the two mapper defects; eighteen controls passed. No setup failure. |
| `python3 scripts/list-docs.py validate` | Exit 0. Validated 439 decisions and 1,946 specifications. |
| `python3 scripts/lint-spec-files.test.py` | Exit 0. All 36 linter tests passed. |
| `python3 scripts/lint-spec-files.py --all` | Exit 0. All specification files passed. |
| PR-documentation `validateCoverage` preflight | Actual four-file planning change: `exempt`. Prospective mapper change: `covered`, one accepted work order, zero errors. |
| `git diff --check -- docs/specs docs/plans/workspace-delete-permission-errors` | Exit 0. No whitespace errors. |
| Requirement preservation assertion | Existing AC 001.1 through 001.8 and the entire requirement 002 section match HEAD exactly. |

Diagnostic commands ran from `apps/backend` with Go 1.24.4 on Linux/amd64.
The overlays and logs were isolated under
`/tmp/kandev-5171-analysis-40b5ba24`, outside the repository. The temporary
sources are investigation artifacts, not permanent tests or implementation.

```bash
go test -trimpath -p 2 -overlay /tmp/kandev-5171-analysis-40b5ba24/overlay.json ./internal/task/handlers -run 'TestAnalysis5171RegisteredDeletionTrace|TestHTTPDeleteWorkspace|TestWSDeleteWorkspace' -count=1 -v
go test -trimpath -p 2 -overlay /tmp/kandev-5171-analysis-40b5ba24/overlay.json ./internal/task/handlers -run '^TestAnalysis5171RegisteredDeletionTrace$' -count=1 -v
go test -trimpath -p 2 -overlay /tmp/kandev-5171-analysis-40b5ba24/red-overlay.json ./internal/task/handlers -run '^TestAnalysis5171RegisteredDeletionTrace$' -count=1 -v
```

The first command used the initial 26-case source, before the two storage-text
cases expanded the same scratch overlay. The RED command changed expectations
only, with no production edits. No race, browser, live credential, full backend,
or compute-cleanup check ran during planning. The work order owns later race
and compatibility verification.

The PR-documentation preflight uses Node 24.16.0 and the repository validator.
The following command is reproducible from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/workspaces/requirements/deletion.md',
  'docs/specs/workspaces/system-design/deletion.md',
  'docs/plans/workspace-delete-permission-errors/plan.md',
  'docs/plans/workspace-delete-permission-errors/task-01-project-delete-errors.md',
];
const code = [
  'apps/backend/internal/task/handlers/workspace_handlers.go',
  'apps/backend/internal/task/handlers/workspace_handlers_test.go',
  'apps/backend/internal/task/handlers/workspace_delete_permissions_integration_test.go',
  'apps/backend/internal/task/handlers/workspace_delete_errors_test.go',
];
const paths = [...docs, ...code];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [
  ...docs.map(filename => ({ filename, status: filename.startsWith('docs/plans/') ? 'added' : 'modified' })),
  ...code.map(filename => ({ filename, status: filename.includes('workspace_delete_') ? 'added' : 'modified' })),
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(result.status, result.errors);
if (!result.ok) process.exitCode = 1;
JS
```

At implementation completion, the workspace changes are these four
documentation files, the bounded correction in `workspace_handlers.go`, the
typed-absence fixture update, and the two new handler test files. The work order
is `done`, the plan is `complete`, and all listed implementation checks passed.
The final remote main, issue assignment, and PR #5179/#5184 heads were rechecked
at 21:38 UTC. They match the evidence recorded here, with no open PR for #5171.

## Risks

- Using an unowned workspace fixture grants unscoped compatibility access and
  cannot reproduce a collaborator denial. Seed a real owner and membership.
- An org admin is not automatically a workspace manager. Success controls must
  hold management scope; unreachable admins remain not-found.
- Reusing the substring classifier reintroduces false absence for storage errors.
  Replacing that classifier globally exceeds this package's scope.
- Wrapped error text can expose names or storage detail. Only the fixed
  permission message and existing not-found messages belong in responses.
- A post-commit cleanup failure can accompany an already committed deletion.
  Preserve that service behavior and internal-error category; do not promise
  rollback for every `500` or synthesize deletion events in the handler.
- This package does not address concurrent membership revocation after the
  existing service check or implement the draft cascade Store transaction model.
- The local diagnosis does not verify live authentication or real compute cleanup.
  Those are explicit evidence limits, not blockers for the isolated mapper fix.
