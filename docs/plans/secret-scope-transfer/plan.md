---
spec: docs/specs/workspaces/requirements/secret-scope-transfer.md
created: 2026-08-11
status: completed
---

# Implementation Plan: Copy and Move Secrets Between Scopes

## Overview

Add copy/move operations that transfer an encrypted secret between Global and
Workspace scopes (and between workspaces) without exposing the value. The
service authorizes and validates, then delegates to new atomic store
operations (`CopyScoped`/`MoveScoped`) so name-conflict detection and move
deletion share one transaction. The frontend adds a shared Copy/Move dialog to
the existing `SecretsSettings` rows on both the Global and Workspace pages. No
schema changes: scope and `workspace_id` remain immutable.

Design decisions hardened by adversarial review rounds 1-2: conflict checks
and moves are transactional and dialect-serialized (no TOCTOU race, no
compensation window, exactly one winner under concurrency), only the known
missing/unauthorized sentinels resolve to `404` while raw internal failures
stay sanitized `500`s, the name limit is defined as UTF-8 bytes, destination
workspace existence is verified even in auth-disabled mode and fails closed
when unconfigured, copied rows keep the caller's ownership, legacy empty-scope
Global rows still conflict, the Global store always receives Global-target
results, the default target-name suffix uses locale-independent origin tokens,
and the destination payload is cleared when switching to General.

## Backend

### Service layer (`apps/backend/internal/secrets`)

- Add `CopySecretRequest` (request DTO with `target_scope`,
  `target_workspace_id`, optional `name`) and the `ErrSecretNameConflict`
  sentinel next to `ErrNotFound` in `store.go` or `models.go`.
- Add `Service.Copy(ctx, sourceID, sourceWorkspaceID, req)` and
  `Service.Move(...)`:
  1. Resolve the source: `sourceWorkspaceID == ""` → `Get` (Global only);
     non-empty → `GetWorkspaceSecret` (authorized Workspace only). Store-level
     missing rows surface as `ErrNotFound`; other errors pass through
     unclassified (handlers turn only known sentinels into `404`).
  2. Normalize the target name with presence semantics: `req.Name` is a
     `*string`; JSON-omitted means "use the source name", explicit `null`,
     present-but-empty, or whitespace-only is a validation error, and a
     present name enforces the existing 1-100 UTF-8 byte rule. Because Go's
     standard decoding maps omitted and `null` to the same nil pointer, the
     request type implements a custom `UnmarshalJSON` that records `name`
     presence, so the service can tell them apart.
  3. Validate the target with `validateSecretScope` and reject a destination
     equal to the source scope+workspace. Every validation failure is wrapped
     with the `ErrSecretValidation` sentinel so handlers classify with
     `errors.Is`, never by message text.
  4. Authorize the destination: `authorizeScope` for authenticated callers
     plus a workspace-existence callback that runs unconditionally (including
     auth-disabled contexts). Missing/unauthorized destinations surface as the
     secrets sentinel `ErrWorkspaceAccessDenied`; raw lookup/storage errors
     inside the callbacks pass through unclassified so they stay `500`.
     Destination verification is passed into the store operation as an in-tx
     callback and runs while the transfer transaction holds the destination's
     advisory lock (see store layer), so it is inside the lock boundary.
  5. Call `store.CopyScoped`/`store.MoveScoped` (with the verification
     callback) and wrap the result. Do not reveal or re-encrypt plaintext in
     the service.
- Add the sentinels `ErrWorkspaceAccessDenied` and `ErrSecretValidation` next
  to `ErrNotFound` and `ErrSecretNameConflict`. Classification uses
  `errors.Is` exclusively; no handler matches error text.
- Add `SetWorkspaceExistenceChecker(func(context.Context, string) error)`.
  A nil checker fails closed: any workspace destination resolves as
  `ErrWorkspaceAccessDenied` (never a silent pass-through for phantom
  workspaces).
