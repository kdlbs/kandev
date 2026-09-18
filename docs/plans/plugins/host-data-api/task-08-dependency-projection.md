---
id: "08-dependency-projection"
title: "Expose task dependencies in the plugin and canvas data API"
status: done
wave: 5
depends_on: ["04-host-data-impl"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASK-DEPS-001
  - REQ-PLUGINS-TASK-DEPS-002
  - REQ-PLUGINS-TASK-DEPS-003
  - REQ-PLUGINS-TASK-DEPS-004
  - REQ-PLUGINS-TASK-DEPS-005
  - REQ-PLUGINS-TASK-DEPS-006
acceptance_criteria:
  - AC-PLUGINS-TASK-DEPS-001.1
  - AC-PLUGINS-TASK-DEPS-001.2
  - AC-PLUGINS-TASK-DEPS-001.3
  - AC-PLUGINS-TASK-DEPS-001.4
  - AC-PLUGINS-TASK-DEPS-001.5
  - AC-PLUGINS-TASK-DEPS-001.6
  - AC-PLUGINS-TASK-DEPS-001.7
  - AC-PLUGINS-TASK-DEPS-001.8
  - AC-PLUGINS-TASK-DEPS-001.9
  - AC-PLUGINS-TASK-DEPS-001.10
  - AC-PLUGINS-TASK-DEPS-001.11
  - AC-PLUGINS-TASK-DEPS-001.12
  - AC-PLUGINS-TASK-DEPS-001.13
  - AC-PLUGINS-TASK-DEPS-001.14
  - AC-PLUGINS-TASK-DEPS-001.15
  - AC-PLUGINS-TASK-DEPS-001.16
  - AC-PLUGINS-TASK-DEPS-001.17
  - AC-PLUGINS-TASK-DEPS-001.18
  - AC-PLUGINS-TASK-DEPS-002.1
  - AC-PLUGINS-TASK-DEPS-002.2
  - AC-PLUGINS-TASK-DEPS-002.3
  - AC-PLUGINS-TASK-DEPS-002.4
  - AC-PLUGINS-TASK-DEPS-002.5
  - AC-PLUGINS-TASK-DEPS-003.1
  - AC-PLUGINS-TASK-DEPS-003.2
  - AC-PLUGINS-TASK-DEPS-003.3
  - AC-PLUGINS-TASK-DEPS-003.4
  - AC-PLUGINS-TASK-DEPS-003.5
  - AC-PLUGINS-TASK-DEPS-003.6
  - AC-PLUGINS-TASK-DEPS-003.7
  - AC-PLUGINS-TASK-DEPS-003.8
  - AC-PLUGINS-TASK-DEPS-004.1
  - AC-PLUGINS-TASK-DEPS-004.2
  - AC-PLUGINS-TASK-DEPS-004.3
  - AC-PLUGINS-TASK-DEPS-004.4
  - AC-PLUGINS-TASK-DEPS-004.5
  - AC-PLUGINS-TASK-DEPS-004.6
  - AC-PLUGINS-TASK-DEPS-004.7
  - AC-PLUGINS-TASK-DEPS-004.8
  - AC-PLUGINS-TASK-DEPS-004.9
  - AC-PLUGINS-TASK-DEPS-004.10
  - AC-PLUGINS-TASK-DEPS-004.11
  - AC-PLUGINS-TASK-DEPS-004.12
  - AC-PLUGINS-TASK-DEPS-004.13
  - AC-PLUGINS-TASK-DEPS-004.14
  - AC-PLUGINS-TASK-DEPS-004.15
  - AC-PLUGINS-TASK-DEPS-005.1
  - AC-PLUGINS-TASK-DEPS-005.2
  - AC-PLUGINS-TASK-DEPS-005.3
  - AC-PLUGINS-TASK-DEPS-005.4
  - AC-PLUGINS-TASK-DEPS-005.5
  - AC-PLUGINS-TASK-DEPS-005.6
  - AC-PLUGINS-TASK-DEPS-005.7
  - AC-PLUGINS-TASK-DEPS-005.8
  - AC-PLUGINS-TASK-DEPS-006.1
  - AC-PLUGINS-TASK-DEPS-006.2
  - AC-PLUGINS-TASK-DEPS-006.3
  - AC-PLUGINS-TASK-DEPS-006.4
  - AC-PLUGINS-TASK-DEPS-006.5
system_design:
  - ../../../specs/plugins/system-design/task-dependency-projection.md
  - ../../../specs/plugins/system-design/task-dependency-refresh.md
  - ../../../specs/plugins/system-design/task-dependency-edge-ends.md
  - ../../../specs/plugins/system-design/task-dependency-response-bounds.md
---

# Task 08: Expose task dependencies in the plugin and canvas data API

## Summary

A workspace canvas or gRPC plugin reading the Host Data API's `Task` read model
could not see a single dependency edge, even though the kanban board already
derives and ships a complete view (`blocked`, `blocked_reason`, `depends_on`,
`blocks`, `start_when_unblocked`) via `BuildDependencyViews`. This task adds
the same seven-field, read-only projection to every plugin task read model
across the gRPC surface (proto, SDK, `grpcHostServer`), the canvas JSON
surface (`webapp_protocol_json.go`), and the shared derivation the two reuse
(`internal/task/service/service_dependencies.go`), landing it as a field on
the existing `api_read:tasks` capability with no new grant.

## Scope

- **Field set** (all seven, always present within one host version — absence
  is never a third state): `blocked`, `blocked_reason`
  (`pending|failed|unknown`), `depends_on[]`, `blocks[]`,
  `depends_on_truncated`, `blocks_truncated`, `start_when_unblocked`.
- **Attachment rule**: the projection is derived and attached on every plugin
  task read model object a response actually serializes, and on no discarded
  or intermediate read — enforced via non-attaching (`fetchTask`,
  `fetchTaskForScopeCheck`, `writeTaskUpdate`) vs. attaching
  (`taskReader.Get/Update/Move`, `pluginOwnedTaskTreeManager.Preview`) helper
  pairs across nine call sites (6 gRPC RPCs, 3 canvas routes).
- **Withheld verdict**: `blocked=true, blocked_reason="unknown"`, empty lists,
  both truncation flags false, `start_when_unblocked=false` — the single
  fail-closed sentinel for all four derivation-failure paths and for a caller
  lacking `api_read:tasks` (indistinguishable from each other by design).
- **Fan-out bound**: `maxDependencyFanOut = 4096` distinct edge ends on
  multi-task flows (`ListTasks`, canvas task list, preview RPC), returned as
  gRPC `ResourceExhausted`; single-task reads and write RPCs are exempt and
  use the unbounded derivation.
- **Edge-end redaction**: canvas surfaces redact `title`/`state` on an edge
  end their scope does not admit as directly readable (5-way switch over
  scope kind), decided without an additional read; gRPC redacts nothing.
- **Shared-derivation fixes** (Kanban-visible, no migration): ordering
  (`ListBlockersForTasks` gains `ORDER BY created_at, blocker_task_id`),
  dangling-edge drop extended to the `Blocks` direction, fail-closed on all
  four derivation-failure paths, edge-end workspace id carried on the ref,
  and chunking both batched id-query families
  (`sqliteMaxHostParams = 500`) past SQLite's host-parameter ceiling on the
  unpaginated preview flow.
- **Events**: deliberately unchanged — dependency fields are not added to the
  canvas SSE allowlist (`webapp_events.go`'s `commonFields`); the contract is
  refetch-on-signal off the existing `task.updated` /
  `task.dependencies_resolved` / `task.dependency_failed` events, including a
  `task.state_changed` refetch rule for any task named in a cached task's
  edge lists.
- **No new capability**: the projection is a field set on `api_read:tasks`;
  version skew is gated by the manifest's existing `min_kandev_version` floor.

## Acceptance

- `GET ./_kandev/v1/data/tasks` on a workspace canvas and gRPC
  `ListTasks`/`GetTask` return the same seven dependency fields, redaction
  aside, for the same tasks.
- Listing N tasks issues a constant number of dependency queries, not one per
  task (batched `BuildDependencyViews`/`BuildDependencyViewsBounded`).
- A derivation failure or a caller without `api_read:tasks` yields the
  withheld verdict, never a fail-open empty/false projection.
- Mapper round-trip (proto ↔ SDK), canvas JSON projection, the redaction-scope
  table, the withheld-verdict path, and the fan-out/chunking bound are each
  covered by tests in the relevant package.

## Verification

- `go test ./internal/plugins/... ./internal/task/service/... ./internal/task/repository/sqlite/... ./internal/office/repository/sqlite/... ./pkg/pluginsdk/...` — all green.
- `golangci-lint run ./...` (backend) — 0 issues.
- `gofmt -l` on touched files — clean.
- Three independent review legs (code-reviewer, security-reviewer,
  test-supervisor) plus a mandatory cross-vendor `codex` pass ran against the
  final diff; every finding was independently re-verified against source.
  codex's one finding and the security-reviewer's SEC-001 finding were both
  confirmed to predate this branch and fall outside its diff (see the task
  plan's Review round 3 entry for the full trace); no production defect
  survived review. The residual is test-rigor coverage gaps, tracked in the
  task plan rather than blocking this PR.

## Files touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`,
  `apps/backend/pkg/pluginsdk/data_types.go` (+ tests) — `Task` field
  additions, `TaskDependencyRef`, proto/SDK conversions.
- `apps/backend/internal/plugins/host.go`, `host_data.go`,
  `host_data_dependencies.go` (+ tests), `host_write.go` (+ tests),
  `service.go`, `webapp_protocol.go`, `webapp_protocol_data.go`,
  `webapp_protocol_json.go` (+ tests), `webapp_events.go` (test only) — gRPC
  and canvas attachment, redaction, withheld-verdict, and logging.
- `apps/backend/internal/task/service/service_dependencies.go` (+ tests) —
  shared derivation: ordering, dangling-edge drop, fail-closed paths,
  workspace id on refs, fan-out bound.
- `apps/backend/internal/office/repository/sqlite/blockers.go` (+ tests),
  `apps/backend/internal/task/repository/sqlite/task.go` (+ tests) — ordering
  and chunking on the batched id queries.
- `docs/public/canvases.md`, `docs/public/plugins-authoring.md`,
  `apps/backend/internal/mcp/canvasskill/files/references/browser-api.md` —
  consumer-facing contract docs.

## Output contract

Summary, files changed, and status update here + in `plan.md`.

## Output

**Status: done.** Landed across 9 commits on
`feature/expose-task-dependen-61269a`
(`5d359841a`..`9a5b0b761`), covering the frozen spec's `-001` through `-006`
requirements in full. Spec Review ran 8 rounds to freeze the contract; Build
ran 7 rounds (2 operator-authorized beyond the standing 4-round cap) fixing
findings from Testing and Review; Testing round 7 and Review round 3 both
found no production defect. See the task plan for the full round-by-round
record, including the two review findings adjudicated as pre-existing and
out of this branch's scope (a codex finding about the internal event bus,
and a security-reviewer finding about gRPC plugin event-delivery capability
gating — both confirmed unmodified by this branch and filed as follow-up
candidates rather than blockers).
