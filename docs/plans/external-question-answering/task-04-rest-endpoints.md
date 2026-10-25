---
id: "04-rest-endpoints"
title: "Clarification REST endpoints on ResolveBundle, with authorization"
status: done
wave: 4
depends_on: ["03-resolve-bundle"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-question-answering.md"
---

# Task 04: Clarification REST endpoints on `ResolveBundle`, with authorization

Rewire the four existing endpoints onto the resolver and add the ownership check they never had.
After this task the shipped product is already fixed: the double-answer bug is closed and a PAT
holder can no longer read or answer another user's question. The MCP tools are additive on top.

- **Acceptance:**
  1. `POST /:id/respond` returns `200 {"success":true,"claimed":…,"status":…,"response":…,"resume":…}`
     for both winner and loser, `400` for validation, `404` for an unknown or unauthorized bundle,
     and `500` naming the bundle when partially applied. No response path publishes a resume event
     for a loser.
  2. All four endpoints deny a caller who cannot access the owning task, with a `404` body
     indistinguishable from a fabricated `pending_id` and carrying no question text, option labels,
     task ID, session ID, or workspace ID (A2, A3).
  3. With auth disabled, all four endpoints behave exactly as before this change (A4).
  4. `POST /:id/cancel` succeeds for a bundle whose in-memory entry is gone but whose durable
     record is unresolved (X2), and still closes `CancelCh` when an entry exists (X3).

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/clarification internal/backendapp && \
    go test -race ./internal/clarification/... ./internal/backendapp/... && \
    go build ./... && \
    golangci-lint run ./internal/clarification/... --timeout=5m
  ```

- **Files likely touched:**
  - `apps/backend/internal/clarification/handlers.go` (`Handlers`/`NewHandlers`/`RegisterRoutes`
    gain the resolver; `httpRespond` collapses onto it; `httpCancelRequest`, `httpGetRequest`,
    `httpWaitForResponse` authorize; the event-fallback branch at `:326-336`, the
    `ErrAlreadyResponded` 409 at `:301`, and `applyAnswersToMessages` at `:447` are deleted)
  - `apps/backend/internal/backendapp/helpers.go:1135` (build and pass the resolver)
  - `apps/backend/internal/clarification/handlers_test.go`
  - `apps/backend/internal/clarification/handlers_authz_test.go` (new — revive's 800-effective-line
    file limit applies to test files, so authorization cases go in their own file)

- **Dependencies:** 03.
- **Parallelism:** `sequential` — shares `apps/backend/internal/backendapp/helpers.go` with task 05.

- **Inputs:**
  - Spec § *Authorization* (A1–A6), § *Cancel joins the same claim* (X1–X3), R8a,
    § *Failure modes*, and the **BREAKING CHANGE** note (the web UI is the only in-tree production
    caller and is covered by task 07).
  - Plan § *Backend → REST endpoints* for the exact response envelope and error mapping.
  - `httpCreateRequest` is explicitly out of scope (A6) — do not touch it.
  - Authorization tests must cover both auth modes; A4 is the compatibility guarantee, not an
    afterthought. Follow `internal/auth/authn.IdentityFromContext` and the synthetic-identity rule
    in `apps/backend/AGENTS.md` § *Opt-in authentication & per-user scoping*.
  - Reconcile **Files likely touched** with the actual diff before finishing.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented in commit `782dac9af`. `Handlers` gained the `*Resolver`; `httpRespond` collapsed onto
`ResolveBundle` and now returns the uniform `{success, claimed, status, response, resume}` envelope
for both winner and loser; `httpCancelRequest`, `httpGetRequest`, and `httpWaitForResponse` now
authorize before reading; the event-fallback branch, the `ErrAlreadyResponded` 409, and
`applyAnswersToMessages` were deleted (their behavior moved into the resolver in task 03).
`internal/backendapp/helpers.go` builds and passes the resolver.

Verified in this session's final gauntlet (Wave 10): `go test ./internal/clarification/...
./internal/backendapp/...` passes, including `handlers_authz_test.go` (all four endpoints denying a
caller who cannot access the owning task, with a 404 body identical to a fabricated `pending_id`, in
both auth-enabled and auth-disabled modes) and the cancel-with-no-in-memory-entry case (X2).
`gofmt -l` reports no files; `go build ./...` succeeds; `golangci-lint run
./internal/clarification/... --timeout=5m` is clean.

No external side effects. The response envelope is a breaking change to the REST contract, but the
in-tree web client is the only production caller and was updated in task 07.
</content>
