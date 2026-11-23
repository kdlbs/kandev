---
created: 2026-11-23
status: implemented
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-010
system_design:
  - ../../specs/agents/system-design/claude-background-shell-output.md
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
legacy_specs: []
---

# Implementation Plan: Claude background shell output

## Overview

Show the output of Claude detached shells in the background jobs viewer
(GitHub issue #5439). agentctl tails the provider output file named by the
verified launch result and emits the existing `background_work_output` chunks.
One work order covers the adapter, the mock-agent fixture, and E2E evidence. Decision:
[ADR-2026-11-23](../../decisions/2026-11-23-claude-background-shell-output-file.md).

## Confirmed defect and evidence

- `stampBackgroundShellWork` (`acp/normalize.go`) gives every ACP detached shell
  `InspectionOnlyWorkloadCapabilities()` with `Output: "none"`. No production
  code emits `EventTypeBackgroundWorkOutput`: the orchestrator handler,
  `AppendBackgroundWorkloadOutput`, and the web viewer are wired, but have no
  producer. The viewer therefore renders `task:backgroundOutputUnavailable`.
- The local Kandev database holds real Claude launch results in
  `normalized.shell_exec.output.stdout`, for example `Command running in
  background with ID: bi069qsmx. Output is being written to:
  /tmp/claude-0/<project>/<session>/tasks/bi069qsmx.output`.
- The named files exist as regular files under the native session directory
  and grow while the command runs.
- Installed bridge 0.109.0 streams these files only to ACP v2
  (`background-output.js`) and AIR `asyncTasks` clients. Kandev uses ACP v1 and
  advertises neither. Advertising `asyncTasks` stays rejected
  (ADR-2026-11-01).

Root cause: Kandev never reads the provider's output file, and the ACP shell
capability declares output unavailable.

## Scope

### In scope

- Path extraction and validation for verified Claude detached shells.
- One poller per adapter (2 s, stat-first) following at most 20 jobs, with a
  final read on terminal exit and a 30-minute no-growth cutoff.
- `Output: "stream"` capability only when a valid path exists.
- Mock-agent fixture output file and one desktop E2E.

### Out of scope

- AIR `asyncTasks`, ACP v2 migration, SDK changes, and background controls.
- Tracking timeout-moved foreground commands as workloads.
- Subagent and Monitor output files, web or locale changes, feature flags.
- Resuming a tail after agentctl restart, historical backfill.

## Work orders

| # | Work order | Depends on |
| --- | --- | --- |
| 01 | [Tail Claude background shell output](task-01-tail-claude-shell-output.md) | none |

## Regression test

`TestClaudeBackgroundShellOutputStreamsFromLaunchFile` in
`acp/claude_shell_output_test.go` drives a verified 0.109.0 launch frame whose
result names a temp `tasks/<id>.output` file under the adapter's native session
directory. It fails today: the launch observation advertises `Output: "none"`
and no `background_work_output` event is sent.

## Risks

- **Wording dependency.** If Claude changes the sentence, output reverts to the
  explicit unavailable state. Settlement does not depend on it.
- **Out-of-workspace read.** Limited to the validated task-file shape in the
  current native session, regular files only.
- **Polling cost.** Bounded to one goroutine and at most 20 `stat` calls every
  2 s per adapter; files are read only when they grow.
- **Missed exit.** A job whose terminal exit never arrives stops being followed
  after 30 minutes without new output; its retained output stays.
- **Quiet long jobs.** A job silent for over 30 minutes stops streaming;
  later output is not shown. This trades completeness for a hard bound.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp/... -run 'ClaudeBackgroundShellOutput|ClaudeShellOutputPath|ClaudeShellOutputFollower|ClaudeBackgroundTerminal' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp/... ./cmd/mock-agent/... -count=1)
bash scripts/lint-go-changed apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go apps/backend/internal/task/service/background_work_test.go
(cd apps/backend && go test -trimpath ./internal/task/service -run '^TestBackgroundWorkOutputMergeUsesJSONDeliveredUTF8Offsets$' -count=1)
make -C apps/backend build
(cd apps/web && pnpm e2e:run tests/task/claude-background-shell-output.spec.ts)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
git diff --check
```
