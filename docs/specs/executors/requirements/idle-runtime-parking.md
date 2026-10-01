---
status: active
system: executors
created: 2026-09-28
updated: 2026-10-01
owners:
  - kandev
---

# Workspace ACP idle suspension requirements

## Overview

Users can reduce resource use by suspending idle ACP agent processes in a workspace.
The workspace policy applies to all ACP agents. It is disabled by default and has a default timeout of two hours.
The executor system owns process suspension and recovery. Workspace settings expose the policy without creating a separate runtime contract.
The always-on idle reclaim of settled runtimes runs regardless of this policy. It keeps a waiting session whose runtime record cleanup would remove for lack of a resume identity.

## Terminology

- **Idle suspension:** Stop an idle ACP process while preserving its session, conversation identity, and workspace for resume.
- **Idle:** A settled Kandev session with no active turn, prompt dispatch, pending input, or known active work.
- **Task focus:** An explicit task open/selection, session selection, or return to a visible task in a focused browser window.
  Background subscriptions, list rendering, polling, and hidden tabs are not focus.

## Requirements

### REQ-EXECUTORS-IDLE-PARKING-001: Workspace ACP idle suspension

**Intent:** Release idle ACP processes and restore the same sessions automatically when users or messages need them.

#### Acceptance criteria

- **AC-EXECUTORS-IDLE-PARKING-001.1:** Each workspace shall expose an idle-suspension switch and a configurable positive timeout. Defaults shall be disabled and 120 minutes. Existing workspaces shall retain disabled behavior after migration. Changes shall apply without a backend restart and shall not affect other workspaces.
- **AC-EXECUTORS-IDLE-PARKING-001.2:** With the policy enabled, Kandev shall suspend ACP sessions in `WAITING_FOR_INPUT`, `IDLE`, or `COMPLETED` after the timeout. An active turn, dispatch, pending question, permission, or known active background work shall prevent suspension. Provider identity shall not select eligibility. No provider-specific proof of complete internal inactivity shall be required.
- **AC-EXECUTORS-IDLE-PARKING-001.3:** Suspension shall preserve task/session identity, history, workflow state, workspace files, and provider resume identity. It shall not cancel the session, complete a workflow step, or report an agent failure.
- **AC-EXECUTORS-IDLE-PARKING-001.4:** Any accepted actionable message targeting an idle-suspended session shall resume it and deliver the message once. This includes user, agent, automation, and queued messages. Passive stored notifications shall not trigger resume.
- **AC-EXECUTORS-IDLE-PARKING-001.5:** Task focus shall resume the selected idle-suspended session without a synthetic prompt. This recovery shall also apply when the general prevent-auto-start-on-open preference is enabled. Focus shall not start never-started, manually stopped, canceled, archived, or workflow-parked sessions through this exception.
- **AC-EXECUTORS-IDLE-PARKING-001.6:** A resumed session shall start a new idle interval after it settles. A focus event shall refresh the idle interval for a settled live session. Keeping a task visible shall not indefinitely reset that interval. Passive streams and health checks shall not count as activity.
- **AC-EXECUTORS-IDLE-PARKING-001.7:** Concurrent focus and messages shall produce at most one resume and exactly one delivery per accepted message. Stale suspension work and terminal events shall not stop or complete a successor execution.
- **AC-EXECUTORS-IDLE-PARKING-001.8:** Failed suspension or resume shall preserve durable ownership and conversation identity for retry. Kandev shall never replace an unrecoverable conversation silently. Missing resume identity or unsupported restore shall produce a visible diagnostic and preserve the process when suspension has not started.
- **AC-EXECUTORS-IDLE-PARKING-001.9:** Desktop and phone workspace settings shall expose the same switch, timeout, validation, and saved state. Phone users shall use the normal touch composer and task navigation for recovery.
- **AC-EXECUTORS-IDLE-PARKING-001.10:** Disabling the policy shall prevent new suspensions without starting every suspended process. Existing suspended sessions shall still resume on focus or messages. Diagnostics shall identify suspended instances and bounded skip/failure reasons without tokens or credentials.

### REQ-EXECUTORS-IDLE-PARKING-002: Resume-safe idle reclaim

**Intent:** Keep the always-on idle reclaim from removing the runtime record of a waiting session that has no resume identity.

#### Acceptance criteria

- **AC-EXECUTORS-IDLE-PARKING-002.1:** When the always-on idle reclaim finds a `WAITING_FOR_INPUT` or `IDLE` session with no live agent and no active turn, and its runtime record has no resume identity and a status other than `running`, it shall keep the session's runtime and runtime record. This includes a prepared session whose agent has not started.
- **AC-EXECUTORS-IDLE-PARKING-002.2:** After such a reclaim pass, a prompt to a `WAITING_FOR_INPUT` session with a prepared runtime record shall attempt to start its agent in the same session. It shall not report the session runtime as unavailable.
- **AC-EXECUTORS-IDLE-PARKING-002.3:** A session whose runtime record holds a resume identity or has status `running`, and a `COMPLETED` session without a resume identity, shall keep the existing reclaim behavior. The runtime is released, a record with a resume identity or status `running` is repaired in place, and a completed record without either may be removed.

## Compatibility

This user-selected retention policy uses Kandev-observed activity. It does not promise preservation of provider work that ACP does not expose.
Known activity remains protected. Absence of a provider-specific quiescence API is not a blocker or an exclusion.
All ACP provider families use the shared policy, including OpenCode. Provider resume/load capability remains the existing restoration prerequisite.
Executor adapters stop only the session's ACP process resources. They preserve task-owned containers, Pods, worktrees, and shared runtimes.
No OS-specific background-process probe or macOS-only test gates the general feature.

## Exclusions

Passthrough terminals and never-started sessions are outside ACP idle suspension. Taskless run-owned sessions retain their run lifecycle.
The active-session admission ceiling and disconnected-owner agentctl timeout remain separate policies.

## References

- [Design](../system-design/idle-runtime-parking.md)
- [Plan](../../../plans/attached-acp-idle-parking/plan.md)
- [Open-time activation](../../tasks/requirements/prevent-agent-autostart-on-open.md)
