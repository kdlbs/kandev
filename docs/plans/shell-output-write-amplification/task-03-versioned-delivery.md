---
id: "03-versioned-delivery"
title: "Activate versioned end-to-end delivery"
status: done
wave: 3
depends_on:
  - "01-output-codec"
  - "02-canonical-storage"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
acceptance_criteria:
  - AC-PLATFORM-INCREMENTAL-SHELL-001.1
  - AC-PLATFORM-INCREMENTAL-SHELL-001.2
  - AC-PLATFORM-INCREMENTAL-SHELL-001.3
  - AC-PLATFORM-INCREMENTAL-SHELL-001.4
  - AC-PLATFORM-INCREMENTAL-SHELL-001.5
system_design:
  - ../../specs/platform/system-design/incremental-shell-delivery.md
---

# Task 03: Activate versioned end-to-end delivery

## Summary

Connect the codec and canonical projector through mutually supported v2 delivery, keeping v1 retained-runtime recovery. The complete vertical path must work before compact production events are enabled.

## In scope

- Add protocol-v2 negotiation, journal-format-v2 validated startup upgrade, and explicit old-version readers. Audit all `journal.CurrentVersion` comparisons in client/lifecycle/retention and every stream/replay/submission entry point.
- Encode selected foreground views before writer serialization; maintain accepted baselines and checkpoint cadence across interleaved tools. Strip only duplicate recognized carriers. Preserve first/quiet/terminal/barrier ordering and semantic foreground activity.
- Receive compact inbox bodies and dispatch transactional canonical projection. Skip only proved duplicate message persistence; retain lifecycle, late-terminal, permission/background and completion behavior. Propagate projection errors into recovery.

## Out of scope

Task 01 codec and Task 02 canonical projector/storage must already pass; no full-output fallback may silently hide a v2 projection failure.

## Acceptance

- `TestIncrementalShellSlowStorageBound` fails on the current snapshot path, then passes byte budgets and equal normalized final output through real writer/inbox/projector boundaries; fast and interleaved cases pass too.
- `TestIncrementalShellRetainedRuntimeCompatibility` / `TestIncrementalShellUnsupportedReader` cover new backend+v1/v2, old reader rejection on every data path and unresolved submission preservation. No unsafe legacy fallback or automatic runtime replacement occurs.
- Crash/commit retry, lost ACK, missing baseline, terminal replacement/cancel, huge escaped checkpoints and post-grace replay preserve order, hashes, effects, output and Stop.

## Verification

Run from the repository root. Add the regression tests and show their expected
red result before making production changes. Commands below define the required checks; actual results are recorded below. Run PostgreSQL only against an isolated configured test database.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/types/streams ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -run 'Shell|CommandEcho|Background|Permission' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'Delivery|Durable|IncrementalShell' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'ToolUpdate|LateTerminal|IncrementalShell' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'IncrementalShell|AgentDelivery' -count=1)
```

Run `bash scripts/lint-go-changed` with every changed Go file in this task before
completion, plus `git diff --check`. Record those exact paths/results here.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/{shell_snapshot.go,adapter_tools.go,adapter_updates.go}` and focused tests
- `apps/backend/internal/agentctl/server/process/{delivery_batch.go,delivery_pressure.go}` and new shell-encoding tests
- `apps/backend/internal/agentctl/journal/{journal.go,recovery.go,location.go}` and upgrade/replay tests
- `apps/backend/internal/agentctl/server/api/{durable_delivery.go,agent.go}` and endpoint tests
- `apps/backend/internal/agent/runtime/agentctl` event decoding/client negotiation
- `apps/backend/internal/agent/runtime/lifecycle` version negotiation/adoption/preflight/ACK/recovery/retention and stream projection
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` and focused tests
- `apps/backend/internal/task/repository/sqlite/agent_delivery_shell_projection.go`

## Dependencies

01-output-codec, 02-canonical-storage.

## Risks

Version equality checks are distributed. Terminal substantive-output behavior currently depends partly on contents, which compact carriers must replace semantically.

## Parallelism

`sequential`. The primary session owns integration and shared status updates.

## Inputs

- [Requirement](../../specs/platform/requirements/incremental-shell-delivery.md).
- [System design](../../specs/platform/system-design/incremental-shell-delivery.md).
- [Plan](plan.md), including diagnosis, compatibility matrix and test names.
- [Decision record](../../decisions/2026-10-19-incremental-shell-delivery.md).
- Existing `adapter_shell_snapshot_test.go`, `shell_output_test.go`,
  `agent_delivery_test.go`, `message_payload_test.go` and `durable_delivery_stream_test.go`
  patterns in their respective implementation packages.

## Results

Implemented and verified. Final browser rebuild and documentation checks are recorded in Task 04.

The writer now emits compact accepted revisions, progress checkpoints and immediate terminal checkpoints, preserves interleaved baselines, and suppresses output no-ops before checkpoint selection. Baselines advance only with journal acceptance. The SQL projector commits output, summary, effect and cursor together, and lifecycle callbacks publish the committed message instead of writing a second full snapshot.

Protocol and journal v2 guards preserve old records and allow new backends to consume retained v1 runtimes. All production comparisons now use explicit supported protocol checks, including orchestrator admission and restart reconciliation. New v1 idle/non-durable tests failed against the previous equality checks, then passed after correction. Unsupported readers retain Status/Stop but cannot stream compact output or admit submissions.

Passed checks:

- ACP race regressions: `-run 'Shell|CommandEcho|Background|Permission'`, 13.705s.
- Runtime agentctl/lifecycle race regressions: `-run 'Delivery|Durable|IncrementalShell'`, 1.101s / 23.357s.
- Orchestrator race regressions: `-run 'ToolUpdate|LateTerminal|IncrementalShell|PrepareAgentDeliverySubmission|RestartSubmission|LegacyNative'`, 7.961s.
- Task SQL race regressions: `-run 'IncrementalShell|AgentDelivery'`, 143.915s, including the full endurance fixture.
- Journal migration/reader tests include an injected rollback immediately before commit and preserve the v1 version header and records; targeted race run passed in 1.062s.
- `go run ./cmd/sqlguard ./internal`: passed.

Full API/process tests use a disposable HOME with the existing Go caches and installed Node/npm on PATH: the host login profile references a missing cargo file, and default PATH omits npm. No user profile was edited. Interrupted runs without completion are not passing evidence.


Final results:

- Full race run of `types/streams`, `journal`, `server/process`, and `server/api`: passed (2.188s, 21.429s, 368.215s, 143.717s respectively). The only later production edit prevents checkpoint text from being counted as fresh output progress; the targeted current-source race run passed no-op, boundary and both-saturated-field cases in 5.458s. The latter covers 1,100 revisions and three checkpoint intervals under 4 MiB.
- `TestIncrementalShellTwoSaturatedFieldsDoNotReseed` first failed because the second revision was another full checkpoint. Excluding checkpoint bytes from the next progress counter fixes the two-field lifetime bound; stdout-only results are unchanged.
- Changed-Go lint across all modified/new Go files passed with zero issues. After later test/counter edits, focused lint passed for the journal migration test, endurance test and producer implementation/tests. SQL guard passed.
