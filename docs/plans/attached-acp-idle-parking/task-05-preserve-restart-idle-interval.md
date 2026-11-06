---
id: "05-preserve-restart-idle-interval"
title: "Preserve the idle interval across backend restart"
status: completed
wave: 5
depends_on: ["04-resume-safe-idle-reclaim"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.6
  - AC-EXECUTORS-IDLE-PARKING-001.7
  - AC-EXECUTORS-IDLE-PARKING-001.8
  - AC-EXECUTORS-IDLE-PARKING-001.10
  - AC-EXECUTORS-IDLE-PARKING-001.11
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 05: Preserve the restart idle interval

## Outcome and scope

Preserve trustworthy lifecycle activity across same-session backend recovery. Passive runtime, session, and transcript timestamps must not restart the activity interval. The workspace revision retains its existing role as the conservative policy-generation fence and interval floor. Unknown recovery history receives one full interval after settled observation. Explicit focus remains an execution-bound interval floor across restart.

## Responsibilities

- Persist the execution-bound lifecycle clock through normal runtime inventory upserts. Restore it before local or remote replay using the authoritative lifecycle record.
- Retained terminal outcomes carry their original observation time. Durable replay uses journal `CreatedAt`, while preserving payload identity, projection, acknowledgement, and deduplication.
- Unknown replay may clear uncertain restored history, but must preserve activity observed live by the current backend, including prompt, steering, focus, and agent activity.
- Persist the explicit-focus floor through existing session metadata, bound to the current execution. Passive metadata writes are not semantic activity. Sibling or replacement executions cannot inherit the floor.
- Preserve policy/focus floors, active-prompt protection, accepted and known work, runtime ownership, resume tokens, task compute, and LSP lease fences. Routine scan skip summaries remain at Debug; suspension outcomes remain at Info.
- Add permanent package regressions using normal SQLite persistence, actual startup and remote reconstruction, the retained-outcome completion path, and real bbolt/SQLite durable delivery.
- No new table, journal, endpoint, provider allowlist, UI, runtime setting, or database write for each agent event.

## Acceptance

1. Normal lifecycle persistence followed by `Manager.Start` preserves real activity despite passive metadata updates; an explicitly removed clock stays absent on the next startup.
2. Remote reconstruction uses the persisted lifecycle owner clock even when adapter metadata contains only connection configuration.
3. Missing, malformed, wrong-execution, zero, and future snapshots do not become trustworthy history. Older replay cannot replace newer activity; unknown replay cannot erase current-backend live activity.
4. Retained and durable terminal replay preserve observation time, output evidence, payload identity, projection-before-ACK, and duplicate suppression.
5. Unknown-history candidates receive a stable full interval. Policy changes and explicit focus preserve their floors, including focus before a backend restart. Existing prompt, work, lease, ownership, and resume-safety guards remain effective.
6. Permanent tests and final-source hosted verification must pass before delivery.

## Verification

Run on hosted execution only for this review. From `apps/backend`:

- `go test -trimpath -tags fts5 -race -p 2 -count=1 -timeout=10m -v ./internal/agent/runtime/lifecycle -run '^(TestIdleActivity|TestApplyRecoveredTurnOutcome|TestGetPromptActivityForSession|TestExecutionStore_ClaimPromptActivity|TestSuspendIdle)'`
- `go test -trimpath -tags fts5 -race -p 2 -count=1 -timeout=10m -v ./internal/orchestrator -run '^(TestIdleParking|TestPromptAfterIdleReclaimLaunchesPreparedSession)'`
- `go test -trimpath -tags fts5 -race -p 2 -count=1 -timeout=10m -v ./internal/agentctl/server/instance -run '^(TestRetainedOutcomeActivity|TestTurnOutcome)'`
- `go test -trimpath -tags fts5 -race -p 2 -count=1 -timeout=10m -v ./internal/orchestrator/executor -run '^TestIdleActivityMetadata'`
- Full final-source backend, frontend, and E2E workflows, including their selected Windows, PostgreSQL, containers/Kubernetes, desktop/native, and artifact contracts.
- Existing specification metadata/lint and public-documentation checks on the final source.

## Results and provenance

The original contribution reported temporary overlay checks and targeted local checks, then removed the overlays. Those historical author reports are not permanent regression coverage or final-head hosted proof. The original work order's instruction to exclude permanent tests has been superseded by current repository and review requirements.

Independent review identified missing permanent coverage, loss of newer live activity on unknown replay, and loss of the explicit-focus floor on backend restart. The scan-level Info advisory is addressed by retaining routine skip summaries at Debug. All 21 new permanent top-level regressions passed on committed application source `b1eb394e8dad2b7a1269734f7647b4b6219c3f2e` in [hosted focused GREEN run 39081986921](https://github.com/kdlbs/kandev/actions/runs/39081986921). The four Go packages ran with `-race`; documentation catalog, specification lint, and public-documentation validation also passed. The tested artifact and source archive are retained with verified content hashes and ZIP CRC.

Hosted RED run 39077958720 established 15 live-activity replay assertion failures with passing controls. Run 39080767441 established the focus-floor assertion failure after a passing live-focus control. Earlier nil-executor, repository-interface, stale-row, missing delivery-owner, and missing ACK-descriptor fixture failures are retained as failed attempts and are not product RED. The completed tests use the real journal owner descriptor and assert SQL projection at the actual ACK boundary.

Full backend, frontend, and E2E verification on the published final head remains a delivery gate. [PR 4510](https://github.com/kdlbs/kandev/pull/4510) records those actual workflow results and the final review dispositions. No contributor code, tests, hooks, or dependency installation ran on the review host.
