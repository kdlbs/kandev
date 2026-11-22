---
id: "17-host-message-source"
title: "Host: Message.Source on the message read DTO"
status: pending
wave: 2
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
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
Pending.