- Classification happens at the wiring boundary (`backendapp/main.go`), which
  can import both packages: adapters wrap the task service's
  `AuthorizeWorkspaceAccess` and workspace-existence lookup so that
  `errors.Is(err, repoerrors.ErrWorkspaceNotFound)` becomes
  `secrets.ErrWorkspaceAccessDenied`, while any other error (e.g. a database
  failure) is returned unchanged for the `500` classifier. The secrets service
  never imports `repoerrors`.
- Do not touch shared methods (`Get`, `authorizeScope`, CRUD); wrap only at
  the Copy/Move boundary.
- Tests in a new `copy_move_test.go` using `newTestSQLiteStore` wrapped in
  `NewUserVisibleStore` (so the internal-ID boundary is real): copy
  global→workspace, workspace→global, workspace A→workspace B; move both
  directions; omitted-name default; name validation (empty, whitespace, >100
  bytes with multibyte input); same-scope rejection; name conflict (Global and
  workspace targets); unauthorized/missing source; unauthorized destination;
  nonexistent destination with authorizer disabled (existence checker denies);
  internal `github:` source ID seeded as a real row → not-found; list
  consistency after each operation; values verifiable only through reveal.

### Store layer (`apps/backend/internal/secrets`)

- Extend `ScopedSecretStore` with callback-bearing transfer methods:
  - `CopyScoped(ctx, sourceID, targetScope, targetWorkspaceID, targetName, verifyDestination func(context.Context) error) (*Secret, error)`
  - `MoveScoped(ctx, sourceID, targetScope, targetWorkspaceID, targetName, verifyDestination func(context.Context) error) (*Secret, error)`
  `verifyDestination` runs inside the transaction while the destination lock
  is held, before the conflict check and insert; `UserVisibleStore` passes it
  through; an error rolls back and surfaces for the service's 404 mapping.
- `sqliteStore` (serves SQLite AND PostgreSQL through `Provide`): one
  transaction per operation, atomic on both dialects:
  - `CopyScoped`: BEGIN with a defined lock contract; select the source row
    with the existing per-user visibility predicate; verify no row with the
    target `(name, normalized scope, workspace_id)` visible to the caller,
    where an empty stored scope normalizes to Global (else ROLLBACK +
    `ErrSecretNameConflict`); INSERT the copy with a new ID, the source's
    encrypted value/nonce, the target scope/workspace, AND
    `user_id = scopeOwner(ctx)` (never an unowned row); COMMIT.
  - Lock contract (dialect-gated, defined in the store): SQLite starts the
    transaction with `BEGIN IMMEDIATE` (writer lock up front); PostgreSQL
    executes `BEGIN`, then `SELECT pg_advisory_xact_lock(<target scope key>)`
    on that same transaction BEFORE the source select and conflict check,
    serializing competing writers under READ COMMITTED. The PostgreSQL
    statement never runs on SQLite. One authoritative lock scheme: exported
    `WorkspaceLockKey(workspaceID string) int64` (deterministic FNV-1a 64-bit
    of the `kandev-secret-transfer:` prefix + workspace ID; stable across
    restarts; collision-safe for UUIDs — a collision only over-serializes)
    plus a distinct stable `GlobalSecretTransferLockKey` for Global targets.
    A Move additionally locks the source workspace key; multi-key acquisitions
    happen in sorted key order.
  - Shared deletion boundary: a workspace-scoped secret can never outlive its
    workspace. Locks are workspace-keyed, not global: the secrets package
    exports a deterministic `WorkspaceLockKey(workspaceID)` (pure function,
    no new package cycle — the task service already consumes secrets
    interfaces) used by both sides on PostgreSQL. The transfer transaction
    acquires the destination workspace's key (plus the source workspace's key
    for Move, in sorted order) and, while holding it, runs the destination
    existence/authorization verification via an in-tx callback (wired to the
    service's existence checker) before the conflict check and insert.
    `DeleteWorkspaceSecretsTx` acquires the same key at its start and holds it
    to commit, so a transfer that inserts before a deletion commits is cleaned
    up by that deletion, and a transfer after the deletion commits observes
    the workspace gone. SQLite's single writer serializes transfer and
    cleanup; the residual cross-database window matches the existing create
    path. A deterministic PostgreSQL race test (delete vs transfer on
    separate connections, channel-scheduled) asserts no secret remains
    attached to the deleted workspace.
  - Transaction boundary: every statement in the operation — the source
    select, the conflict check, the insert, the Move source delete, and the
    returned-row read — executes on the transaction-bound executor (a pinned
    writer connection), never on the store's reader handle (`s.ro`) or a
    second connection. Lock-acquisition failure rolls back and surfaces as a
    store error (sanitized `500` upstream).
  - Return contract: both methods return the inserted row's metadata (ID,
    name, scope, workspace, `created_at`, `updated_at`) via `RETURNING` where
    portable or a same-transaction select; a zero-value struct is not a valid
    result.
  - `MoveScoped`: same plus DELETE the source row using the same visibility
    predicate as the source select, before COMMIT. Missing source →
    `ErrNotFound`; any failure rolls back (source intact, no copy).
