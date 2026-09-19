---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001
created: 2026-08-11
owners:
  - tbd
---
# Copy and Move Secrets Between Scopes System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Moving a credential between a workspace and the user's Global set (or between
workspaces) currently means reveal, copy the plaintext, create a new secret,
and delete the old one. Users must handle the value outside Kandev, which is
both tedious and leak-prone. Kandev should copy or move a secret across scopes
as one operation, without ever showing the value.

## What

- Every secret row on the Global secrets page (`/settings/general/secrets`) and
  on a Workspace secrets page (`/settings/workspace/:id/secrets`) offers a
  **Copy/Move** action.
- The action opens one dialog that works the same on both pages. It contains:
  - a **Copy / Move** radio (default Copy);
  - a **destination** picker: **General** plus every workspace; when the source
    secret is Workspace-scoped its own workspace is excluded, and when the
    source is Global, General is excluded (a same-scope destination is a no-op);
  - an editable **target name** field, pre-filled with
    `<name> (from Global)` for a Global source or `<name> (from <workspace
    name>)` for a Workspace source.
- **Copy** creates a new secret in the destination scope with the chosen name
  and the source's value. The source stays.
- **Move** copies the secret to the destination and then removes the source, as
  one atomic operation. The dialog states that the original will be removed
  from its current scope.
- Workspace-to-workspace copy/move is supported through the destination picker.
- The persisted default target name is locale-independent: the origin token is
  the literal string `Global` for a Global source and the raw workspace name
  for a Workspace source. Only the surrounding pattern
  (`<name> (from <origin>)`). The dialog default is truncated so
  its UTF-8 encoding is at most 100 bytes, matching the backend's byte-based
  name limit.
- If the chosen target name already exists in the destination scope, the
  operation is blocked: the name field is marked invalid (red) with an error
  message. The backend enforces this **atomically** inside the same transaction
  that creates the copy and returns `409 Conflict`; the dialog also pre-checks
  against loaded destination names when available.
- Copying or moving into the scope the secret already belongs to is rejected
  with a clear validation error.
- Copying or moving a secret to a workspace requires that the destination
  workspace exists and that the caller has access to it; a workspace source
  also requires access to its owning workspace. Missing, unauthorized, or
  nonexistent targets resolve to "not found" and never disclose whether the
  secret or workspace exists.
- After a successful operation the visible lists update immediately: the
  source list drops the secret after a Move, and the destination list shows
  the new secret when it is the page being viewed. A Move or Copy into General
  always adds the returned item to the Global store, regardless of which page
  started it.
- The flow never reveals or copies plaintext through the UI; the encrypted
  value moves between rows server-side.

## API surface

### HTTP

```text
POST /api/v1/secrets/:id/copy?workspace_id=<source-workspace-id>
POST /api/v1/secrets/:id/move?workspace_id=<source-workspace-id>
```text

The `workspace_id` query parameter identifies the source secret's workspace and
is required when the source is Workspace-scoped (matching the existing scoped
CRUD routes). The request body:

```json
{
  "target_scope": "global" | "workspace",
  "target_workspace_id": "<id>",
  "name": "<optional target name>"
}
```

- `target_workspace_id` is required when `target_scope` is `workspace`.
- `name` is optional with presence semantics: a JSON-omitted `name` means "use
  the source secret's name"; a present `name` must be non-empty after trimming
  (an explicitly empty or whitespace-only name is a `400` validation error),
  and an explicit JSON `null` is rejected as a `400` validation error rather
  than treated as omitted. The name limit is 100 UTF-8 bytes after trimming,
  matching the existing create/update validation.
- Success returns `201 Created` with the new `SecretListItem` (for Move the
  source is already deleted at that point).
- Errors:
  - `400` invalid payload or validation (bad scope/workspace combination,
    same-scope destination, invalid name);
  - `404` missing or unauthorized source, or missing/unauthorized destination
    workspace;
  - `409` target name conflict (atomically detected);
  - `500` sanitized internal failure (decryption, storage). Details go to
    server logs, never the response.

### WebSocket

```text
secrets.copy
secrets.move
```text

