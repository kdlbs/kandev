---
created: 2026-10-31
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
legacy_specs: []
---

# Implementation Plan: SSH delivery reattachment

## Overview

Restore the SSH transport before asking it to prove an interrupted prompt outcome. Task 01 covers executor replacement and lifecycle routing. Task 02 corrects the first Retry response when terminal evidence commits during reconciliation. No installed-service replacement or restart is authorized by this package.

## Scope

Include live and saved SSH execution recovery, authenticated owner checks, concurrent retry fencing, and retained replay. Exclude transport-wide policy changes, automatic prompt replay, new conversations, direct SQL repair, remote Stop semantics, other executors, and deployment.

## Technical approach

Follow the SSH section in the linked system design. `RecoverAgentPromptStreamWithIdentity` is the orchestration seam; executor-owned verified transport replacement must precede `ReconcileAgentDelivery`. Avoid replacing a healthy tracked state and avoid startup-only registration against a still-tracked lost transport.

| Provider | Identity/transport | Behavior | Evidence/fallback |
| --- | --- | --- | --- |
| SSH, tracked lost client | Canonical saved SSH owner + authenticated instance | Replace transport, retain stream and cursor | New integrated fault test; otherwise blocked |
| SSH, absent execution | Saved executor record + authenticated remote instance | Reattach before reconciliation | New retained-owner test; missing proof blocked |
| Local/worktree | Existing runtime lease | Existing behavior | Existing lifecycle tests |
| Other remote providers | Existing provider-specific capabilities | No new behavior | Existing tests; no SSH fallback |

## Tests

AC .4: real fake-SSH transport loss with running and completed journal submissions; same remote process and native session; one tool marker; original stream/cursor; no ACP start/load/resume or prompt dispatch. Existing SSH process-adoption tests are useful baselines but do not cover the full live Retry path.

AC .16: simultaneous retries, late old-stream acknowledgment, retry cancellation, host-key rejection, mismatched task/session/instance/generation, missing journal, absent execution, local-provider routing, and later retry after failure.

## E2E tests

Use a new disposable Kandev task and an independent backend/data directory/port plus a test-only TCP relay. Never point the test backend at an existing database. Complete a command during a 15-second SSH disconnect, invoke the existing Retry connection action, and verify same conversation, exactly one synthetic marker pair, authoritative settlement, projected tail, and a distinct follow-up. Recheck all original session IDs/states and backend PID. Test repeated Retry and unavailable evidence without clearing blocks. Both scoped live acceptance stages passed through the existing WebSocket actions; browser rendering was not separately tested.

## Work orders

- [x] [Task 01: Restore owned SSH transport before delivery proof](task-01-restore-transport.md)
- [x] [Task 02: Report committed terminal settlement on the first Retry](task-02-report-settled.md)

## Verification results

Task 01 is done. Exact work-order tests, focused race tests, Go vet, changed-package lint, specification catalog validation, 36 linter self-tests, full specification lint and diff whitespace checks passed. Live macOS SSH fault acceptance passed using a new task and independent backend: same controller/native conversation, one command execution, retained terminal settlement, repeated Retry and a successful distinct follow-up. Existing backend and task/session states were preserved. All test processes were stopped normally.

Task 02 is also done and removes the first-response timing limitation recorded by Task 01. A deterministic real-SQL race test and real macOS SSH fault acceptance both returned settled on the first Retry with the current canonical revision, while preserving ownership and independent-block checks. A repeated Retry and distinct follow-up passed. UI rendering was not separately verified. The broader durable-delivery requirement document remains draft; this work verifies only its scoped disconnect-recovery criteria and does not promote every proposed guarantee. See the linked work-order results.

## Risks

The installed binary reports v0.98.0 but does not expose a build revision; the official v0.98.0 source is a version-matched reference, not proof of identical build contents. The old synthetic controller was intentionally terminated after evidence capture, so it cannot demonstrate live reattachment now. Runtime-aware SSH recovery must not fall back to a local journal or interpret remote unavailability as death. Installing a fix into the active app would require a separate planned maintenance step.

## First-response correction continuation

Task 02 reuses REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, AC .5 and .16 and the same system design. After runtime reconciliation returns, refresh the canonical evidence for the exact original recovery binding. A committed matching terminal settlement wins over a transient runtime owner-mismatch response. Missing terminal proof, a newer generation, changed execution/stream/submission, or an independent recovery block must not be converted to settled. No polling budget, schema, UI, prompt dispatch or installed-service change is introduced.


Task 02 verification: exact Retry regression command 2.467 s, race scope 4.491 s, changed-package lint 0 issues, Go vet passed, and full specification checks passed. Live acceptance preserved existing tasks/sessions and configuration hashes. The tested binary was not installed into the existing app.
