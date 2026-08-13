---
id: "04-disposition-endpoint"
title: "Disposition PATCH endpoint"
status: done
wave: 3
depends_on: ["01-schema-and-activation"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 04: Disposition PATCH endpoint

Expose `PATCH /api/v1/github/task-prs/:associationId/disposition` so a human can
record why a pull request ended the way it did, with validation that refuses
every ambiguous or self-referential body and writes nothing on rejection.

- **Acceptance:**
  1. A valid body persists `disposition`, persists `superseded_by_url` when
     supplied, sets `disposition_recorded_at` to the current UTC instant, and
     returns the updated association. A `null` disposition clears all three
     columns in one statement (AC-20, AC-21, AC-22).
  2. HTTP 400 with nothing written for: a disposition outside the five permitted
     values; a `superseded_by_url` while the resulting disposition is not
     `superseded`; an unparseable PR URL (reusing `parsePRURL` and wrapping
     `ErrInvalidPRURL`); a URL resolving to the association's own
     `(owner, repo, number)` (AC-23, AC-24, AC-25, AC-26). HTTP 404 with no
     distinction between missing and cross-workspace (AC-28).
  3. Accepted on a detached association and on any `state` (AC-27, AC-29b). A
     changing write publishes `github.task_pr.updated`; an identical re-PATCH
     publishes nothing and does not advance `disposition_recorded_at` (AC-29).
     All eight columns appear on the task-PR JSON returned by the existing
     endpoints and carried by the event (AC-30). A disposition expvar counter
     increments (AC-38).

- **Verification:**
  ```
  cd apps/backend && gofmt -l internal/github && \
    go test ./internal/github/... && \
    make lint
  ```

- **Files likely touched:**
  - `apps/backend/internal/github/service_task_pr_disposition.go` (new) —
    `ErrInvalidDisposition`, `SetTaskPRDisposition`.
  - `apps/backend/internal/github/controller.go` — route registration beside the
    existing task-PR routes.
  - `apps/backend/internal/github/handlers.go` — `httpSetTaskPRDisposition`,
    error-to-status mapping.
  - `apps/backend/internal/github/controller_task_pr_disposition_test.go` (new)
  - `apps/backend/internal/github/store_task_pr_disposition_test.go` (new) — the
    disjoint-writer pinning test.
  - `apps/backend/internal/github/metrics_vars.go` — only the disposition
    counter, if task 03 has not created the file yet.

- **Dependencies:** task 01 (`UpdateTaskPRDisposition`, the `TaskPR` fields, and
  `validTaskPRDisposition`).
- **Parallelism:** parallel-safe with task 03 — see that task's note. Run
  sequentially unless the user explicitly asks otherwise.

- **Inputs:**
  - Spec: AC-20 through AC-30, AC-38; Permissions; Failure modes; the
    Concurrency section's disjoint-writer requirement.
  - Plan: "Disposition endpoint (task 04)", including the fixed order of
    operations (lookup and authorize first, so AC-26 can compare against the
    association's own identity and authorization matches the detach endpoint).
  - Patterns: `service_task_pr_detach.go` for the service shape, workspace
    check, `authorizeWorkspaceAccess`, and event publication; the existing
    `ErrInvalidPRURL` mapping at `controller.go:244`.
  - Decision to honour: a nil `disposition` — whether the JSON key was absent or
    explicitly `null` — means clear. The only body where the difference could
    matter (`{"superseded_by_url": "..."}` with no disposition) is already a 400
    under AC-24, so distinguishing them buys no behaviour.
  - Constraint: the disposition statement must name no sync-owned column, and
    `UpdateTaskPR` must name no `disposition*` column. The pinning test exists so
    this cannot regress silently.

- **Output contract:** summary of the endpoint contract and validation order;
  files changed; exact test commands and counts; blockers; risks; status update
  in this file and `plan.md`.

## Results

**Status: done.** Implemented per the plan's order of operations, with one
recorded design decision beyond the plan's literal text.

- `SetTaskPRDisposition`: lookup + workspace authorization first (matches
  `DetachTaskPR`), then `normalizeTaskPRDisposition` validates in the order
  AC-23 → AC-24 → AC-25 → AC-26, then `dispositionUnchanged` short-circuits
  an identical re-PATCH (AC-29) before any write.
- **Decision recorded (not explicit in the plan):** `superseded_by_url` is
  NOT merged with the stored value — every accepted PATCH writes exactly the
  `(disposition, superseded_by_url)` pair the request carries, full stop. A
  `disposition: "superseded"` PATCH with no `superseded_by_url` key clears
  any previously-recorded URL rather than preserving it. Chosen because (a)
  it keeps `UpdateTaskPRDisposition` a single unconditional `UPDATE ... SET`
  with no read-modify-write, matching the "one statement" design goal in the
  spec's Concurrency section, and (b) AC-21's "persist superseded_by_url
  when supplied" is equally well satisfied by "the request is the complete
  desired state" as by "merge with stored." A client that wants to keep an
  existing URL on a value-only change must resend it.
- Route: `PATCH /api/v1/github/task-prs/:associationId/disposition`,
  registered beside the existing task-PR routes in `controller.go`.
  `httpSetTaskPRDisposition` maps `ErrTaskPRNotFound` → 404,
  `ErrInvalidDisposition` / `ErrInvalidPRURL` → 400, mirroring
  `httpDeleteTaskPR`'s error-mapping shape.

**Files changed:** `service_task_pr_disposition.go` (new),
`controller.go` (route + handler), `controller_task_pr_disposition_test.go`
(new, 12 assertions across happy path / AC-22 through AC-29b),
`store_task_pr_disposition_test.go` (from task 01, disjoint-writer pinning).

**Commands run:**
```
cd apps/backend && gofmt -l internal/github                       # clean
go build ./...                                                    # ok
go test ./internal/github/... -run TestHttpSetTaskPRDisposition -v -count=1   # 12/12 pass, all first-try
go test ./internal/github/... -count=1                            # ok, 61.5s
golangci-lint run ./internal/github/... ./internal/persistence/... --timeout=5m   # 0 issues
golangci-lint run ./... --timeout=8m                               # 0 issues (whole backend)
```
The two `unused` warnings flagged after task 01 (for
`validTaskPRDisposition` / the disposition expvar map) resolved once this
task wired them up.

**Security/trust and external side effects:** None beyond the existing
task-PR mutation surface — this endpoint adds no new capability beyond
writing an annotation on an association the caller can already detach
(same workspace-scoped authorization as `DetachTaskPR`).
