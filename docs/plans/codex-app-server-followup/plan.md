---
created: 2026-09-24
status: complete
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-005
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-004
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
  - ../../specs/costs/system-design/conversation-usage.md
legacy_specs: []
---

# Implementation plan: Codex protocol follow-up

## Overview

Apply the useful Agent Orchestrator findings after the active implementation session finishes its current work.
The user explicitly requested this follow-up, implementation by that session, and a push to the existing PR.
Finish the current work first. Then reconcile the latest code, implement remaining gaps, test, commit, and push to that PR.
Do not start another task or session, interrupt current work, create another PR, or merge the PR.

This package extends the [original plan](../codex-app-server/plan.md).
Its evidence is in the [comparison report](../codex-app-server/research-agent-orchestrator.md), pinned to external commit `1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5`.
The report describes an earlier working tree. Recheck each finding against the completed implementation before changing code.
When a condition already holds, record its tests and avoid duplicating the implementation.

## Scope

In scope: nonblocking approval handling, provider-offered decisions, request cancellation and resolution, reconnect identity, temporal usage attribution, and protocol-coverage tests.
These refine the existing acceptance criteria. They do not introduce another provider or accounting system.

Out of scope: a second process host, rollout-file billing ingestion, ACP migration, new UI layouts, new-worktree forks, and unverified dollar charges.
Keep the existing feature flag, process ownership, normalized controls, and single ledger writer.
No raw Codex payloads enter React components. No dependency on the external project is required.

## Technical approach

1. Complete approval lifecycle handling in `pkg/codexappserver` and the native adapter. Preserve ordered notifications while approval handlers wait independently.
2. Verify reconnect, resume, and usage identity through existing agentctl and accounting boundaries. Bind measurements to their original model and turn.
3. Add schema-based request coverage and diagnostic fixtures to the shared client and `codexdbg`. Then deliver the verified changes to the existing PR.

Admission for outstanding server requests must be bounded. Reserve request identity before concurrent handling; cancel handlers on resolution or disconnect.
Overload must produce an explicit tested outcome, never silently drop a lifecycle event or accumulate unlimited goroutines.
Store offered decision payloads in the adapter. Send normalized option IDs through existing permission controls and validate selections before replying.
Preserve the distinction between a user declining, cancellation, and provider-side resolution.

An application reconnect can retain the same native RPC client. It must not initialize or resume that client again.
A new native process must initialize and resume the saved thread, without silently starting a replacement conversation.
Do not add native-stream reattachment merely to copy the external project. If current Kandev paths support it, preserve correlation identity across replacement clients.

## Tests

The test names below are proposed; equivalent existing tests can satisfy them if their assertions cover the same cases.

| Work order | Acceptance | Required evidence |
| --- | --- | --- |
| 01 | AC-AGENTS-CODEX-NATIVE-002.2, 002.3; AC-AGENTS-CODEX-NATIVE-003.2 | `client_test.go`: `TestApprovalWaitDoesNotBlockNotifications`, `TestServerRequestAdmissionIsBoundedAndRejectsOverflow`, `TestResolvedServerRequestCancelsOnlyMatchingRequestID`, `TestAnswerWinsRequestResolutionWithoutDoubleResponse`; adapter `adapter_test.go`: `TestOfferedDecisionRoundTrip`, `TestUnofferedApprovalSelectionReturnsInvalidParams` |
| 02 | AC-AGENTS-CODEX-NATIVE-002.1, 002.3; AC-AGENTS-CODEX-NATIVE-003.3; AC-AGENTS-CODEX-NATIVE-004.2 | `process/attachment_test.go`: `TestIsAttachedStaysTrueAcrossAnOverlappingReconnect`; adapter `adapter_test.go`: `TestResumeRestoresNativeChildBindingFromThreadHistory`; scoped replay and fork identity checks |
| 02 | AC-COSTS-CONVERSATION-USAGE-001.2, 001.4, 001.5; AC-COSTS-CONVERSATION-USAGE-002.4; AC-COSTS-CONVERSATION-USAGE-004.3 | adapter `usage_test.go`: `TestDelayedUsageRetainsTurnModelAndGeneration`, `TestDelayedChildUsageRetainsItsParentTurnDuringUnrelatedTurn`, `TestCounterResetDoesNotCreateSpend`; task usage replay integration evidence |
| 03 | AC-AGENTS-CODEX-NATIVE-002.4; AC-AGENTS-CODEX-NATIVE-005.3, 005.5 | `schema_test.go`: `TestServerRequestCoverage`; debugger `request_policy_test.go` and `recorder_test.go`: synthetic approval, replay, and correlation fixtures |

