---
created: 2026-10-27
status: complete
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
  - ../../specs/platform/system-design/durable-delivery-failure.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Implementation plan: resilient agent output delivery

## Overview

Prevent oversized tool reports from breaking execution and make writer failure visible without waiting for another output event.
Raise event capacity with aligned replay budgets, prepare every tool category, preserve real terminal evidence, and prove desktop/phone recovery.
Three work orders execute sequentially. The complete package is the release unit.
All three work orders are implemented and verified in the same package pull request.

Platform owns this package because it owns durable delivery between agentctl and the backend.
The frontend consumes that contract through existing chat and recovery surfaces.

## Confirmed intent

The user accepted a modest cap increase together with shared size preparation and reliable failure recovery.
Use 2 MiB for events while retaining the existing prompt limit.
Keep native results intact and use explicit bounded Kandev previews for larger display data.
Preserve historical uncertainty and recover without resending the old instruction.
Full-output artifacts and live database repairs are excluded.

The user then requested a simple solution with the existing interface unchanged.
New output widgets, preview controls, labels, and metadata projection are excluded.
The 1536 KiB display budget, 4 MiB replay ceiling, and two-second status probes are engineering choices within the backend scope.
They preserve headroom and fit the existing queue bounds.

## Evidence and reproduction

Affected task: `2f3e5072-970f-4363-be99-73a9dbf79c1b`.
Affected session: `174443d4-4830-460d-80e0-dfc4845aa528`.
Native conversation: `01a17a36-59e4-71f3-8a88-6ce3f35c1e01`.
Running backend evidence identified commit `b6263ea4c3165c86ec60e29e798475ad552ad6ce`.
The causal source files matched the investigation checkout.

- October 26, 21:56:01 UTC: search completed with 1,048,607 text bytes.
- The normalized completion serialized to 1,060,995 bytes before submission metadata, exceeding the 1 MiB cap.
- Generic-only preparation skipped `code_search`. Agentctl reported a failed durable append and cancelled Codex.
- Native history recorded `turn_aborted: interrupted` immediately. Backend receipt/projection stopped at sequence 132.
- At 22:01:46 UTC, the watchdog showed a waiting notice. The retained execution prevented execution-less session recovery.
- October 27, 09:24 UTC: user cancellation timed out and escalated. SQL still held a `dispatching` submission.
- The next instruction was blocked by `unresolved_durable_work`.

A temporary Go replay of the captured event reproduced `agent delivery stream is full` after existing preparation.
The temporary helper, journal, event copy, and extracted logs were removed.
Permanent fixtures must be synthetic. Do not copy private prompts or search results into tests.

## Scope

### In scope

- A 2 MiB event limit, independent 1 MiB submission limit, and shared capacity relationships.
- Byte-bounded HTTP/WS/retained replay, bounded client reads, and mixed-peer compatibility.
- All-category display preparation before cloning, queueing, and immutable acceptance.
- Typed size errors, preserved shell patches, and existing truncation representations.
- Independent authenticated health observation, bounded cancellation drain, and exact evidence-based recovery.
- Regression coverage for current desktop/phone recovery controls, plus public guidance.

### Out of scope

- New UI components, preview controls, tool-output notices, labels, and locale keys.
- New common marker metadata or canonical projection plumbing.

- Full original-output storage, downloads, content-addressed artifacts, or lossless fragmentation.
- Larger prompt payloads, shell tails, or new operator settings and runtime flags.
- Rewriting accepted hashes, advancing cursors without receipt, new SQL tables, or a new journal format.
- Provider retry, workflow completion, process survival, or background-work authority changes.
- Automatically replacing retained agents or repairing the reported live session.

## Technical approach

### 1. Limits and replay

Introduce the shared `internal/agentctl/types/deliverylimits` contract described by the tool design.
Update journal event defaults, submission defaults/getter, disk headroom, writer/client queues, and API/client budget handling together.
Retain 4 MiB queue bounds and prove that a maximum accepted event fits its complete live envelope.

Bound `Journal.Replay` before decoding the complete selected page.
Count stored encoded records and reserve response-envelope space.
Apply the 4 MiB encoded-page ceiling to HTTP, WS catch-up, and retained inspection.
Every consumer continues through short pages until the captured high-water mark.
Older peers without byte-budget metadata use one-event HTTP pages and bounded response reads.

### 2. All-category preparation

Generalize `streams.BoundGenericToolEvent` into shared preparation across ACP, native Codex, and direct process emission.
Use explicit typed display policies and one aggregate budget, including duplicate `ToolCallContents` and rich blocks.
Keep generic value limits, shell normalization, compact patch baselines, and immutable accepted records.

