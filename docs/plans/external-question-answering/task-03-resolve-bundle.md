---
id: "03-resolve-bundle"
title: "ResolveBundle: the single authorized, idempotent resolution"
status: done
wave: 3
depends_on: ["01-resolutions-table", "02-bundle-queries"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-question-answering.md"
---

# Task 03: `ResolveBundle` — the single authorized, idempotent resolution

The deliverable of this spec. One service operation that authorizes, validates, claims, applies,
and publishes — in that order — so a second answerer can never overwrite the transcript or trigger
a second agent turn. No caller is rewired here; tasks 04 and 05 do that.

- **Acceptance:**
  1. Two concurrent `ResolveBundle` calls on one `pending_id` against a shared real database yield
     exactly one `claimed=true`, one row, one published event, and a loser carrying the winner's
     stored `status`, `response`, and `resume` (R1, R2, R3).
  2. Clearing the in-memory store between two calls does not make the second one a winner (R6).
  3. Every validation failure (N6, N7, N8, N8a, N8b) returns before the claim, writes no row, and
     leaves the bundle answerable (N8c).
  4. A per-question message-update failure returns `ErrPartiallyApplied`, publishes nothing, and
     leaves the row in place (R5).
  5. A caller who cannot access the bundle's task gets `ErrBundleNotFound`, identical to a
     fabricated `pending_id` (A1, A3, A5); an unscoped caller is authorized for every bundle (A4).

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/clarification && \
    go test -race ./internal/clarification/... && \
    go build ./... && \
    golangci-lint run ./internal/clarification/... --timeout=5m
  ```

- **Files likely touched:**
  - `apps/backend/internal/clarification/resolver.go` (new — `Resolver`, `Outcome`, `OutcomeKind`,
    `Resolution`, `ErrBundleNotFound`, `ErrValidation`, `ErrPartiallyApplied`)
  - `apps/backend/internal/clarification/validate.go` (new — `validateRespondAnswers` moved out of
    `handlers.go:348`, extended with the option-ID check (N8) and the 2000-character caps (N8b))
  - `apps/backend/internal/clarification/resolver_test.go` (new)
  - `apps/backend/internal/clarification/resolver_validation_test.go` (new)
  - `apps/backend/internal/clarification/resolver_concurrency_test.go` (new)

- **Dependencies:** 01 (claim row), 02 (bundle reads).
- **Parallelism:** `sequential`.

- **Inputs:**
  - Spec § *The resolution claim* (steps 1–5), R1–R9, R8a, A1–A5, N6–N9, X1–X3, D2,
    § *Failure modes* in full.
  - Plan § *Backend → Service* for the type signatures, the `OutcomeKind` decision, and the
    pessimistic-then-upgrade `resume` write ordering.
  - Behavior to preserve verbatim, moved rather than rewritten: `buildAnswerSummary`
    (`handlers.go:788`), `formatAnswerBody`'s `(no answer)` (`:835`, N7), the event payload shapes in
    `publishPrimaryAnsweredEvent` / `respondViaEventFallback` / `publishStaleDismissedEvent` /
    `publishCancelledEvent`, and `resolveClarificationEventContext`'s in-store-then-durable fallback.
  - Declare narrow local interfaces for the repository, the task authorizer
    (`(*service.Service).AuthorizeTaskAccess`, `task/service/service_access.go:103`), the message
    creator, and the event bus. `internal/clarification` must not import `internal/task/service`.
  - Concurrency coverage must run against a real shared database — the claim is a database
    constraint and a mocked store proves nothing (spec § *Verification notes*).
  - Go limits apply: ≤80 lines and ≤50 statements per function. `ResolveBundle` will need helper
    extraction for step 5.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented across commits `aea228b65`, `2018e148f`. Added `internal/clarification/resolver.go`
(`Resolver.ResolveBundle` — authorize, validate, claim-insert, and only the winner applies
per-question updates and publishes, in that order) and `internal/clarification/validate.go`
(`validateRespondAnswers` moved from `handlers.go` with the option-ID check and 2000-character caps
added). Declared narrow local interfaces for the repository, task authorizer, message creator, and
event bus so `internal/clarification` does not import `internal/task/service`.

Verified in this session's final gauntlet (Wave 10): `go test ./internal/clarification/...` passes,
including `resolver_test.go` (the full `ResolveBundle` behavior matrix — winning/losing claims, live
waiter delivery, partial-application failure, resume-reported-despite-persist-failure, and more),
`resolver_restart_test.go` (R6: a cleared/restarted in-memory `Store` does not let a second caller win
an already-durably-claimed resolution), and `outcome_validation_test.go` (every N6/N7/N8/N8a/N8b
validation case rejected pre-claim with no row written). Concurrency is proved one layer down, against
a real shared database rather than a mocked store, by
`internal/task/repository/sqlite/clarification_resolution_test.go`'s
`TestInsertClarificationResolution_ConcurrentGoroutines_ExactlyOneWinner`: concurrent claim-insert
calls against a real shared SQLite database yield exactly one winner and one row. `gofmt -l` reports
no files; `go build ./...` succeeds; `golangci-lint run ./internal/clarification/... --timeout=5m`
is clean.

No external side effects.
</content>
