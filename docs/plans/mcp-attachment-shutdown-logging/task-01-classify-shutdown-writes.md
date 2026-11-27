---
id: "01-classify-shutdown-writes"
title: "Classify late MCP shutdown writes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
acceptance_criteria:
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.20
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.21
system_design:
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
---

# Task 01: Classify late MCP shutdown writes

## Summary

Use TDD to reproduce late MCP connection-close evidence after durable writer
shutdown, then classify only the expected rejection at debug level. Share the
classification between evidence and attempt publishers and retain genuine
error logging and the existing delivery boundary.

## In scope

- Add `TestMCPAttachmentShutdownClosedWriterLogsDebug` first and show it fails
  because the late callback currently emits an ERROR with the typed closed
  writer error. Use a temporary journal and an observer-backed logger.
- Extract a narrow producer logging helper and call it from both MCP publishers.
- Implement the control, legacy, and restart tests named in the plan.
- Keep comments about invariants only; keep incident history in this package.

## Out of scope

Writer/journal lifetime changes, generic durable delivery error classification,
launcher filtering, browser UI, metrics, and live-instance mutations.

## Acceptance

1. Both MCP publishers meet AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19
   with debug rejection only for stopping/stopped state plus typed writer closure
   or a cancellation/clean journal-close rejection tied to this manager's permanent
   shutdown, preserving the return without event publication.
2. Active-process closed-writer failures, combined failures, and unrelated journal or cancellation
   errors remain ERROR under .20; the classifier never acquires `Manager.mu`.
3. The Stop and StopForTeardown regressions, legacy/restart controls, and existing full-journal
   shutdown regression pass under the race detector, establishing .21.

## Verification

Run from the repository root:

```bash
mcp_test_home=$(mktemp -d)
mcp_test_cache=$(cd apps/backend && go env GOCACHE)
mcp_test_modules=$(cd apps/backend && go env GOMODCACHE)
(
  cd apps/backend && env HOME="$mcp_test_home" XDG_CONFIG_HOME="$mcp_test_home/.config" \
    GOCACHE="$mcp_test_cache" GOMODCACHE="$mcp_test_modules" \
    go test -trimpath -race ./internal/agentctl/server/process -count=1
)
mcp_test_result=$?
rm -rf "$mcp_test_home"
test "$mcp_test_result" -eq 0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For red/green evidence, first run the new regression alone with
`go test -trimpath ./internal/agentctl/server/process -run '^TestMCPAttachmentShutdownClosedWriterLogsDebug$' -count=1 -v`
from `apps/backend`. Record the actual failure before adding the helper.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/mcp_attachment_logging.go`
- `apps/backend/internal/agentctl/server/process/mcp_attachment_logging_test.go`
- This work order and `plan.md` for status and exact verification results.

## Dependencies

None. Existing private `errDeliveryWriterClosed` and atomic `Manager.Status`
provide the classification boundary; no schema or transport change is needed.

## Risks

- Never downgrade based only on error text, cancellation, or stopped status.
  Permanent journal/cancellation rejections require this manager's own shutdown
  evidence; combined errors and close failures remain ERROR.
- Preserve main's adapter-drain-before-writer-close ordering; do not reopen the writer for late callbacks.
- `StatusStopped` can follow natural process exit. The typed closed-writer
  condition is required even in that state; unrelated errors remain visible.
- Restart coverage must assert persisted sequence continuity, not merely a log.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/expected-runtime-log-severity.md),
  AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19 through .21.
- [System design](../../specs/platform/system-design/expected-runtime-log-severity.md),
  MCP observations during shutdown and test strategy.
- `manager.go`: MCP publisher methods, `persistDeliveryEvent`,
  `closeAdapterAndStdin`, and `StopForTeardown`.
- `delivery_batch.go`: closed-writer sentinel and writer lifecycle.
- Existing `delivery_pressure_test.go` and `manager_delivery_shutdown_test.go`.
- Scoped backend guidance and `/tdd` backend test patterns.

## Results

Implemented on 2026-10-12 against refreshed main `fd79be40a7dc6c902f49fe9df649365fc6613352`.

- Red: real `Stop` regression failed for both publisher methods because the
  closed-writer rejection was ERROR. Real `StopForTeardown` also failed for
  callbacks during adapter close (`context canceled`) and after owned journal
  close (`database not open`).
- Green: all six MCP test functions pass, including permanent teardown,
  replay/no-publication, restart continuity, legacy delivery, and genuine
  failure controls with canceled manager lifetime.
- Final full package: isolated-HOME `go test -trimpath -race
  ./internal/agentctl/server/process -count=1` passed (267.163s).
  The first ambient-HOME run failed two existing runner readiness tests because
  the host login profile sources a missing `/home/jcfs/.cargo/env`. Both passed
  in isolation; no host profile or unrelated production code was changed.
- `golangci-lint run ./internal/agentctl/server/process
  --new-from-rev=fd79be40a7dc6c902f49fe9df649365fc6613352 --timeout=5m`: passed,
  zero issues after the final production change.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local PR documentation coverage evaluator: `covered`, with the exact changed
  work order, requirement, design, and runtime paths.
- Public docs and screenshots are unnecessary: only internal diagnostic
  severity changes. Writer draining, storage failure handling, journal lifetime,
  and shutdown control flow are preserved.

Publication, CI/review evidence, and merge remain external delivery gates.
