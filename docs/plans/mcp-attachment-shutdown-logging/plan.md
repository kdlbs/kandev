---
created: 2026-10-11
status: implemented
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
system_design:
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
legacy_specs: []
---

# Implementation plan: MCP attachment shutdown logging

## Overview

Classify late MCP diagnostic writes rejected by an intentionally closed durable
writer as expected shutdown. Keep real journal failures at error level and
preserve the current commit-before-publish and shutdown boundaries. One
sequential work order implements and tests the producer-side classification.

Platform owns this repair because it owns runtime diagnostics and shutdown
semantics. No rendered UI or public command contract changes.

## Original diagnosis (2026-10-11)

- Baseline: diagnostic bundle `16eab9746ff81c5b2be99d4da9a5655d`, runtime commit
  `e6692746b0b825c543e6c5f5fb3cb183d65c24bd`, collected on 2026-10-11.
  Its manifest is ready with no source warnings and no file-sink loss.
- All five matching entries in today's retained backend log follow intentional
  stop and `closing ACP adapter`. The reported 14:22:47.854 Lisbon-time entry
  follows `task tree deleted` at 14:22:47.583 and adapter close at 14:22:47.850.
- `Manager.closeAdapterAndStdin` closes `deliveryEventWriter` before the adapter.
  The MCP server's reporter calls `Manager.PublishMCPAttachment` for stateful
  connection closure. This late observation can encounter the closed writer.
- A temporary `TestReproMCPConnectionClosedAfterDeliveryWriterShutdown`
  committed an initial observation, executed `closeAdapterAndStdin`, and
  published connection-close evidence. It confirmed the error field was
  `durable delivery writer is closed`, while the journal remained available.
  The test passed with `go test -trimpath ./internal/agentctl/server/process
  -run '^TestReproMCPConnectionClosedAfterDeliveryWriterShutdown$' -count=1 -v`.
  The temporary test was removed after diagnosis; this is reproduction evidence,
  not implementation validation.
- Older retained files contain 49 attachment event/attempt errors, including
  startup failures. Their underlying errors are unavailable in the forwarded
  JSON envelopes; this shutdown reproduction does not establish their cause.
- The launcher deliberately forwards only recognized JSON envelope fields.
  Its severity is correct; expanding that filtering is outside this repair.

## Recheck against advanced main (2026-10-12)

The initial implementation base was `8d662b1f564fb1e0a49c90f412903e6d92d49ae2`.
Before delivery the branch was refreshed to `fd79be40a7dc6c902f49fe9df649365fc6613352`;
that additional main commit did not alter the process package or these contracts.
Merged #4432 moved writer closure out of `closeAdapterAndStdin` into the
`forwardUpdates` defer, after terminal-update draining. The prior reproduction
and ordering description above are historical evidence only. Preserve the new
ordering; reproduce the remaining late callback through real `Manager.Stop`.

A fresh ready backend bundle `e96e2ac7fde11573280e7b6591efb2c7`, collected at
12:21 UTC, identifies runtime `a789b16d70358a159103c5cb0168017d63b8dfce`.
It still records MCP commit errors during archive stops at 10:58:25.492 and
11:35:18.415 Lisbon time. Child JSON filtering still hides their raw error fields.
Both MCP publisher sites remain unconditional ERROR on the refreshed source.
The narrow classifier is still needed and must preserve #4432. A second real
`StopForTeardown` regression showed ERROR for both `context canceled` during
adapter Close and `database not open` after the manager closed its journal.
Both publisher methods reproduce these failures. Classify them only with the
manager's own canceled lifetime and clean journal-close evidence.

## Scope

### In scope

- Implement AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19 through .21 in
  both MCP publisher entry points.
- Prove typed closure plus stopping/stopped state classification with observer
  tests and exercise restart, journal availability, and genuine-error controls.

### Out of scope

- Changing writer close ordering, journal persistence, ACK, replay, prompt
  admission, agent error delivery, or session transitions.
- Changing launcher JSON filtering, adding raw child error forwarding, or
  investigating unrelated historical startup failures in this patch.
- Restarting or mutating the user's live instance.

## Technical approach