Reuse existing typed truncation flags, generic preview values, and rich-content text markers.
Do not add a common marker namespace, new message metadata, or a projection layer.
Omit unsafe structured bodies as a whole instead of presenting a partial file or patch as complete.
Existing renderers and normalized hash contracts remain unchanged.

### 3. Writer failure and admission

Keep a bounded native-event consumer through cancellation drain after an ordinary append fails.
Retain the captured failure identity and attempt to commit only an actual terminal through existing reserved capacity.
Actual failed terminal persistence preserves uncertainty.

Extend existing status JSON with a correlated delivery-failure snapshot.
An owned `StreamManager` monitor probes every two seconds with a two-second request deadline.
It uses existing recovery fences and native inventory proof.
It never depends on browser focus or output arrival, and never manufactures completion or prompt authority.

### 4. Existing interface

Keep the current tool rows, output disclosures, copy, layout, and recovery actions.
Delivery failures activate the existing recovery card promptly through the current backend model.
No new screen, preview widget, notification, locale key, or interaction is planned.
Desktop/phone E2E verifies current actions, draft retention, reload, and recovery without resending old work.

### Provider and transport compatibility

| Source | Representation | Coverage | Unsupported fallback |
| --- | --- | --- | --- |
| Codex ACP | Search/read/edit/execute classifications, raw content | Real normalizer and both emit paths | Typed size refusal with visible recovery |
| Other ACP agents | Shared typed or generic normalized reports | Shared policies plus representative ACP fixtures | Preserve provenance and refuse unknown oversized control data |
| Native Codex app-server | Existing normalized tool events | Real `emit` seam and immutable snapshots | Same preparation and health boundary |
| Direct process producers | Existing `AgentEvent` | Defensive process preparation and final byte guard | Distinct size error, no ACK wait |
| Retained v2 peer | Existing journal and status | Old-reader normalization, old-producer replay, no replacement | Precise recovery refusal when correlation is unavailable |
| Identified legacy transport | Existing non-durable event path | New-producer preparation and current legacy projection | Existing uncertainty, never false durable capability |

## Companion packages

- [Generic tool delivery](../bounded-tool-delivery/plan.md) remains implemented for its original narrower scope.
- [Durable stream repair](../durable-agent-stream-repair/plan.md) retains its completed stream/projection evidence.
- [Journal shutdown recovery](../agentctl-journal-shutdown-recovery/plan.md) owns its existing ACK, outage, and delivery work orders.
- [Recovery convergence](../session-recovery-convergence/plan.md) remains authoritative for native retirement and independent admission guards.
- [Shell write amplification](../shell-output-write-amplification/plan.md) retains version-2 shell encoding and supported output semantics.

This package extends those implementations. It does not mark their unrelated open delivery tasks complete or replace historical results.

## Tests

Every proposed test name belongs to a work order. RED must fail for the intended defect before production changes.

| Criteria | Planned evidence |
| --- | --- |
| BOUNDED-TOOL-DATA-004.1, 004.4, 004.5 | `TestDeliveryEventLimitBoundaries`, `TestDeliveryEventLimitDoesNotRaiseSubmissionLimit`, `TestDeliveryEventTooLargeDoesNotWaitForACK` |
| BOUNDED-TOOL-DATA-004.3; DURABLE-AGENT-DELIVERY-007.1, 007.2 | `TestDeliveryReplayByteBudget`, `TestDeliveryReplayShortPageReachesTerminal`, `TestDeliveryLimitLegacyCompatibility` |
| BOUNDED-TOOL-DATA-001.1 through 001.3; 004.2 | `TestAllToolDeliverySearchContinuesToTerminal`, `TestAllToolDeliveryCategoriesAndDuplicateCarriers` |
| BOUNDED-TOOL-DATA-001.4, 001.5; 002.1 through 002.4 | `TestAllToolDeliveryBoundedWorkAndSnapshots`, `TestAllToolDeliveryReplayHashCompatibility` |
| BOUNDED-TOOL-DATA-003.1 through 003.3 | `TestAllToolDeliveryExistingRepresentation` and existing-interface desktop/phone regressions |
| DURABLE-AGENT-DELIVERY-001.2 through 001.4 | `TestDeliveryFailureDrainsActualTerminal`, `TestDeliveryFailureTerminalWritePreservesUnknown`, `TestDeliveryHealthStatusBypassesFailedWriter` |
| DURABLE-AGENT-DELIVERY-006.1, 006.2, 006.5, 006.6 | `TestDeliveryHealthFailureWithoutOutput`, `TestDeliveryHealthHealthySilence`, `TestDeliveryHealthRecoveryKeepsIndependentFence` |
| DURABLE-AGENT-DELIVERY-006.12, 006.25, 006.26 | `TestDeliveryHealthRecoveryDoesNotResend`, `TestDeliveryHealthStaleOwnerAndSiblingIsolation`, existing recovery-control E2E |
| DURABLE-AGENT-DELIVERY-007.4 through 007.6 | Mixed-peer capability, genuine storage failure, and retained-instance adoption regressions |

