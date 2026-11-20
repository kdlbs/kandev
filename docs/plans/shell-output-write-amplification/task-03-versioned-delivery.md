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

### PR remediation

A focused self-check found that compact title updates changed metadata but left
message content at the initial title, unlike the legacy tool-message updater.
`TestIncrementalShellProjectionUpdatesTitleAndContent` first failed with the
initial content instead of the new title. The canonical transaction now updates
content for non-empty titles and preserves it for empty-title updates, including
metadata-only revisions. The targeted SQLite race tests passed with two Go
workers. This preserves the approved projection contract; it adds no wire or UI
behavior. Remote CI/review and merge evidence remain external until completed.

The same review pass reproduced stale output when an ordinary/background result
followed a compact foreground result. Ordinary complete replacements now retire
the chunk head and reference in their existing message transaction; projected
summary-only updates retain their reference. Both direct and conversation-receipt
writers have rollback coverage. The producer clears the accepted baseline at
that ordinary normalized boundary, so later compact output starts a fresh epoch.
These cases initially failed against the old reference/baseline handling.

Remediation checks use `GOMAXPROCS=2` and `-p 2`: targeted SQLite race tests
passed in 5.333s; producer ordinary-boundary, no-op, two-field and interleaving
race tests passed in 9.672s. Isolated PostgreSQL duplicate projection and both
ordinary-writer paths passed in 3.911s; that fixture was limited to one CPU and
512 MiB and was stopped immediately afterward. The focused title lint and SQL
guard passed. Final combined lint, remote findings and exact-head CI remain
pending until the remediation batch is published.


The delivery review additionally reproduced a missing canonical card before an
asynchronous subscriber ran, mixed PostgreSQL metadata/output snapshots, extra
checkpoints driven only by tool metadata, and rejection of fresh checkpoints at
an identity change. Foreground starts now commit their card before cursor
advancement and callbacks only publish it. Detail reads use REPEATABLE READ;
identity changes admit only a self-contained base-zero checkpoint. Metadata-only
updates preserve the checkpoint progress counters. Each behavioral fix has a
focused failing regression followed by a passing check.

Removal now revokes valid canonical references even for empty or seven-byte
results whose tombstone exceeds their body; model and scanner regressions first
failed with retained output and now pass. Byte-savings accounting remains
nonnegative. The UI requirement distinguishes retained v1 storage from canonical
v2 chunks, and the shared desktop/phone E2E additionally asserts final exit
status and absence of earlier live output after reload. This adds no product UI
or public API behavior; existing public operations/session documentation remains
applicable.

The CI WebSocket header assertion is updated to require the v2 negotiation
header while preserving absent Authorization for unauthenticated clients. The
original CI artifact reproduces the obsolete nil-header expectation.
Remote thread dispositions, exact-head CI and merge remain pending until the
remediation commit is published and checked.


A direct PostgreSQL replacement with projected metadata also reproduced orphaned
canonical chunks: the incoming metadata intentionally omits the private storage
marker. All direct message updates now use the existing stored-row guard, matching
the conversation-receipt path, rather than deciding admission from an incoming
private marker. The PostgreSQL replacement regression covers that omission.

Canonical shell history is detected from the stored reference or its removal
receipt under the row lock. Direct PostgreSQL legacy-message replacements retain
their existing behavior; conversation-receipt and agent-plan writes keep their
existing removal guards. PostgreSQL regressions cover both legacy replacement
and non-resurrection of removed canonical output.


Current-source remediation checks passed with `GOMAXPROCS=2` and `-p 2`:
producer/model/scanner race checks; WebSocket negotiation headers; SQLite and
isolated PostgreSQL projection, replacement, snapshot, replay and removal
regressions; lifecycle `Delivery|Durable|IncrementalShell` and orchestrator
`ToolUpdate|LateTerminal` cases. The final writer-boundary SQL pass took 15.489s;
after the identity helper extraction its focused race pass took 10.829s. The
lifecycle and orchestrator package checks took 21.637s and 33.147s. Persistence
store conformance passed in 316.697s before the final writer-only refinements;
schema and initialization were unchanged by those refinements. SQL guard passed.

Lint found an over-limit projector and a repeated status literal. Identity
reconciliation was extracted without changing behavior, and the terminal status
uses a constant. The earlier combined lint run also exceeded five minutes; the
same 79-file scope is being checked in package groups before publication.


Final changed-Go lint passed with zero issues across all 79 files in
`verification-go-files.txt`, using the repository helper in three sequential
package groups: SQLite (20 files, 47.06s), runtime/orchestrator (34 files,
216.74s), and agentctl/support (25 files, 8.89s). The grouping retains the full
14-package scope while keeping each run within the helper's timeout. Two Go
workers and the default memory target were retained; no linter or hook was
bypassed. The role/status refactors replace identical strings with constants. Remote CI/review disposition remains pending publication.
