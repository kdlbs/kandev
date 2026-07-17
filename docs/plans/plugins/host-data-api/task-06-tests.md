---
id: "06-tests"
title: "Capability-gating, DTO-mapping, and plugin-reads-sessions integration tests"
status: done
wave: 4
depends_on: ["04-host-data-impl"]
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 06: Host data API tests

Cover the spec scenarios for the Host data API at the host boundary. (Aggregation
SQL and SDK round-trip conversions are unit-tested in tasks 02/03; this task owns
the host RPC behavior and the end-to-end plugin path.)

## Scope
- **Capability gating (`internal/plugins/host_test.go`):** for each read RPC,
  assert `PermissionDenied` with `capability 'api_read:<resource>' not declared`
  when the manifest omits the resource, and success when it declares it. Assert
  write RPCs return `Unimplemented`.
- **DTO mapping:** table-driven internal-model → DTO assertions per resource,
  covering optional/NULL fields and RFC3339 timestamp formatting; assert the
  Session DTO carries `acp_session_id`.
- **Pagination:** `ListTasks` (or another list RPC) returns `has_more` + a
  `next_cursor` that yields the next page and terminates.
- **Ephemeral exclusion:** `ListTasks` without `include_ephemeral` omits ephemeral
  tasks.
- **Integration:** drive a Host client (SDK `grpcHostClient` over the broker, or
  the existing plugins test harness) to call `ListSessions` +
  `ListSessionCodeStats` against a seeded DB and assert rows come back without any
  file-level DB access — the concrete "plugin reads sessions via the API" scenario.

## Acceptance
- Tests exist for each spec scenario listed above and pass.
- No test opens the SQLite file to stand in for the plugin path — the plugin-facing
  assertion goes through the Host RPC surface.

## Verification
- `cd apps/backend && go test ./internal/plugins/...`

## Files likely touched
- `apps/backend/internal/plugins/host_test.go`
- possibly `apps/backend/internal/plugins/host_data_test.go`

## Inputs
- Host impl from task-04; existing test harness in
  `internal/plugins/host_test.go` / `service_test.go`.
- Spec: "Host data API" scenarios.
- Backend testing conventions in `apps/backend/CLAUDE.md` (synctest, goleak).

## Dependencies
Task 04.

## Output contract
Summary, list of scenarios covered, test result, coverage gaps if any, and status
update here + in `plan.md`.
</content>