Use a shared, narrow logging helper at `PublishMCPAttachment` and
`PublishMCPAttachmentAttempt` in `internal/agentctl/server/process/manager.go`.
Extract the helper into `mcp_attachment_logging.go` to keep the change small.
It requires stopping/stopped status and either a typed closed writer or a
rejection tied to permanent manager shutdown: exact cancellation with the
manager lifetime canceled, or an exact closed-journal error with canceled
lifetime, nil journal, the manager's closed marker, and no close failure.
These expected rejections receive a debug entry. All other failures retain the existing error-level message and
error field. Preserve the return on failed persistence and nonblocking update
publication. Do not acquire `Manager.mu` in the helper.

| Runtime path | Expected behavior | Evidence |
| --- | --- | --- |
| Durable, stopping/stopped, closed writer | Debug rejection, no uncommitted publication | Both publisher methods and lifecycle reproduction |
| Durable, running/starting, closed writer | Existing ERROR | Observer control cases |
| Permanent teardown, canceled lifetime or clean owned journal close | Debug rejection, no reopen | Real StopForTeardown callback regression |
| Durable, any status, storage or unrelated cancellation failure | Existing ERROR | Typed and combined-error controls |
| Legacy, no durable journal | Existing update delivery | Publisher regression |
| Restarted durable manager | New evidence commits through the ordinary writer | Restart regression |

The boundary is provider-neutral. The incident used an ACP adapter and a
stateful MCP connection; stateless HTTP does not fabricate a connection-close
event. Unknown error shapes remain errors.

## Tests

`mcp_attachment_logging_test.go` owns the new regression suite:

- `TestMCPAttachmentShutdownClosedWriterLogsDebug`: initial committed evidence,
  actual writer shutdown, then late connection-close observation; no error log,
  no extra committed event, original record still replayable.
- `TestMCPAttachmentCommitErrorClassification`: both publishers, stopping,
  stopped, starting, and running states; closed-writer and unrelated errors.
- `TestMCPAttachmentPermanentTeardownLogsDebug`: callback during adapter close
  and after permanent journal close, for both publishers; resources remain closed.
- `TestMCPAttachmentCanceledShutdownKeepsStorageFailures`: real corruption,
  exhaustion, deadlines, and combined failures retain ERROR even with a canceled lifetime.
- `TestMCPAttachmentLegacyPublicationUnchanged`: no-journal event/attempt delivery.
- `TestMCPAttachmentPublicationAfterWriterRestart`: return to the supported
  running writer path after stop; evidence commits with sequence continuity.

Run the existing full-journal writer-close and closed-journal tests as part of
the process package. No browser test is required for this logging-only repair.

## Work orders

- [x] [Task 01: Classify late MCP shutdown writes](task-01-classify-shutdown-writes.md)

## Verification results

The original temporary diagnostic reproduction passed and was removed.
Design-package validation on 2026-10-11:

- `python3 scripts/list-docs.py validate`: passed (374 decisions, 1525 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; the plan and work order are present as untracked files.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: passed
  (`covered`) using the package's four documents and the prospective
  `manager.go` implementation path. This validates references, not production
  implementation. The shell lacks `node` on PATH; the existing mise-managed
  Node binary was used directly, with no dependency installation.
- Owned temporary extraction removed; original diagnostic ZIP retained.

The design turn made no production or permanent test changes and did not mutate the live instance.
Implementation began on 2026-10-12 after the user requested delivery through merge.

### Implementation validation (2026-10-12)

Task 01 is done. The final isolated-HOME process-package race suite passed
(267.163s). All six MCP regression functions are included. Package lint passed
with zero issues against refreshed main, and catalog validation, specification
lint, diff checks, and exact-path PR documentation coverage passed.

The ambient-HOME run's two existing runner readiness failures were traced to
a login profile sourcing missing `/home/jcfs/.cargo/env`; both passed with a
fresh test home. No user profile or unrelated code was changed. The fresh
home is a disposable test fixture, and Go caches remain at their original paths.

The real permanent-teardown regression established the additional cancellation
and clean owned journal-close cases before their classifier was implemented.
Unrelated, combined, and active-process errors retain ERROR. This repair leaves
main's terminal-drain-before-writer-close ordering intact.

See [Task 01 results](task-01-classify-shutdown-writes.md#results) for exact
commands. Commit hooks, remote CI/review, and merge remain external gates.

## Risks

- Matching all canceled writes or all stopped statuses would hide real storage
  failures. Require shutdown status and typed rejection plus the matching manager shutdown evidence.
- Changing main's drain-before-close ordering can lose terminal evidence or stall shutdown under delivery pressure.
- Historical startup failures remain unclassified; do not promise this patch
  fixes every occurrence of the same outer log message.