Payload: `{ "id": "<source>", "workspace_id": "<source-workspace>",
"target_scope": "global" | "workspace", "target_workspace_id": "<id>", "name":
"<optional>" }` (`name` follows the same presence semantics as HTTP;
`workspace_id` follows the same source-scoping rule as the HTTP query
parameter: omitted or empty selects a Global source, and a Workspace source
requires a non-empty value, resolving to not-found when missing). Success
responses return the new `SecretListItem` as the WS payload (same fields as
the HTTP response), mirroring the existing `secrets.create` convention. Error
codes: `ErrorCodeConflict` (name conflict), `ErrorCodeNotFound`
(missing/unauthorized source or destination), `ErrorCodeBadRequest`
(validation), `ErrorCodeInternalError` (unexpected failures, sanitized).

### Go service surface

```go
type CopySecretRequest struct {
    Scope       SecretScope `json:"target_scope"`
    WorkspaceID string      `json:"target_workspace_id"`
    Name        *string     `json:"name,omitempty"` // omitted = use source name; null/empty/whitespace = validation error
}

var ErrSecretNameConflict    = errors.New("a secret with this name already exists in the target scope")
var ErrWorkspaceAccessDenied = errors.New("workspace access denied or workspace not found")
var ErrSecretValidation      = errors.New("secret validation failed")
```

Handlers classify exclusively with `errors.Is` against these sentinels and
`ErrNotFound`; they never match error text.

Because Go's standard JSON decoding maps both an omitted key and an explicit
`null` to a nil pointer, the request type (or the handler decode path)
implements presence-aware decoding: a custom `UnmarshalJSON` records whether
the `name` key was present, so the service rejects explicit `null` as a
validation error while defaulting a truly omitted key to the source name.

