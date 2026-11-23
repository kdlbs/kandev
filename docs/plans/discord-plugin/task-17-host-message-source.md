---
id: "17-host-message-source"
title: "Host: Message.Source on the message read DTO"
status: done
wave: 2
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-007
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-007.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-007.2
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
---

# Task 17: `Message.Source`

Repository: `kdlbs/kandev`. Use `/tdd`.

- **Acceptance:**
  1. `pluginsdk.Message` gains `Source string`, populated from the persisted
     `metadata.source` (for example `plugin:<id>`) and empty when absent. No
     other metadata is exposed.
  2. The field is available on `Messages().List` and on the exact message
     observations (`ExactMessageObservation.Message`), with a wire round-trip
     test.
- **Verification:**
  - Regenerate protos.
  - `cd apps/backend && go test ./internal/plugins/...`
  - `cd pluginsdk && go test ./...`
- **Files likely touched:** `pluginsdk/data_types.go` (+ test), `pluginsdk/proto/kandev/plugin/v1/plugin.proto` (+ generated), `apps/backend/internal/plugins/host_data.go` (+ tests).
- **Dependencies:** task-01.
- **Parallelism:** sequential with task-02 (shared proto).
- **Inputs:** `Message` at `pluginsdk/data_types.go:1014-1023`; the
  message-to-DTO mapping in `host_data.go`.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
`pluginsdk.Message.Source` and proto `Message.source = 9` carry the persisted
string `metadata.source` only; a missing or non-string value maps to empty.
`messageModelToDTO` (`apps/backend/internal/plugins/host_data_mappers.go`) is
the single mapping behind `Messages().List` and `ListSanitizedMessages`, so
both surfaces expose it.

- **Files:** `pluginsdk/data_types.go`, `pluginsdk/proto/kandev/plugin/v1/plugin.proto`,
  regenerated `plugin.pb.go`, `apps/backend/internal/plugins/host_data_mappers.go`;
  tests in `pluginsdk/host_data_test.go` (bufconn round trip),
  `apps/backend/internal/plugins/host_data_test.go`, `host_data_wire_test.go`
  (real broker transport), and `host_exact_queries_test.go`. The SDK contract
  change bumps `pluginsdk/sdk-release.json` and `@kdlbs/plugin-sdk` to `0.4.0`
  (`additive`, host fields null until Stable preparation), synced with
  `scripts/pluginsdk/sync-record.mjs`.
- **Tests:** red first (all four assertions failed with empty `Source`), then
  `cd pluginsdk && make proto` (pinned buf v1.47.2; only `plugin.pb.go`
  changed), `cd apps/backend && go test ./internal/plugins/...` and
  `cd pluginsdk && go test ./...` passed.
- **Dependency note:** task-01 is still `in_progress` (ADR `proposed`); the ADR
  already decides `Message.Source`, and the gate in #5420 allows this task.