- `UserVisibleStore`: delegate both, and reject internal (`github:` prefix)
  source IDs up front.
- Every compile-time `ScopedSecretStore` implementer gains the two methods:
  `sqliteStore`, `UserVisibleStore`, and the lifecycle test double
  `inMemorySecretStore` (`internal/agent/runtime/lifecycle/manager_auth_test.go`).
- No schema change; no uniqueness constraint (existing create behavior allows
  duplicate names; the serialized transaction provides transfer atomicity).
- Tests (`transfer_test.go`, SQLite by default): copy keeps source, move
  removes source, conflict rolls back to no-op, copied row is owned
  (`user_id = scopeOwner(ctx)`) with user A/user B ownership boundaries,
  legacy empty-scope target conflicts with Global, per-user scoping,
  UserVisibleStore internal-ID rejection, and a deterministic
  concurrent-same-target-name test using a one-way schedule (transfer A
  signals "lock acquired"; the test asserts B is still blocked on the lock;
  A completes; B proceeds and receives `ErrSecretNameConflict`) — the schedule
  never requires both transfers to hold the lock at once.
- Environment-gated PostgreSQL tests (existing `PostgresDSNFromEnv` pattern):
  copy, Move rollback, scoped conflict detection, copied-row ownership,
  legacy empty-scope conflict, and the concurrent same-name race under READ
  COMMITTED with the advisory lock.

### HTTP + WebSocket handlers (`handlers.go`, `apps/backend/pkg/websocket/actions.go`)

- Add `ActionSecretCopy = "secrets.copy"` and `ActionSecretMove =
  "secrets.move"` to `actions.go` next to the existing secret actions.
- Add `POST /api/v1/secrets/:id/copy` and `POST /api/v1/secrets/:id/move`
  routes using the existing `?workspace_id=` source-scoping dispatch. Success:
  `201 Created` with the new `SecretListItem`.
- Error mapping (HTTP and WS), classified exclusively via `errors.Is` against
  the package sentinels — never by matching error text:
  - `errors.Is(err, secrets.ErrSecretValidation)` (payload, scope/workspace
    combination, same-scope, invalid/empty name) → `400` /
    `ErrorCodeBadRequest`;
  - `errors.Is(err, secrets.ErrNotFound)` (missing source, missing secret) or
    `errors.Is(err, secrets.ErrWorkspaceAccessDenied)` (missing/unauthorized
    destination, unconfigured existence checker) → `404` /
    `ErrorCodeNotFound`;
  - `errors.Is(err, secrets.ErrSecretNameConflict)` → `409` /
    `ErrorCodeConflict`;
  - everything else → `500` / `ErrorCodeInternalError` with a sanitized
    message; details logged server-side.