The criteria retain their full `AC-PLATFORM-` prefixes in work-order frontmatter.
No new SQL query, projection contract, or renderer metadata is planned.
Reuse established hash, native retirement, shell patch, and queue-admission regressions.

## E2E tests

Task 03 owns `tests/session/resilient-output-delivery.spec.ts` for `chromium` and
`tests/session/mobile-resilient-output-delivery.spec.ts` for `mobile-chrome`.
Use existing isolated mock-agent/durable-delivery fixtures and real backend processing.
Do not seed only the desired UI state.

1. Near-limit search reaches its actual terminal and admits one distinct next instruction.
2. Larger results retain existing truncation representation and correct status without a new widget.
3. Failed delivery activates the existing recovery card without another event or user action.
4. Existing Retry/Resume/Stop behavior, draft retention, and reload remain correct. Valid recovery never resends the old instruction.
5. Failed terminal persistence or owner verification preserves uncertainty without blocking an unrelated session.

The lifecycle fake-time test proves the ten-second bound.
Phone tests preserve current action reachability and containment. No new layout or control-sizing audit is required.
Backend fixtures cover tool/provider variants without adding browser UI for each category.

## Work orders

- [x] [Task 01: Align event limits and replay budgets](task-01-limits-and-replay.md)
- [x] [Task 02: Prepare every tool report before acceptance](task-02-all-tool-preparation.md)
- [x] [Task 03: Observe writer failure and preserve recovery evidence](task-03-delivery-failure-recovery.md)

## Verification results

The package was narrowed to the existing interface at the user's request on 2026-10-27.

- Specification catalog: passed, with 404 decisions and 1748 specifications.
- Full specification lint: passed.
- Work-order coverage preflight: passed for three work orders and 12 reference documents.
- Changed-document links, whitespace, and inventory: passed, with 16 Markdown files and no code changes.
- The linter's 36 regression tests passed during initial package authoring. Its code did not change.

Production, unit, race, desktop browser, and phone browser checks passed on 2026-10-27.
The changed-file Go lint reported 0 issues. The full `./internal/agentctl/...`
suite and the full orchestrator suite passed. The focused SQLite retention and
delivery suites passed; the complete SQLite package exceeded its test timeout
under host contention, so that package is recorded as partially re-run rather
than fully green.

Review remediation on 2026-10-27:

- The health monitor is owned by the live stream connection, fenced by a
  per-connection attachment token, cancelled on disconnect, replacement, or
  shutdown, and starts when the stream attaches rather than waiting for
  delivery-mode negotiation.
- A committed cancellation terminal resumes the same forwarding loop instead of
  leaving a live runtime without a consumer. A real oversized-write E2E submits
  a distinct next instruction after recovery.
- Sticky writer failures participate in the real reconciler. A live process and
  open socket cannot report recovered, blocked outcomes publish the actionable
  uncertain phase, and a real terminal or cleared failure publishes recovery.
- Display preparation now preflights huge bodies with bounded cost accounting;
  oversized diffs, binaries, todos, and wide content slices avoid full
  serialization and oversized slice reservations.
- Desktop and phone E2E use genuine oversized-output failures rather than
  seeded recovery UI. They exercise Retry, Stop, reload, draft retention, and
  prove the original instruction is not resent.
- After the drain timeout, ordinary output from the failed submission remains
  suppressed; only its terminal or a committed event from a different
  submission releases the sticky failure. Retained peers that report only an
  unhealthy state still enter the existing reconciler.

The implementation is committed and published for review; focused unit, race, desktop, and phone checks passed.

## Risks

- `MaxEventBytes` currently also limits submissions. A one-line increase silently widens prompt admission.
- Journal payloads become base64 inside record envelopes. A raw-text budget alone underestimates replay bytes.
- A short page is not replay completion. Incorrect consumers can miss the actual terminal.
- Typed fields can carry control evidence. Arbitrary JSON truncation can corrupt permissions, lineage, patches, or recovery authority.
- A new event field or metadata contract adds compatibility risk. Reuse existing representations and hash fixtures.
- Old retained producers do not gain preparation by backend adoption. They require honest compatibility and health handling.
- Returning from the native-event consumer can hide the actual cancellation. Terminal write failure still requires uncertainty.
- Status results can arrive after replacement. Every state change needs a captured owner and prompt identity.

## Delivery boundary

All three work orders are complete in this package pull request.
After acceptance, promote the bounded-tool pair and new failure design.
The broader durable-delivery requirement and companion packages retain their independent maturity gates.
No generic review, QA, or full-verification work order is added.
