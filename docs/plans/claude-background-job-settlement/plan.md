---
created: 2026-11-20
status: complete
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
system_design:
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
  - ../../specs/agents/system-design/claude-monitor-lifecycle.md
legacy_specs: []
---

# Fix plan for Claude ACP background jobs

## Overview

Keep active Claude Monitors visible in the existing Background Jobs chip above
chat. Correct the count for finished Monitors and detached Bash commands on
the managed Claude ACP v1 bridge. Qualify the managed Bash exit first. The
Monitor work reuses its test bridge identity and workload assertions.

## Incident and reproduction

On November 20, 2026, Claude launched one detached Go test command and 36
Monitors in one turn. Claude's native transcript reports 32 completed jobs
and five failed Monitors by 10:44:52 UTC. Kandev retained all 37 workload rows
as Running. The original raw ACP capture was unavailable. Native outcomes
were matched to the persisted rows by tool-call ID.

- **Bash.** The session initialized with the managed bridge `0.107.0`.
  `verifiedClaudeTerminalBridge` accepts null-code exits only for `0.97.0`.
  The smallest reproduction is a `0.107.0` initialize response, a detached
  Bash launch, and a separate matching `terminal_exit` with null fields.
- **Monitor.** The retained `foreground_idle` event at 10:51 UTC carried
  `prompt_handoff: true`. No successor opened before the original turn ended.
  `handOffTurnLocked` protected the active Monitor bindings.
  `takePromptEndMonitors` skipped those bindings. Bulk tool-message cleanup
  changed transcript status without publishing Monitor workload state.
  Reproduce both sides with two tracked Monitors after the prompt returns.
  Complete one while its sibling keeps watching. The completed record should
  retire by ID and the live sibling should remain counted. An ordinary prompt
  completion with no handoff signal also exercises the legacy sweep.