All backend paths in this table are under `apps/backend`.
Protocol tests use fake servers and deterministic synchronization rather than timing sleeps.
No authenticated model turn is needed for these regression checks. Record real-provider checks separately if existing authorization and fixtures permit them.

## E2E tests

Extend the existing fake-server native adapter lifecycle harness to prove an unresolved approval can coexist with child events and later resolve once.
Use the original package's existing desktop and phone approval/question scenarios for user-visible parity.
This package changes protocol behavior beneath those controls, not their rendered layout.
If normalized option or request-state contracts change, extend the corresponding frontend tests and run the existing guarded browser scenarios from the original work order.
Record the exact scenario paths and commands used. Do not claim browser coverage from Go fake-server results.

## Work orders

- [x] [01: Approval concurrency and offered decisions](task-01-approval-lifecycle.md)
- [x] [02: Reconnect and usage identity](task-02-reconnect-and-usage.md)
- [x] [03: Protocol conformance and PR delivery](task-03-conformance-and-delivery.md)

The primary session executed the work orders in order and recorded any incomplete acceptance criteria.
This follow-up does not authorize more implementation subagents.

## Verification results

Planning validation passed: catalog validation (306 decisions, 1146 specifications), full specification lint, and local link, requirement-ID, and whitespace checks for all four files.

Task 02 is complete. An earlier PR fixup on `0a974dec628d13a4addb977b1791e4c244800a6` found a repeated `turn_fallback` string in backend static checks. The backend test shards passed, and the E2E workflow had 11 checks pending when `scripts/pr-await` reached its deadline. Replacing the string with a constant fixed the lint failure; this earlier result is superseded by the exact-head results below.

Task 01 implementation is complete. Direct native question requests now use Kandev clarification controls, with secret questions rejected and provider resolution closing the waiting clarification. Live Codex 0.154.0 question behavior remains unverified.

Task 03 is complete. The direct-question behavior and desktop/mobile screenshots are on PR #3916. After the later plan-status commit exposed a race in `TestLSPContinuityReconnectsToSameTaskHostStream`, the release path now sends the browser acknowledgement before upstream `exit`; the test waits for a signaled lifecycle-fence release. The targeted test passed 50 race-detector repetitions, and the full WebSocket package passed with `-race`.

To resolve the PR's conflict with `main`, commit `4795ed8244184e421d3a7d2a6b8379e1f15628f9` merged the current base and preserved both the LSP release behavior and the newer mainline changes. The merge hook initially found the settings profile page over its line limit; extracting `DeleteProfileCard` kept behavior unchanged and made the changed-file lint pass. The merged WebSocket package race tests, web i18n checks, and focused E2E lint passed. Exact-head CI on `4795ed8244184e421d3a7d2a6b8379e1f15628f9` finished with 60 passed, 0 failed, and 0 pending. The documentation-coverage publisher hit GitHub code-search HTTP 429 several times, then passed after the reported cooldown. The PR has no unresolved review threads and is mergeable/clean; it remains open.

The original Task 07 remains pending. Its live run did not observe an approval request, exact response usage, or child-to-collaboration-call correlation. Docker, SSH, and Kind tests use a fake app-server and do not prove upstream Codex compatibility inside those executors. These limitations are recorded in the original work order; no full Task 07 rerun is claimed.

## Risks

- The current PR can already contain fixes for findings in the report. Recheck before editing.
- Concurrent approval handling can introduce reply-after-resolution races unless request state has one owner.
- Model information can be absent or delayed. Preserve unknown provenance rather than price against an unrelated current model.
- The internal-only response event can be absent. Keep honest fallback and incomplete states.
- The generated schema may not enumerate every envelope method. Use the pinned full protocol source where needed and record that evidence explicitly.
- PR head or merge-queue state can change. Follow the repository push and PR-fixup workflow; never force-push another session's changes.
