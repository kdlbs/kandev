---
name: planner-orchestration
description: Enforce Kandev's planner-and-worker execution model. Use in the user-started primary session for feature, fix, debug, review, verification, and delivery work; use in delegated sessions to distinguish worker execution from planner coordination.
---

# Planner Orchestration

The user-started primary session is the **planner**. A custom subagent launched
through the current harness's native delegation tool is a **worker**.

## Planner Contract

The planner may:

- Clarify intent and ask the user for decisions.
- Read repository context needed to create or review a plan.
- Create and update specs, plans, task files, and ADRs.
- Decompose work, assign file ownership, and order dependency waves.
- Spawn workers, monitor results, review diffs and reports, and communicate
  status to the user.

The planner must not:

- Edit application code, tests, generated files, package metadata, or CI.
- Run implementation, test, formatting, lint, build, commit, push, or PR
  commands.
- Resolve implementation conflicts or fix worker output directly.
- Continue as an implementation fallback when delegation is unavailable.

If no suitable worker can be launched, stop and tell the user which delegated
capability is unavailable. Do not silently become the worker.

## Native Delegation Only

Delegate through the active coding harness, targeting the registered project
agent for the work packet:

- Claude Code: use the native `Agent` tool and `.claude/agents/<role>.md`.
- Codex: use native `spawn_agent`, `send_message`/`followup_task`, and
  `wait_agent` with `.codex/agents/<role>.toml`.
- Cursor: use Cursor's native custom-subagent invocation and
  `.cursor/agents/<role>.md`.
- OpenCode: use the native `Task` tool and `.opencode/agents/<role>.md`.

Never use Kandev MCP `spawn_session_kandev`, `create_task_kandev`, or
`message_task_kandev` to create or control implementation workers. Those APIs
manage Kandev platform entities and may be used only when the user explicitly
asks to create or manage Kandev tasks or sessions. They are not a delegation
fallback. If native harness delegation is unavailable, stop and report it.

## Worker Contract

A worker executes exactly one bounded assignment. It follows the assigned
execution skill, edits only its owned files, runs the named verification, and
reports results to the planner. Workers do not spawn other workers or broaden
their assignment.

## Work Packet

Every delegated assignment must contain:

```text
Role: <architect | implementer | test-engineer | qa | code-review | security-auditor |
       simplify | verify | pr-poller>
Objective: <one bounded outcome>
Inputs: <task/spec/plan paths and relevant context>
Owned files: <specific paths or narrow search targets>
Forbidden files: <overlapping or out-of-scope paths>
Acceptance:
- <observable condition>
Verification:
- <exact command>
Dependencies: <completed task IDs or none>
Output:
- Follow the role's exact output contract when it defines one.
- Otherwise: summary, files changed, commands/results, blockers, risks, divergence.
Constraints:
- Follow scoped AGENTS.md and assigned skills.
- Do not spawn subagents.
```

Workers share a mutable checkout unless the runtime explicitly provides an
isolated worktree. Run them sequentially by default. Parallelize only when
their owned files do not overlap and neither worker changes shared schemas,
generated contracts, migrations, lockfiles, or package-wide configuration.

## Model Tiers

The planner inherits the strong model selected by the user. Platform mirrors
pin workers by role:

- Balanced workers: implementation, tests, QA, simplification.
- Cheap workers: polling and mechanical verification where the platform has a
  suitable lower-cost model.
- Frontier workers: architecture, security, and deep code review only.

Do not omit a worker model when omission inherits the planner's model, except
for OpenCode where this repository intentionally keeps provider choice
inherited.

## Completion

The planner accepts a worker result only after checking that its file scope,
acceptance criteria, and reported verification match the assignment. Any
follow-up fix is a new worker assignment, not planner execution.