- Handler tests with gin/httptest (conventions from
  `internal/agent/settings/handlers/agent_update_handlers_test.go`): success
  (201), conflict (409), validation (400), missing source (404), **unauthorized
  source (404)**, **unauthorized destination (404)**, **nonexistent
  destination with the existence checker denying (404)**, **unconfigured
  (nil) existence checker denying (404)**, and injected internal failures from
  the source lookup, the authorizer, and the existence checker (each `500`,
  sanitized body, no raw error text), plus matching WS error-code mapping for
  every case.

## Frontend

### API and types

- `apps/web/lib/types/http-secrets.ts`: add `CopyMoveSecretRequest`
  (`target_scope`, `target_workspace_id`, optional `name`).
- `apps/web/lib/api/domains/secrets-api.ts`: add `copySecret(id, payload,
  options?)` and `moveSecret(id, payload, options?)` POST helpers reusing
  `withSecretQuery` for the source `workspace_id` (options include
  `workspaceId` for the source scope). Surface the HTTP status so the dialog
  can distinguish `409` (name field) from other errors.
- Add API tests in `secrets-api.test.ts` matching the existing fetch mocking
  convention.

### Dialog and row wiring

- Extract sections from the 640-line `secrets-settings.tsx` (the frontend
  limit is 600 lines) before adding wiring: move `SecretListItemRow`,
  `SecretForm`, and the delete dialog into focused components. Keep
  `secrets-settings.tsx` under the limit.
- New `apps/web/components/settings/copy-move-secret-dialog.tsx`:
  - Props: `secret`, `originToken` (`general` for Global sources, the raw
    workspace name for Workspace sources), `onClose`,
    `onCompleted(item, mode)`.
  - The dialog's primary submit label reflects the selected mode: distinct
    localized keys (`copySecret` / `moveSecret`) or one interpolated mode
    label, switching when the radio changes (never a static "Copy" button in
    Move mode). The Move warning accompanies the Move label.
  - Copy/Move radio (default Copy), destination `Select` (General + workspaces
    from `useAppStore((s) => s.workspaces.items)`; exclude the source's own
    scope; exclude General when the source is Global), editable name `Input`
    pre-filled via the translated pattern `{{name}} (from {{origin}})` with the
    literal origin token.
  - Workspace hydration failure is handled explicitly: the destination hook
    exposes loading/error state, the dialog shows a localized retry/failure
    state instead of an empty picker (which would leave a Global source with
    zero valid destinations), and the picker is never presented as a valid
    transfer form when the workspace list failed to load.
  - Mobile presentation is an implementation requirement, not just a spec
    goal: the dialog uses a responsive presentation (a Drawer/sheet at phone
    width or explicit `DialogContent` overrides) with safe-area padding, 44px
    touch targets for the radios, destination picker, name input, actions,
    and the close affordance, and focus restoration on close. The scroll
    invariant is a single owner: the Settings content area
    (`settings-scroll-container`) remains the only vertical scroller, and the
    dialog is constrained to fit without nested vertical scrolling
    (content-sized, with the picker list scrolling only when it cannot fit).
    The shared `Dialog`/`Select` defaults are desktop-oriented and must be
    overridden, not inherited.
  - The request payload is modeled as a discriminated union with a payload
    builder: `name?: string` (undefined = omitted, no nullable branch),
    `target_workspace_id` cleared when the destination is General (never
    submits `global` with a stale workspace ID, which the backend rejects),
    and `name: null` never emitted. An API test asserts the serialized bodies
    for a string name and an omitted name contain no `"name": null`.
  - Default-name truncation: trim so the UTF-8 byte length is at most 100
    (code-point aware), never relying on `String.prototype.length` semantics.
  - Move mode shows an inline warning that the original is removed from the
    source scope.
  - Best-effort conflict pre-check: a small hook loads the destination scope's
    names and marks the name field `aria-invalid` with an inline error on a
    trimmed match; submit is disabled. Backend `409` also surfaces on the name
    field; other backend errors show a generic message.
  - Submit calls `copySecret`/`moveSecret`; on success calls `onCompleted`.
