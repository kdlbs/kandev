---
id: "02-host-reader"
title: "Expose the authorized automation history Host reader"
status: done
wave: 2
depends_on: ["01-source-observations"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AUTOMATION-READS-001
acceptance_criteria:
  - AC-PLUGINS-AUTOMATION-READS-001.1
  - AC-PLUGINS-AUTOMATION-READS-001.2
  - AC-PLUGINS-AUTOMATION-READS-001.3
  - AC-PLUGINS-AUTOMATION-READS-001.4
  - AC-PLUGINS-AUTOMATION-READS-001.5
  - AC-PLUGINS-AUTOMATION-READS-001.6
  - AC-PLUGINS-AUTOMATION-READS-001.7
  - AC-PLUGINS-AUTOMATION-READS-001.8
  - AC-PLUGINS-AUTOMATION-READS-001.9
  - AC-PLUGINS-AUTOMATION-READS-001.10
  - AC-PLUGINS-AUTOMATION-READS-001.11
  - AC-PLUGINS-AUTOMATION-READS-001.12
system_design:
  - ../../specs/plugins/system-design/automation-history-reads.md
---

# Task 02: Expose the Authorized Automation History Host Reader

## Summary

Deliver the optional SDK reader through the actual Host broker to the source
observations from Task 01. Apply current workspace authority, explicit public
mapping, existing snapshots, and truthful capability discovery.

## In scope

- Optional `AutomationHistoryHost`, reader/query/observation types, and
  `HostAutomationHistory` helper without extending existing Go interfaces.
- Three additive RPCs and generated protobuf/gRPC artifacts.
- Plugin-owned source interface and live dependency getter.
- `backendapp` adapter, safe schedule/summary/error projection, and normal
  startup wiring independent of Office/Coordinator product gates.
- Existing admission and snapshot helpers, per-call revocation checks,
  overflow errors, and exact read receipts.
- Support/authority discovery, unsupported dependencies, and wire tests.

## Out of scope

- Browser APIs, settings UI, provider integrations, and automatic approvals.
- A second cursor implementation or changes to shared snapshot limits.
- Publication, deployment, a stable backport, and consumer export policy.

## Acceptance

1. All three SDK operations reach the authorized source path through the real
   broker and preserve the designed observation fields.
2. Foreign IDs, missing/revoked grants, cursor mismatch/expiry, missing
   dependencies, and source errors produce explicit outcomes without leaks.
3. Snapshots remain bounded and immutable. Fresh exact reads observe later
   state, sanitized output excludes private fields, and legacy methods work.

## Verification

Run from the repository root after the TDD implementation:

```bash
make -C pluginsdk proto
make -C pluginsdk test
go -C apps/backend test -trimpath -race ./internal/plugins -run 'TestAutomationHistory|TestExactWorkspaceSnapshot|TestManagedConversationScheduleHostAuthorizationAndReplay' -count=1
go -C apps/backend test -trimpath -race ./internal/backendapp -run '^TestPluginAutomationHistory' -count=1
git diff --check
```

The named new tests must exist before recording success. Use the files and
test names in the [plan](plan.md#tests).

Include denial before source access, changed approval revisions, a different
installation, foreign run IDs, and malformed filters.
Include equal timestamps, stale snapshots, changed source values, 5,001 rows,
and the byte limit.
Prove that a cached page cannot bypass revocation. A fake denied source must
record zero calls.

Seed credential patterns and private trigger/claim fields. Inspect the decoded
SDK response and protobuf response for their absence. Exercise late wiring on
an existing Host object and a missing dependency in discovery.

## Files likely touched

- `pluginsdk/automation_history.go`, `automation_history_grpc.go`, and
  `automation_history_test.go` (new).
- `pluginsdk/host.go` optional server wiring.
- `pluginsdk/proto/kandev/plugin/v1/plugin.proto` and generated Go outputs.
- `apps/backend/internal/plugins/automation_history.go` (new source interface).
- `apps/backend/internal/plugins/host_automation_history.go` (new).
- `apps/backend/internal/plugins/host_automation_history_test.go` and
  `host_automation_history_wire_test.go` (new).
- `apps/backend/internal/plugins/service.go`, `host.go`, and `host_exact.go`
  for dependency and method registration.
- `apps/backend/internal/backendapp/adapters_plugin_automation_history.go`
  and `adapters_plugin_automation_history_test.go` (new).
- `apps/backend/internal/backendapp/services.go` for startup wiring.

## Dependencies

- [Task 01](task-01-source-observations.md).

## Risks

- Direct automation imports in the plugin package create an import cycle.
- Extending the base Host or exact-query interface breaks existing consumers.
- A remote SDK wrapper can exist even when its server lacks the new RPC.
- Broad text sanitizers can corrupt opaque identity fields. Sanitize excerpts
  separately and preserve IDs as strings.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/automation-history-reads.md).
- [System design](../../specs/plugins/system-design/automation-history-reads.md).
- `pluginsdk/managed_automation_schedules.go` optional-interface pattern.
- `plugins/exact_read_snapshots.go`, `host_exact_queries.go`,
  `host_data_wire_test.go`, and `host_managed_automation_schedules_test.go`.
- `backendapp/adapters_plugin_managed_automation.go` adapter pattern.
- `apps/backend/AGENTS.md` and `/tdd`.

## Results

Status: done.

Added the optional `pluginsdk.AutomationHistoryHost` and
`HostAutomationHistory` helper, three additive Host RPCs with regenerated
protobuf/gRPC artifacts, and the plugin-owned `AutomationHistoryDataSource`
seam. The Host implementation reuses existing admission, snapshots, receipts,
and capability discovery. `backendapp` wires a safe adapter that maps source
observations without exposing automation models. The excerpt pipeline strips
system content before its final credential-redacted bound and cuts at any
remaining system tag, so an excerpt that ends inside a hidden block fails
closed instead of returning the hidden prefix. A credential-shaped suffix cut
before the normal redaction minimum is also masked. Dependencies are read
through a live getter, so hosts created before automation wiring observe it
later.

Verification:

- `make -C pluginsdk proto`: passed.
- `make -C pluginsdk test`: passed.
- `go -C apps/backend test -trimpath -race ./internal/plugins -run 'TestAutomationHistory|TestExactWorkspaceSnapshot|TestManagedConversationScheduleHostAuthorizationAndReplay' -count=1`: passed.
- `go -C apps/backend test -trimpath -race ./internal/backendapp -run '^TestPluginAutomationHistory' -count=1`: passed.
- `go -C apps/backend build ./...`: passed.
- Added regressions for a system block crossing the old 280-character summary
  boundary, an unterminated system block that must produce no excerpt, and a
  credential-shaped suffix cut before the normal redaction minimum.
- `bash scripts/lint-go-changed <changed-go-files>`: 0 issues.
- `git diff --check`: passed.