The installed `0.107.0` bridge helpers were exercised with synthetic IDs.
Launch metadata suppressed the Bash terminal exit while the task was tracked.
A later task end emitted a matching null-code exit. The Monitor async task
runtime emitted zero frames with Kandev's current client capabilities. The
official release source documents [the Bash terminal frame](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.107.0/src/v1/background-work.ts#L24-L53)
and [the async-task capability check](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.107.0/src/async-tasks.ts#L126-L130).
The inspected native session records carry `origin.kind=task-notification`,
`origin.producer=session-task`, the native session ID, and terminal status
alongside both registration IDs. This is the proposed supplemental source.

## Scope

### In scope

- Exact-version Bash exit qualification and a test against the managed pin.
- Active Monitor visibility across foreground idle, prompt completion,
  successor handoff, and reload of the same attached session.
- Exact per-Monitor settlement from provider evidence. Qualify the native
  completion source for the managed v1 release.
- Regression coverage for mixed live and finished work, desktop and phone
  History, and verified owner exit.

### Out of scope

- AIR `asyncTasks` negotiation, SDK upgrades, new provider protocols, and
  historical replay or database backfill.
- New controls, layouts, labels, and global agent tool-use policy.

For any live reproduction, reuse one watcher or execution handle for each
outstanding command. Use the original job's ID for correlation.

## Technical approach

`dialect_claude_terminal.go` qualifies the official name and exact captured
versions. Both versions flow through the existing typed terminal observation.
`monitor.go` retains the Monitor's registration and uses its original binding
for verified terminal evidence. Keep the live binding during prompt cleanup.
For managed v1, qualify a bounded read of the owning Claude session's
provider-produced completion records on the launching executor. Both repairs
use existing workload delivery, persistence, and revisions.

| Connection | Evidence | Workload result |
| --- | --- | --- |
| Claude ACP v1 `0.97.0`, `0.107.0`, or qualified managed `0.109.0`, Bash | Matching separate null-code terminal exit | Ended with unavailable outcome. A reported numeric outcome follows the existing path. |
| Older or unqualified Bash bridge | Existing qualified evidence or verified process exit | Existing fallback. |
| Claude ACP v1, Monitor | Explicit tool-scoped terminal evidence for a tracked Monitor | Only that Monitor records the provider outcome, including after handoff. |
| Managed Claude ACP v1 `0.107.0`, Monitor | Qualified Claude Code `2.1.323` native outcome for both registration IDs | Exact completed, failed, or interrupted Monitor moves to History. |
| Managed Claude ACP v1 `0.107.0`, Monitor | Foreground idle, prompt response, or a successor turn | A watch still tracked by the live owner stays in the running chip. |
| Unavailable or unqualified Monitor evidence | Source unavailable, ambiguous, or inconclusive | Preserve tracked state. Existing recovery and verified process exit govern ownership loss. |

The [shell exit design](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md)
and [Monitor design](../../specs/agents/system-design/claude-monitor-lifecycle.md)
define the evidence and ownership rules. The [accepted evidence decision](../../decisions/2026-11-20-claude-monitor-completion-evidence.md)
records the executor-local source boundary. The completed
[0.97.0 repair](../claude-background-terminal-exit/plan.md) supplies the
starting integration and browser fixtures.

## Tests

| Criteria | Regression evidence |
| --- | --- |
| 002.2, 010.3, 010.6 | `TestClaudeBackgroundTerminalExitManagedDefault` in `claude_background_terminal_test.go` fails on the exact-version guard. Extend the existing projection and accounting tests for matching identity, persistence, late delivery, and a live sibling. |
| 002.1–002.3, 010.1, 010.4, 010.7–010.8 | `TestClaudeMonitorSurvivesPromptCompletion`, handoff preservation, source parser and identity tests, plus `TestClaudeMonitorWorkloadProjection` cover live bindings, exact ownership, projection, provider outcome retention, and terminal monotonicity. `TestVerifiedBackgroundWorkStopStillRequiresSuccessfulLivenessProbe` and `TestOwnedTeardownSettlesOpenBackgroundWork` cover verified owner settlement. |
| 005.1, 005.6, 005.8, 005.9, 010.5, 010.8 | Both browser projects assert the persisted count, the Monitor rows in the chip summary, and History before and after reload. Verify the draft survives workload updates. |

## E2E tests

On a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` once
before the first browser command.

Reuse `claude-background-terminal-exit.spec.ts` in `chromium` and its
`mobile-claude-background-terminal-exit.spec.ts` counterpart in
`mobile-chrome` with a test-owned managed bridge identity. Two Bash launches
must count down from two jobs to one to zero as their distinct exits arrive.

Add `claude-monitor-lifecycle.spec.ts` and
`mobile-claude-monitor-lifecycle.spec.ts` under `apps/web/e2e/tests/task/`.
Keep two Monitors visible above chat after prompt completion and reload.
Open the running summary and verify both Monitor labels. Finish one during a
successor prompt and observe the count drop to one. Finish the second and
verify both in History. Desktop uses the existing popover and Background Work
panel. Phone uses the running summary drawer and the full-height view.

## Work orders

- [x] [Task 01. Qualify managed Bash exits](task-01-qualify-managed-shell-exits.md)
- [x] [Task 02. Keep live Monitors visible and settle finished watches](task-02-reconcile-monitor-lifecycle.md)

## Verification results

- Read-only incident correlation matched all 37 job outcomes by tool-call ID.
- Installed bridge helper probes passed for Bash launch suppression and
  null exit. The disabled Monitor async-task runtime emitted zero updates.
- Read-only inspection of the incident native session verified the Monitor
  task notification schema and exact ID correlation. No live provider run was
  performed during implementation.
- Existing targeted ACP adapter tests passed for shell exits, Monitor
  attestation, prompt-end sweep, and handoff preservation.
- The docs catalog, specification lint and its 36 tests, and local PR
  documentation coverage passed. Coverage validated both work orders
  against representative production paths.

Task 01 implementation checks passed: the managed-default red test failed on
the unqualified version, then adapter, orchestrator, and mock-agent tests
passed. The desktop and mobile terminal-exit Playwright scenarios passed.
Task 02 implementation passed its focused ACP, process, orchestrator,
task-service, and mock-agent checks. The changed-Go linter reported zero
issues. `make -C apps/backend build` passed, as did the Chromium and
mobile-chrome Monitor lifecycle scenarios. The UI scenarios kept two live
Monitors through reload and a successor turn, retired one at a time from
matching native outcomes, retained the draft, and showed both rows in History
before and after reload. Transcript cards join terminal observations by session
and source-call ID. Desktop and phone scenarios confirmed the completed card
stops showing Watching while the live sibling remains Watching, before and
after reload. A focused component test covers this projection, and production
event dispatch coverage confirms the foreground stays idle.

Deterministic tests used the sanitized native record fixture and covered
append parsing, exact IDs, duplicate and partial records, and file replacement.
The source fields were qualified against the incident's read-only native
session transcript; they were not rechecked with a live provider.

## Risks

- A null exit from an older bridge can belong to the launch result. Keep the
  compatibility gate tied to captured release behavior.
- Monitor registration and native exit can arrive out of order. Test both
  orderings and fence reads to the owning runtime and original IDs.
- Native record formats and paths can change. Scope the reader to the
  qualified versions and exact file. Unsupported sources retain the current
  state until qualified termination or verified owner exit.
