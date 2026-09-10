---
id: canvases-local-creation-authority-design
title: Owner-authorized canvas creation design
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-10
last_updated: 2026-09-10
requirements:
  - REQ-CANVASES-LOCAL-CREATION-001
---

# Owner-authorized canvas creation design

## Boundary and mapping

Canvases owns the creation workflow and its recorded authorization. Plugins
owns grant storage, release activation, and runtime enforcement. This design
implements `REQ-CANVASES-LOCAL-CREATION-001`; the authority, transaction, and
compatibility sections map to criteria .1-.3, .4-.5, and .6-.7 respectively.

See [the creation authority decision](../../../decisions/2026-09-10-canvas-creation-authority.md)
and [plugin grants](../../plugins/system-design/isolated-web-app-contributions.md#instance-grants).

## Trusted creation authority

`canvasAuthoringService.CreateCanvas` is the only production issuer. Resolve
the execution and session through existing trusted MCP context. Resolve the
workspace owner through the task service and retain current access checks.
Do not use `workspaceOwner`'s lookup-error fallback as authorization. An actual
auth-disabled workspace can use its persisted default-user owner; an absent
owner or failed lookup cannot mint authority.

Add a canvas-owned `canvas_creation_authority` table through
`canvas.Repository.SchemaSQL` initialization:

| Field | Contract |
| --- | --- |
| `canvas_id` | Primary key, references the canvas lifecycle identity |
| `owner_user_id` | Trusted workspace owner at creation |
| `creating_session_id` | Trusted source session |
| `policy_version` | Integer 1 for the initial-publication policy |
| `consumed_at` | Empty until the first release transaction consumes it |

Create the row in the canvas/instance creation transaction. No agent-facing
tool input can write these fields. Scope and task remain in the existing
canvas and instance records. Installation or future import paths do not write
this table, even when the resulting package belongs to the same user.
Existing databases acquire the empty table; do not backfill authority.

At publication require every predicate: a nonzero captured `PublishAuthority`,
local-canvas instance source, task scope, matching task and workspace, matching
creating session, unchanged current owner, pending instance, no active or
persisted release, no previous grant changes, and unconsumed policy version 1.
Check the full instance snapshot again inside the transaction. Re-read the
current task/workspace ownership through the same transaction and serialize
against ownership changes; the adapter's earlier lookup alone is insufficient.
A changed
owner or stale execution fails rather than falling back to inferred ownership.
The trusted adapter supplies the freshly resolved owner; caller-controlled
`SourceUserID` and `SourceActorKind` alone are not sufficient.

## Grant derivation and transaction

Derive grants from `ManifestPermissions` and the existing static web-app
capability vocabulary. Restrict Kandev resource access to the recorded task,
using normal runtime authorization. Normalize each declared external origin
through `NormalizeNetworkOrigins`; initial external grants count as delegated
owner approval, with that owner as `ApprovedBy`. This is deliberate: a locally
requested app can contact its declared HTTPS service on its first run.
Unsupported capabilities and backend/native contributions cannot gain access
through this path. Do not add a generic manifest-controlled auto-approve flag.

Extend the existing `persistAuthorityRelease` transaction. Insert the release
with the current `CreateReleaseIfAuthorityTx` guard, insert only declared
initial grants through a narrow transaction helper in `instances.Store`, set
the plugin identity, activate through `ActivateReleaseTx`, and consume the
creation record. Reuse grant normalization and declaration validation. Do not
call the separate public approval transaction after publication.

The zero-permission first release consumes authority too. A transaction failure
rolls back all writes. Invalid source rejected before persistence leaves the
draft eligible. Grant generation advances through the normal instance writes;
authority changes still invalidate issued capabilities. Publish one activation
event after commit. Retention and artifact cleanup retain their existing
post-commit and failed-publication contracts.

Later publications follow existing `permissionsFit` behavior. They cannot use
creation authority to restore revoked grants. Consumed rows remain until canvas
removal, so pruning releases cannot restore first-publication eligibility.
Archive/restore never creates a row. Canvas/task/workspace removal deletes the
associated row in its existing cleanup transaction. SQLite and PostgreSQL
schema replay and backup enumeration must include this table.

## API and compatibility

Return an additive `initial_permission_policy` field in the create response:
version 1, eligibility, scope `task`, supported permission kinds, and the exact
HTTPS-origin constraint. This is descriptive output, never a publish input.
It must not claim a permission ceiling that the backend does not enforce.
Keep the existing `activated` and `permission_required` publish result fields.

Update the versioned bundled authoring guidance during implementation. Local,
Docker, and remote executors use the same trusted path. Existing releases and
drafts remain unchanged on upgrade and can still use manual review. Operators
do not need to republish an already valid release for the separate embedding
policy correction.

## Validation and observability

Use `authoring_test.go` and `services_canvas_test.go` for owner, session,
source-kind, no-identity, initial grant, subsequent increase, and legacy cases.
Retain `TestPublishPackageFirstReleaseRequiresMatchingGrants` for canvases
without recorded authority. Add mixed eligible/ineligible instances, concurrent
publication, owner/scope drift, transaction rollback, and revoke/retry cases.
Keep runtime denial tests for foreign tasks and undeclared origins.

Diagnostics can record the policy version and safe result code. They must not
record capability URLs, source, network payloads, or grant request bodies.

## Implementation plans

- [Canvas runtime and permission fixes](../../../plans/canvas-runtime-permission-fixes/plan.md)
