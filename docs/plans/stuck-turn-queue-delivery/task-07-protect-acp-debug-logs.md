---
id: "07-protect-acp-debug-logs"
title: "Protect ACP debug logs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-001
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-002
acceptance_criteria:
  - AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-001.1
  - AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-001.2
  - AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-002.1
  - AC-PLATFORM-ACP-DEBUG-LOG-RETENTION-002.2
system_design:
  - ../../specs/platform/system-design/acp-debug-log-retention.md
---

# Task 07: Protect ACP debug logs

## Summary

A file with an open writer survives the count cap, and agent subprocesses and ACP tests no longer write to the real log directory.

## In scope

- Exclude open-writer files from count-cap deletion.
- Remove ACP debug settings from agent environments through one shared predicate.
- Add a `TestMain` to the `acp` test package.
- Update the logging section of `apps/backend/internal/agentctl/AGENTS.md`.

## Out of scope

- Default limits and rotated-segment grouping.
- Coordinating writers across separate agentctl processes.

## Acceptance

- A burst past the limit keeps an open writer's file and deletes the oldest other files.
- Agent environments from all entry points omit the settings, and agentctl keeps logging.
- Running `acp` tests with the flag on adds nothing to the real directory.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/shared ./internal/agentctl/server/config ./internal/agentctl/server/utility ./internal/agentctl/server/acp -count=1)
(cd apps/backend && KANDEV_DEBUG_AGENT_MESSAGES=true go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'AsyncTurn' -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/shared/acplog.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/acplog_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/acpjanitor_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testmain_test.go`
- `apps/backend/internal/agentctl/server/acp/terminal.go`
- `apps/backend/internal/agentctl/server/acp/terminal_test.go`
- `apps/backend/internal/agentctl/AGENTS.md`

## Dependencies

None

## Risks

- A mock agent or nested tool could depend on the inherited flag. Task check: search for readers before removing it.

## Parallelism

`sequential`

## Inputs

- [Retention design](../../specs/platform/system-design/acp-debug-log-retention.md)
- `shared/testmain_test.go`

## Results

Done.

- Age and count retention atomically skip the active segment held by a writer
  registered in the same agentctl process. That segment still counts toward the
  limit. Rotated segments and files written by another agentctl sharing the
  directory are outside this guarantee.
- `launchwrapper.IsAgentDebugSetting`/`StripAgentDebugSettings` define one
  predicate used by `CollectAgentEnvWithoutAdditional`,
  `collectAgentEnvWithError` (including explicit overrides), and
  `sanitizeEnvForAgent`.
- The `acp` package's `TestMain` redirects the shared debug-log registry to a
  temporary directory through `shared.SetDebugLogDirForTest` and exits before
  tests run if the temporary directory cannot be created.
- Agent debug settings are removed from ACP terminal child processes, including
  explicit environment overrides, as well as agent-config and probe children.
- `TestACPLogPruneWriterRegistrationIsAtomicWithRemoval` races pruning against
  writer registration and confirms a concurrent frame remains in a managed log.
- `acpjanitor_test.go` covers an open writer as the oldest file and open writers
  alone exceeding the limit; `config_debugenv_test.go`,
  `acp_executor_debugenv_test.go`, and `launchwrapper/environment_test.go`
  cover the environment isolation.
- `apps/backend/internal/agentctl/AGENTS.md` documents the retention and
  isolation rules.
- Review reruns passed the race-enabled atomic-pruning tests, the ACP status-less
  adapter tests, the terminal environment-sanitization test, and `acpdbg`
  capability-flag test.
