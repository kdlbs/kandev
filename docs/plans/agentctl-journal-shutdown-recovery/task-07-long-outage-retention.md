---
id: "07-long-outage-retention"
title: "Retain output through extended backend outages"
status: completed
wave: 5
depends_on:
  - "04-acknowledgment-capacity"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.5
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 07: Retain output through extended backend outages

## Outcome and ownership

Continue durable agent output beyond the former 256 MiB/2 GiB quotas while usable disk remains.
Own `apps/backend/internal/agentctl/journal`, delivery pressure in `internal/agentctl/server/process`, and directly affected tests.
This task is parallel-safe with Task 05's orchestrator and SQL work. Coordinate lifecycle fixture changes with Task 05's owner.
Do not modify provider plugins, runtime lifecycle, shared models, package lockfiles, or public feature flags.

## Implementation boundary

Keep the current journal format and append-before-publication guarantee.
Default production retention has no fixed logical byte ceiling. Explicit positive internal test limits remain available for quota regression coverage.
Keep the 1 MiB event bound and bounded producer/replay buffers.
Use filesystem available capacity on the journal's actual volume, with a 32 MiB control/recovery reserve and write-size headroom.
Reuse existing disk-capacity support where appropriate; do not query remote filesystems synchronously for every output event or spawn unbounded samplers.
ACK and exact-owner Stop remain operable under pressure. Unknown disk statistics do not establish exhaustion; real write errors retain their typed handling.
Only actual low usable disk or persistence failure can trigger pressure cancellation in the default policy.
Skip optional compaction if its copy would breach the reserve. ACK pruning and reusable pages must keep working without compaction.
No age expiry, unacknowledged rollover deletion, fabricated ACK, prompt resend, or silent truncation is allowed.

## Acceptance

1. Real journal/process tests retain and replay output beyond 256 MiB without canceling an active disconnected agent. Total retention can cross the former 2 GiB boundary; accounting tests must cover that boundary without requiring every unit run to write gigabytes.
2. A simulated week of backend absence does not expire records or change prompt identity. Reconnection replays the exact ordered tail and terminal event with bounded pages while concurrent output remains possible.
3. Low disk, write failure, ACK reclamation, compaction headroom, reopen, and legacy finite-quota tests establish recoverable behavior and bounded resource use.

## Verification

First observe a behavioral RED through the real journal/process path.
Run sequentially from `apps/backend`:

```bash
GOMAXPROCS=2 go test -p 1 -race ./internal/agentctl/journal ./internal/agentctl/server/process -count=1
GOMAXPROCS=2 go test -p 1 -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -count=1
```

Run a targeted real-file large-retention test and record bytes, replay completeness, cancellation count, and reopen result.
Use a deterministic clock for elapsed-week assertions; do not sleep for a week.
Run affected cross-platform compile checks for any platform-specific disk code.
The coordinator updates public docs and reconciles any affected browser quota fixture.

## Results

Implemented and reviewed on 2026-10-10 with GPT-6 Luna max workers.

- Expected RED: `TestDurableDeliveryDefaultRetentionExceedsFormerStreamLimit` cancelled the active `retention-submission` at record 201 despite usable disk space.
- Expected RED: `TestDefaultJournalAccountingCrossesFormerTwoGiBLimit` rejected append with `agent delivery journal is full` at the former 2 GiB boundary.
- GREEN under race: the targeted journal regressions passed. The real manager/process test retained 230 messages of 900,000 payload bytes each (more than 256 MiB encoded), reopened the journal, and replayed them with zero active-turn cancellations.
- Cached capacity depletion, mixed terminal/ordinary batches, bounded sampling, allocation headroom, and compaction reserve have focused regressions.
- Both package race selections passed: journal/process and server API/runtime agentctl. The latest real-file retention assertion also passed under race.
- Review added physical capacity reservations for submission/control writes. Their focused race tests passed.
- A real-file ACK regression reproduced rejection despite reusable bbolt pages, then passed after admission credited reusable pages without reducing the filesystem reserve or allocation headroom.
- Final review added atomic reusable-page reservations and aligned pressure status with admission. The focused journal race suite passed, followed by the exact journal/process race gate (process package: 507.910 seconds).
- The final API/runtime race gate passed (server API: 258.357 seconds; runtime agentctl: 6.490 seconds).
- Scoped lint passed with zero issues. Journal and process test binaries compiled for Darwin arm64 and Windows amd64. These are compilation checks, not runtime tests on those platforms.
- An initial lint attempt encountered another host process holding the lint lock; no lock was removed. The later lint run passed normally.
