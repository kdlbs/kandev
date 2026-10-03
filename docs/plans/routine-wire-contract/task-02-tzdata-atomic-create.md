---
id: "02-tzdata-atomic-create"
title: "Embedded tzdata and atomic routine + trigger create"
status: done
wave: 1
depends_on: ["01-wire-adapter"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-WIRE-001
  - REQ-OFFICE-ROUTINE-WIRE-002
  - REQ-OFFICE-SCHEDULER-001
acceptance_criteria:
  - AC-OFFICE-ROUTINE-WIRE-001.6
  - AC-OFFICE-ROUTINE-WIRE-002.2
  - AC-OFFICE-ROUTINE-WIRE-002.10
  - AC-OFFICE-SCHEDULER-001.14
system_design:
  - ../../specs/office/system-design/routine-wire-contract.md
  - ../../specs/office/system-design/scheduler-01.md
---

# Task 02: Embedded tzdata and atomic routine + trigger create

## Summary

Two independent gaps kept a cron schedule from arming reliably. On a deployed
Windows host the backend resolved an IANA zone such as `Asia/Seoul` with
`time.LoadLocation`, which fails when the host has no system zoneinfo, so a
cron trigger's `next_run_at` could not be computed. Separately, the create
flow sent the routine and its cron trigger as two requests, so a rejected
trigger left a schedule-less routine behind and the operator saw a success.
This task embeds the IANA timezone database in the binary and collapses the
create into one transactional request.

## In scope

- `apps/backend/cmd/kandev/main.go`: blank-import `time/tzdata` so the
  distributed binary resolves an IANA zone with no system zoneinfo directory
  and no build-machine `GOROOT`.
- `apps/backend/internal/office/routines/{dto.go,handler.go,service.go}`: an
  optional `trigger` on `CreateRoutineRequest`; `CreateRoutineWithTrigger`
  validates the trigger before any write and delegates to the repository.
- `apps/backend/internal/office/repository/sqlite/routines.go`:
  `CreateRoutineWithTrigger` inserts the routine and its trigger in one
  transaction, assigning the routine id and rolling both rows back together.
- `apps/web/app/office/routines/routines-content.tsx`: build the routine and
  its optional cron trigger into one request body; a rejected trigger leaves
  the dialog open with the entered values.
- `apps/web/lib/api/domains/office-routine-normalize.ts` and
  `apps/web/lib/state/slices/office/types.ts`: carry the optional `trigger`
  create field through the request builder and model.
- `apps/web/e2e/tests/office/routines-ui.spec.ts`: the create-dialog test
  waits for the single routine-create request instead of a separate trigger
  POST.
- `docs/specs/office/requirements/scheduler.md`,
  `docs/specs/office/requirements/routine-wire-contract.md`,
  `docs/specs/office/system-design/routine-wire-contract.md`: record the
  embedded-tzdata criterion and the one-request create.

## Acceptance

The criteria listed in this file's frontmatter, defined under their
requirements in
[routine-wire-contract.md](../../specs/office/requirements/routine-wire-contract.md)
and [scheduler.md](../../specs/office/requirements/scheduler.md).

## Verification

```bash
gofmt -l apps/backend
cd apps/backend
go test -p 2 ./cmd/kandev/... ./internal/office/routines/... ./internal/office/repository/sqlite/...
cd ../web
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run test -- app/office/routines/routines-content.test.tsx
pnpm run e2e:run -- tests/office/routines-ui.spec.ts
```

## Files likely touched

See `## In scope` above; full diff stat against the branch's merge-base:
`git diff --stat $(git merge-base HEAD upstream/main)..HEAD`.

## Dependencies

Depends on Task 01 (`01-wire-adapter`), which established the snake_case
request builder this task extends with the optional `trigger` field. The
tzdata import is independent of the wire adapter and rides in the same change
because both are the reason a cron schedule failed to arm.

## Parallelism

Not applicable: the Go write path, the client create flow, and the E2E
assertion change together behind one create request.

## Results

Shipped on `kd/up-office-tzdata-routines-v2` as PR
[kdlbs/kandev#4022](https://github.com/kdlbs/kandev/pull/4022). `cmd/kandev`
passes `TestEmbeddedTZDataLoadsIANAZoneWithoutSystemZoneinfo`, which calls
`time.LoadLocation("Asia/Seoul")` in a child process with `GOROOT` pointed at
an empty directory and `ZONEINFO` cleared, reproducing a deployed Windows
binary. `internal/office/routines` and
`internal/office/repository/sqlite` cover the atomic create: a rejected
trigger (`ErrInvalidTrigger` -> 400) writes no routine, and a failed trigger
insert rolls the routine back. The updated Playwright create-dialog test
passes against the single-request flow.