- New `apps/web/hooks/domains/settings/use-secret-destination-names.ts`
  (testable hook): given a target scope+workspace, loads existing secret names
  (`useSecrets("global")` for Global; `listSecrets({scope:"workspace",
  workspaceId})` fetched on demand for Workspace, cached per workspace) and
  returns `{ names, loaded, conflict(name) }`.
- Workspace-list state with a real retry path and a single source of truth:
  the destination picker reads the workspace store only. A dedicated hook
  exposes `{ loading, error, retry }`: when the store's workspace list is
  empty or hydration failed, it fetches `listWorkspaces` on demand and writes
  successful results into the store via `setWorkspaces` (with
  request-generation/abort protection so a stale response never overwrites a
  newer hydration). `retry` re-runs that fetch; failures are preserved as
  `error`, never converted into `items: []`. A later hydration/store update
  simply supersedes the fallback result. This covers the Workspace page too
  (current workspace known, other-workspace list failed), and hydration
  arriving mid-fallback is tested.
- `apps/web/components/settings/secrets-settings.tsx`:
  - Add a Copy/Move button to the (extracted) row component with an accessible
    label and touch-sized target; track the dialog target in state.
  - `onCompleted` routes **by the returned item's scope, never the page scope**:
    - target Global → `useAppStore` `addSecret` directly, from any page;
    - target Workspace → the page's scoped `addSecret` only when the page is
      that workspace's page;
    - `mode === "move"` → remove the source via the page's own scoped
      `removeSecret`.
- i18n: new keys in `apps/web/src/locales/en/settings.json` (Copy/Move label,
  dialog title, Copy/Move radio labels, destination label, General option,
  name label, conflict error, move warning, submit labels, origin suffix
  pattern, destination-list loading state, workspace-list failure state, retry
  action, submit-disabled hint), mirrored in pseudo, pt-pt, and zh-cn. No em
  dashes; `t()` at render time only; origin tokens are values, never catalog
  keys.
- Component tests: dialog rendering, destination filtering (source Global vs
  Workspace), default-name construction with literal origin tokens, byte-aware
  truncation (ASCII and multibyte), conflict pre-check blocking, move warning,
  submit payloads, destination switching Workspace → General asserting the
  request carries no workspace target, `409` surfacing on the name field, and
  `onCompleted` routing (workspace page → Global target adds to the store).
  Hook tests for destination-name loading and `conflict(name)`.

## E2E and documentation

- Desktop Chromium spec `apps/web/e2e/tests/settings/secrets-copy-move.spec.ts`
  (conventions from `repository-secrets.spec.ts`): create a Global secret,
  copy it to a workspace with the default suffixed name, verify both lists;
  move a workspace secret to General and verify the source is gone; attempt a
  conflicting target name and verify the blocked state. Add negative plaintext
  assertions after each flow (`expect(page.locator("body")).not.toContainText(
  <known value>)`), mirroring `repository-secrets.spec.ts`, so a value leak
  fails the suite.
- Mobile coverage in the `mobile-chrome` project (extend
  `mobile-repository-secrets.spec.ts` or add a sibling spec): open the dialog
  at phone width, exercise the destination picker (including switching
  Workspace → General), complete one copy and one move, and assert the mobile
  design contract: ≥44px touch targets (mirroring the existing mobile spec's
  bounding-box assertions), no horizontal overflow, single scroll owner, and
  focus return on close.
- Make the desktop describe/title contain the literal `secrets-copy-move`
  fragment so the `--grep` verification command actually selects the tests;
  document the direct spec-path run as an alternative.
- Docs: add a short copy/move paragraph to the secret-scope section of
  `docs/public/agents-and-profiles.md` (after the Workspace secrets paragraph);
  add the new spec row to `docs/specs/INDEX.md` under `workspaces/`.
