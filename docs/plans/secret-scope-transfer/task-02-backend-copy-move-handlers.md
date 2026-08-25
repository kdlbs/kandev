---
id: "02-backend-copy-move-handlers"
title: "Backend copy/move HTTP + WS handlers"
status: done
wave: 2
depends_on: ["01-backend-copy-move-service"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/secret-scope-transfer.md"
---

# Task 02: Backend Copy/Move Handlers

## Acceptance

- `POST /api/v1/secrets/:id/copy` and `POST /api/v1/secrets/:id/move` exist
  with the spec's request body (`target_scope`, `target_workspace_id`, optional
  `name`) and the existing `?workspace_id=` source-scoping pattern. Success:
  `201 Created` + new `SecretListItem`.
- Error mapping is exact, for both HTTP and WS, classified exclusively with
  `errors.Is` against package sentinels (never by matching error text):
  - `errors.Is(err, secrets.ErrSecretValidation)` (payload, scope/workspace
    combination, same-scope, invalid or explicitly-empty name) → `400` /
    `ErrorCodeBadRequest`;
  - `errors.Is(err, secrets.ErrNotFound)` (missing source, missing secret) or
    `errors.Is(err, secrets.ErrWorkspaceAccessDenied)` (missing/unauthorized
    destination, unconfigured existence checker) → `404` /
    `ErrorCodeNotFound`;
  - `errors.Is(err, secrets.ErrSecretNameConflict)` → `409` /
    `ErrorCodeConflict`;
  - any other failure → `500` / `ErrorCodeInternalError` with a sanitized
    message (details logged server-side, never in the response). Raw
    database/storage errors inside the source lookup, the authorizer, or the
    existence checker must stay in this bucket, not become `404`.
- WS actions `secrets.copy` and `secrets.move` are registered with payload
  `{id, workspace_id, target_scope, target_workspace_id, name}` (`name` with
  the same presence semantics as HTTP). The WS envelope decoder must delegate
  the transfer fields to the same presence-aware `CopySecretRequest` decoder
  used by HTTP (a flat anonymous struct with `json.Unmarshal` would silently
  collapse `name: null` into omitted); it resets the presence marker and all
  fields before every decode. Success responses return the new
  `SecretListItem` as the WS payload, mirroring the existing
  `secrets.create` convention (`ws.NewResponse(..., item)`); response-shape
  tests assert the same fields as the HTTP response, including exact target
  identity: `Scope` equals `target_scope`, `WorkspaceID` equals
  `target_workspace_id` (empty for Global), and `Name` equals the target
  name.
- Handler tests (gin/httptest) cover: success (201), conflict (409),
  validation (400), missing source (404), unauthorized source (404),
  unauthorized destination (404), nonexistent destination with the existence
  checker denying (404), unconfigured (nil) existence checker (404), and
  injected internal failures from the source lookup, the authorizer, and the
  existence checker (each `500` with a sanitized body and no raw error text),
  including an injected internal error whose message contains validation-like
  text that must still be `500`, plus the matching WS error codes for every
  case. HTTP and WS tests cover all four `name` JSON cases: omitted (uses the
  source name), `null` (`400`/`BAD_REQUEST`), empty string (`400`), and
  whitespace-only (`400`). WS tests additionally cover the source-scope rule:
  omitted/empty `workspace_id` selects a Global source (success path) and a
  Workspace source without `workspace_id` resolves to `NOT_FOUND`, matching
  the HTTP `?workspace_id=` semantics. A reused-request WS test reuses one
  envelope across two messages: the first targets workspace A with a non-empty
  source workspace and `name: null` (asserting `BAD_REQUEST`), the second
  targets General while omitting both workspace IDs and the name, asserting
  the second dispatch uses empty target/source workspace IDs, Global scope,
  the second source ID, and the default source name (full field + presence
  reset between decodes, not just the name marker).

## Files likely touched

- `apps/backend/internal/secrets/handlers.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/secrets/handlers_test.go` (new; follow
  `internal/agent/settings/handlers/agent_update_handlers_test.go` gin
  conventions, or `sqlite_store_test.go` fixtures for the store)

## Inputs

- Task 01's `Service.Copy`/`Move`, `ErrSecretNameConflict`, and
  `ErrNotFound`-wrapping contract.
- Existing handler patterns: `httpCreateSecret`, `deleteSecretForWorkspace`
  (query-param dispatch), `wsCreate`/`wsDelete` (payload parse + error codes).

## Dependencies

Task 01.

## TDD sequence

1. Write failing handler tests for the full status/error-code matrix above,
   including an authorizer that denies source and destination independently,
   an existence checker that denies a phantom workspace, a nil existence
   checker, and injected internal errors (raw non-sentinel errors) from each
   callback and the store.
2. Add `ActionSecretCopy`/`ActionSecretMove` constants, HTTP routes, WS
   handlers, and the classification helper (validation / not-found /
   workspace-denied / conflict / internal).
3. Run the secrets and websocket package tests until green.

## Verification

```bash
cd apps/backend && go test ./internal/secrets/... ./pkg/websocket/...
```

## Risks

- The source `workspace_id` query parameter must dispatch to the workspace
  source resolution only when present, matching the existing CRUD dispatch.
- Never map internal failures to `400` or `404`: only the known sentinels
  (`ErrNotFound`, `ErrWorkspaceAccessDenied`) are not-found; raw database or
  storage errors stay sanitized `500`.
- WS error-code mapping must use the existing `ws.ErrorCode*` constants.
- Unauthorized and missing must be indistinguishable in responses (both `404`),
  and `500` responses must never echo the underlying error string.

## Output contract

Report the routes/actions added, the classification helper, test names, and
verification output.
