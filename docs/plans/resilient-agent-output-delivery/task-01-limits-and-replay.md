---
id: "01-limits-and-replay"
title: "Align event limits and replay budgets"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.4
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Align event limits and replay budgets

## Summary

Establish one capacity contract across admission, queues, transport, replay, and disk headroom.
Raise event capacity to 2 MiB while preserving the 1 MiB prompt limit and historical records.

## In scope

- Add the dependency-neutral shared limits contract and assertions for its relationships.
- Separate `MaxSubmissionBytes` from event capacity, including HTTP request sizing and retained submission validation.
- Keep internal smaller event-limit fixtures restrictive by default. Explicit submission overrides remain separate.
- Add `ErrEventTooLarge` before sequence allocation and distinguish it from quota/disk pressure.
- Prove complete near-limit live envelopes fit the 4 MiB producer and backend queue bounds.
- Bound replay pages by encoded bytes and record count before full materialization.
- Cover HTTP replay, WS catch-up, retained inspection, bounded client reads, and short-page progress.
- Add optional authenticated limit metadata and the single-event replay fallback for older peers.
- Retain historical reads when a journal reopens with a smaller append limit.
- Prove that the maximum event fits existing 4 MiB SQL retention accounting without repeated skips.

## Out of scope

Tool display reduction, UI, failure monitoring, changed shell tails, larger prompts, format migrations, and live runtime replacement.

## Acceptance

1. Exact boundary tests accept a 2 MiB payload and reject one extra byte with a distinct size error and no sequence mutation.
   Prompt payloads still reject more than 1 MiB. The permanent size error never waits for ACK.
2. Multiple near-limit records replay through short pages to the actual terminal within the encoded-page budget.
   Every admitted event fits live transport. Restart and duplicate replay preserve original bytes and hashes.
3. New/old v2 compatibility fixtures prove known-field readability, owner preservation, and bounded fallback without forced replacement or legacy downgrade.

## TDD regression

Create `TestDeliveryEventLimitBoundaries`, `TestDeliveryEventLimitDoesNotRaiseSubmissionLimit`, and `TestDeliveryEventTooLargeDoesNotWaitForACK`.
Create `TestDeliveryReplayByteBudget`, `TestDeliveryReplayShortPageReachesTerminal`, and `TestDeliveryLimitLegacyCompatibility`.
Add `TestDeliveryEventLimitRetentionBudget` against the real repository batch path.
The existing implementation must fail the event-cap and replay-byte tests before production edits.
Also cover base64/envelope overhead, an individually unrepresentable historical record, and explicit custom limits.

## Verification

Run from the repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/types/deliverylimits ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite -run 'TestDelivery(EventLimit|EventTooLarge|Replay|LimitLegacy)|TestDeliveryClient' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new limits package and named regressions must exist before this final command runs.
Record actual test discovery and results. A successful command with missing planned tests is insufficient.
Run changed-file Go lint on the files owned by this work order before marking it done.

Run this exact changed-file Go lint block from the repository root:

```bash
python3 - <<'LINT'
import pathlib, subprocess
tracked = subprocess.check_output(['git', 'diff', '--name-only', '--diff-filter=ACMR', 'HEAD', '--', 'apps/backend'], text=True)
untracked = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '--', 'apps/backend'], text=True)
files = sorted({p for p in (tracked + untracked).splitlines() if p.endswith('.go') and pathlib.Path(p).is_file()})
if files:
    subprocess.run(['bash', 'scripts/lint-go-changed', *files], check=True)
LINT
```

## Files likely touched

- `apps/backend/internal/agentctl/types/deliverylimits/limits.go` and `limits_test.go` (new).
- `apps/backend/internal/agentctl/journal/journal.go`, `journal_submissions.go`, `retained.go`, `pressure.go`, `disk_capacity.go`, and their tests.
- `apps/backend/internal/agentctl/journal/location.go` and `recovery.go` for additive capability metadata.
- `apps/backend/internal/agentctl/server/process/delivery_batch.go` and `delivery_pressure.go` for shared relationships and typed size handling.
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`, `durable_delivery_stream_test.go`, and `prompt_durable_delivery_test.go`.
- `apps/backend/internal/agent/runtime/agentctl/agent.go`, `client_delivery.go`, and related tests.
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go` for short-page progress.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_storage_test.go` or a focused new retention-budget regression.
- New focused `delivery_limits_test.go` and replay budget test files alongside these owners.

## Dependencies

None. Read the complete package and the capacity, replay, and compatibility design sections.
The 2 MiB limit is one part of the repair. This work order does not establish all-tool safety by itself.

## Risks

- Event and submission limits currently share one setting.
- Encoded journal records include base64 overhead.
- A count-short page can still precede the captured high-water mark.
- Old readers must retain known JSON fields and historical payload hashes.

## Parallelism

`sequential`

## Inputs

- [Tool capacity design](../../specs/platform/system-design/bounded-tool-delivery.md#capacity-contract).
- Existing `Journal.Replay`, API replay loops, client queue accounting, retained inspection, and journal reopen tests.
- [Boundary decision](../../decisions/2026-10-27-all-tool-delivery-boundary.md).

## Results

Implemented the shared delivery-limit contract, an independent prompt submission limit, a distinct `ErrEventTooLarge` that never enters the capacity wait, byte-bounded replay pages, and optional capability metadata.

Validation passed:

- `(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/types/deliverylimits ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite -run 'TestDelivery(EventLimit|EventTooLarge|Replay|LimitLegacy)|TestDeliveryClient' -count=1)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

`TestDeliveryEventLimitBoundaries`, `TestDeliveryEventLimitDoesNotRaiseSubmissionLimit`, `TestDeliveryEventTooLargeDoesNotWaitForACK`, `TestDeliveryReplayByteBudget`, `TestDeliveryReplayShortPageReachesTerminal`, `TestDeliveryLimitLegacyCompatibility`, and `TestDeliveryEventLimitRetentionBudget` passed. Changed-file Go lint follows in the package verification record.

Changed-file Go lint reported 0 issues.