func (s *Service) Copy(ctx context.Context, sourceID, sourceWorkspaceID string, req *CopySecretRequest) (*SecretListItem, error)
func (s *Service) Move(ctx context.Context, sourceID, sourceWorkspaceID string, req *CopySecretRequest) (*SecretListItem, error)
```

The database schema is unchanged and `scope`/`workspace_id` stay immutable;
the `ScopedSecretStore` interface gains the two transfer methods below. The
service authorizes and validates, then delegates to atomic store operations:

```go
// ScopedSecretStore additions. sourceWorkspaceID is the RESOLVED source's
// workspace ("" for a Global source); a Move locks it alongside the
// destination so concurrent same-source moves serialize without deadlock.
// verifyDestination runs inside the transfer transaction while the
// destination lock is held, before the conflict check and insert; returning
// an error rolls back and surfaces that error (the service maps it to
// ErrWorkspaceAccessDenied / 404).
CopyScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID, targetName string, verifyDestination func(context.Context) error) (*Secret, error)
MoveScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID, targetName string, verifyDestination func(context.Context) error) (*Secret, error)
```

`CopyScoped` checks the target name and inserts the copy in one transaction;
`MoveScoped` additionally deletes the source in the same transaction. Both
return `ErrSecretNameConflict` when the target name is already taken and
`ErrNotFound` when the source row is absent. `UserVisibleStore` delegates and
rejects internal (`github:` prefix) source IDs. The encrypted value is copied
without passing plaintext through the service.

Store invariants for the transfer operations:

- The copied row is created with the caller's `user_id` (the same scoping the
  regular create path writes); a transfer never produces an unowned row.
- The source select and the Move source delete use the same per-user
  visibility predicate as the rest of the store, so a user can only move rows
  they can see.
- The target-name conflict check compares the normalized scope (a legacy
  empty stored scope is Global), so a legacy Global row can never be bypassed.
- The check-then-insert is atomic on both supported dialects: SQLite starts
  the transaction with `BEGIN IMMEDIATE` (writer lock up front), and
  PostgreSQL executes `BEGIN` then a `pg_advisory_xact_lock` on the target
  scope's lock key on that same transaction before the source select and
  conflict check, serializing competing writers so exactly one copy/move wins
  a same-target-name race under READ COMMITTED. The PostgreSQL statement never
  runs on SQLite. No schema or uniqueness-constraint change; pre-existing
  duplicate names remain allowed by the regular create path.
- Lock scheme (single authoritative contract, PostgreSQL only): the secrets
  package exports `WorkspaceLockKey(workspaceID string) int64`, a
  deterministic FNV-1a 64-bit hash of the `kandev-secret-transfer:` prefix
  plus the workspace ID — stable across restarts, collision-safe for UUID
  inputs (a theoretical hash collision only over-serializes, never corrupts)
  — plus a distinct stable constant `GlobalSecretTransferLockKey` for Global
  targets. A transfer acquires the destination scope's key (and the source
  workspace's key for Move, in sorted key order) and, while holding it, runs
  the destination existence/authorization verification inside the transaction
  before the conflict check and insert. The workspace-deletion transaction's
  secret-cleanup step (`DeleteWorkspaceSecretsTx`) acquires the same
  workspace key at its start and holds it to commit. Interleavings are
  therefore closed: a transfer that inserts before a deletion commits is
  cleaned up by that deletion under the same lock, and a transfer that starts
  after the deletion commits observes the workspace gone and fails closed. On
  SQLite the secrets database's single writer serializes transfer and
  cleanup; the residual cross-database window matches the existing create
  path (no schema change). A deterministic race test (separate connections,
  channel-scheduled) proves no secret remains attached to a deleted workspace,
  and a concurrent same-name test with a Global target proves exactly one
  winner on the Global lock.

## Permissions

- A Global source is visible to the caller through the existing global access
  rules; a Workspace source resolves only when the caller is authorized for
  that workspace (same `GetWorkspaceSecret` path as the scoped CRUD routes).
- A workspace destination requires both existence and authorization. The
  workspace authorizer is enforced for authenticated callers; a separate
  existence check runs even in auth-disabled/unscoped contexts so a copy/move
  can never attach a secret to a phantom workspace ID.
- Missing, unauthorized, or nonexistent sources/destinations all resolve to
  "not found" — the error never reveals whether the secret or workspace
  exists.
- Backend-owned internal secret IDs (`github:` prefix) are not addressable and
  resolve to "not found".

## Failure modes

- **Target name conflict:** the operation fails closed with `409 Conflict` and
  a message naming the scope and the conflicting name. The conflict check and
  the copy share one transaction, and competing transfers are serialized
  (writer lock up front on SQLite, advisory transaction lock on PostgreSQL), so
  concurrent requests with the same target name yield exactly one success and
  `409` for the others; nothing partial is created.
- **Same-scope destination:** rejected with a validation error before any
  transfer mutation (a read-only source resolution may already have happened).
- **Source missing or unauthorized:** `404`, nothing changes.
- **Destination missing or unauthorized:** `404`, nothing changes; existence is
  verified even when authorization is disabled. If the workspace-existence
  checker is not wired, workspace destinations fail closed (same `404`).
- **Move failure:** the copy and the source deletion are one transaction. Any
  failure (including a crash) rolls back; the source remains intact and no
  copy exists. There is no intermediate dual-state or compensation window.
- **Internal failure (decryption, storage, or a database error inside an
  authorization/existence lookup):** `500` with a sanitized message; full
  details are logged server-side. Values never appear in error payloads. Only
  the known "missing or unauthorized" sentinels (secret, workspace) map to
  `404`; unknown errors are never reclassified as client mistakes.
- **Destination workspace fetch failure (dialog pre-check):** the dialog falls
  back to the backend's authoritative `409`; the pre-check is best-effort only.
- **Workspace list failed to load (settings hydration failure):** the dialog
  shows a localized retry/failure state and disables submit instead of
  presenting an empty destination picker; a Global source must never see a
  form with zero valid destinations.
- **Workspace list loaded successfully but is empty:** the dialog shows a
  localized "no valid destinations" state (rather than an empty picker) and
  keeps submit disabled; a Global source has no valid target and must not be
  able to submit.
- **Source deleted or destination unavailable while the dialog is open:** the
  backend returns `404` (deliberately ambiguous between a missing source and a
  missing/unauthorized destination, so it never discloses which). The dialog
  shows a localized generic failure message and stays open; rows are never
  removed automatically on a `404`, and no destination item is added. The
  source list refreshes on the next navigation or reload.

## Scenarios

- **GIVEN** a Global secret named `API Key`, **WHEN** the user opens Copy/Move
  on it, chooses Copy, destination `My Workspace`, and keeps the default name
  `API Key (from Global)`, **THEN** a Workspace secret named
  `API Key (from Global)` with the same value appears in `My Workspace` and
  the Global secret remains.
- **GIVEN** a Workspace secret in `My Workspace`, **WHEN** the user chooses
  Move to General, **THEN** a Global secret with the chosen name exists with
  the same value and the Workspace secret is gone, and the Global store gains
  the returned item even though the operation started on the Workspace page.
- **GIVEN** a Global secret, **WHEN** the user chooses Move to `My Workspace`,
  **THEN** the Workspace secret is created and the Global secret is removed.
- **GIVEN** a Workspace secret in `A`, **WHEN** the user copies it to
  workspace `B`, **THEN** a Workspace secret in `B` exists with the same value
  and the secret in `A` remains.
- **GIVEN** a destination scope that already has a secret with the chosen
  name, **WHEN** the user submits Copy or Move, **THEN** the operation is
  blocked with `409`, the name field is marked invalid with an error, and
  neither the destination nor the source changes.
- **GIVEN** two concurrent copy requests with the same target name into an
  empty destination scope, **WHEN** both submit, **THEN** exactly one succeeds
  and the other receives `409`; the destination contains one secret.
- **GIVEN** a source secret and a destination equal to the source's own scope
  and workspace, **WHEN** the user submits, **THEN** a validation error is
  returned and nothing changes.
- **GIVEN** a caller without access to the destination workspace, **WHEN** the
  caller submits a copy/move into it, **THEN** the operation fails with `404`
  without disclosing the workspace's secrets.
- **GIVEN** a destination workspace ID that does not exist and authentication
  disabled, **WHEN** the caller submits a copy/move into it, **THEN** the
  operation fails with `404` and no secret is created.
- **GIVEN** an internal backend-owned secret ID (`github:` prefix), **WHEN**
  the caller submits a copy/move of it, **THEN** the operation fails with
  `404`.
- **GIVEN** a user on the Global secrets page who copies a secret to a
  workspace, **WHEN** the copy completes, **THEN** the Global list is
  unchanged and no workspace-scoped item is added to the Global store.
- **GIVEN** a user on a Workspace secrets page who moves a secret to General,
  **WHEN** the move completes, **THEN** the workspace list no longer contains
  the secret and the Global store gains it.
- **GIVEN** a Global source named with multibyte characters, **WHEN** the
  dialog computes the default target name, **THEN** the default is truncated so
  its UTF-8 length is at most 100 bytes and the copy succeeds without a
  backend validation error.

## Mobile design contract

- Entry point: the Copy/Move button on a secret row opens the same dialog on
  phone widths, from both the Global and Workspace secrets pages.
- Presentation: the dialog adapts to the phone viewport (full-height
  sheet/drawer behavior rather than a centered desktop box), keeps the
  Settings content area as the single scroll owner, respects safe areas, and
  restores focus on close; Escape and back navigation close it.
- Interaction: the Copy/Move radios, destination picker, and name field use
  touch-sized targets (at least 44px), rows stack to one column, and there is
  no horizontal overflow. The destination picker is operable by touch and
  keyboard alike.
- Accessibility: the dialog has a `DialogTitle`, real `Label`/`htmlFor`
  associations for the name and destination controls (never placeholder-only),
  a labeled Copy/Move radio group, conflict errors associated via
  `aria-describedby`, and keyboard-operable radios (Tab/Arrow/Space).
- Parity: phone users can complete the full copy and move flows, including
  choosing a workspace destination and correcting a conflicting target name.

## Out of scope

- Bulk copy/move of multiple secrets at once.
- Automatic conflict resolution (overwrite, auto-rename); conflicts always
  block and require the user to change the target name.
- Copying values into literal environment variables or non-secret storage.
- Changing scope in place: scope and workspace remain immutable; copy/move is
  the supported way to transfer authority.
- Live propagation of the copied value to running tasks or terminals.