- i18n checks: `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.

## Implementation waves

Wave 1:

- [x] [Task 01: Backend copy/move service and store](task-01-backend-copy-move-service.md)

Wave 2:

- [x] [Task 02: Backend copy/move handlers](task-02-backend-copy-move-handlers.md)
  (depends on Task 01)

Wave 3:

- [x] [Task 03: Frontend copy/move dialog and API](task-03-frontend-copy-move-dialog.md)
  (depends on Tasks 01 and 02)

Wave 4:

- [x] [Task 04: E2E and documentation](task-04-e2e-and-docs.md)
  (depends on Task 03)

The primary conversation executes these tasks sequentially unless the user
explicitly authorizes native implementation subagents.

## Risks

- The Global store slice must never receive workspace-scoped items, and every
  Global-target result must reach the store even from a Workspace page: the
  completion handler routes by the returned item's scope, not the page scope.
- Name-conflict checks are best-effort client-side; the transactional
  `CopyScoped`/`MoveScoped` check is the contract and must remain
  authoritative.
- The name limit is UTF-8 bytes on both sides: Go `len()` counts bytes, so the
  dialog's truncation must be byte-aware, not `String.prototype.length`.
- Workspace existence must be enforced even when the authorizer is a no-op
  (auth-disabled); the existence checker is a separate callback.
- `ScopedSecretStore` gains two methods; the compile-time implementers are
  `sqliteStore`, `UserVisibleStore`, and the lifecycle test double
  `inMemorySecretStore` (all three must be updated or the build breaks).
- The in-transaction conflict check is only atomic on both dialects with the
  explicit serialization (SQLite `BEGIN IMMEDIATE`, PostgreSQL advisory
  transaction lock); without it PostgreSQL READ COMMITTED allows duplicate
  inserts under concurrency.
- The copied row must be written with `user_id = scopeOwner(ctx)` and the
  source delete must reuse the visibility predicate; missing either turns a
  user-scoped transfer into an ownership or cross-user leak.
- Handler error mapping must keep only the known sentinels
  (`ErrNotFound`, `ErrWorkspaceAccessDenied`) at `404` and never expose
  internal error text; unknown errors, including database failures inside the
  authorizer/existence callbacks, stay sanitized `500`. Validation
  classification uses `ErrSecretValidation` via `errors.Is`; a raw internal
  error whose text resembles a validation message must still be `500`.
- The existence checker must fail closed when nil; an unconfigured service
  must not accept phantom workspace destinations.
- The PostgreSQL advisory lock must be taken on the same transaction before
  the conflict read with a stable named key, or the READ COMMITTED race
  returns; the SQLite `BEGIN IMMEDIATE` path must never execute the PostgreSQL
  statement.
- i18n ratchet: any touched line in `secrets-settings.tsx`/new files must use
  `t()`; new keys must exist in all four locales; origin tokens are values.
- `secrets-settings.tsx` must stay under the 600-line lint limit; extraction
  happens before new wiring.

## Completion verification

- Backend: `cd apps/backend && go test ./internal/secrets/... ./pkg/websocket/...`;
  then `make -C apps/backend lint` and `make -C apps/backend build`.
- Frontend: `cd apps && pnpm --filter @kandev/web test -- --run`, plus
  `cd apps/web && pnpm run typecheck`, `cd apps && pnpm --filter @kandev/web
  lint`, and `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`.
- E2E: run the managed build-and-run form `pnpm e2e:run` for the new specs
  (desktop `--grep="secrets-copy-move"` since the describe/title contains the
  fragment, plus the mobile-chrome grep for the mobile flow), or explicitly
  rebuild the backend binary and `apps/web` production bundle before
  `pnpm e2e:raw` — `e2e:raw` launches the existing `apps/backend/bin/kandev`
  and serves the existing `apps/web/dist`, which can be stale otherwise.
- Docs: public-doc markdown lint (if gated in CI) and `docs/specs/INDEX.md`
  row present.
