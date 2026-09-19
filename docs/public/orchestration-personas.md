---
title: "Workspace orchestration"
description: "Configure workspace coordinators using your existing agent profiles, roles and Kanban workflows."
---

# Workspace orchestration

Orchestrators are conversational coordinators assigned to an existing workspace. Create as many as you need; each has its own execution profile, workspace context and persistent conversation, with identity and instructions supplied by its global role. Tasks continue to use their own assigned agents and normal Kanban workflows.

## Enable and configure

1. Complete Kanban onboarding and configure your agent profiles, provider logins, executors and workflows.
2. Enable **Workspace orchestration** in **Settings > System > Feature toggles**, then restart Kandev. The experimental flag is `KANDEV_FEATURES_ORCHESTRATION`; it defaults off and is independent of Office.
3. Configure the role's name, icon and instructions in **Settings > Orchestration**. Then open **Settings > Workspaces > your workspace > Orchestration**, or follow the Orchestration link on the workspace overview card or sidebar.
4. Select **Add orchestrator**. Choose the global role, an existing agent profile and executor, and workspace context/delegation guidance. No separate worker personas or model-routing setup is required.
5. Open **Coordinator** from the workspace sidebar or overview card. Select an assignment to see its existing conversation beside the workspace tasks. On mobile, use **Tasks** and **Chat** to switch panes. Configuration and conversation pages link back to the workspace; coordinated tasks link back to their coordinator.

**Settings > Orchestration** manages global roles. Administrators configure each role's name, icon and instructions once. Workspace assignments inherit that configuration; saved role edits apply to all assignments on their next turn. A running turn retains the instructions it started with. Roles in use cannot be deleted. The built-in Chief of staff role is editable but cannot be deleted.

Choose **Persona icon** in the global role settings to use name initials or a built-in emoji icon. Initials use the first and last name words, so Chief of staff displays CS. The same identity appears in chat, workspace lists and sidebar navigation.

Edit orchestrators with the shared **Save changes** and **Reset** controls. Pause blocks new turns; it does not interrupt a running turn. Delete removes the coordinator and terminates its associated runtime sessions, while delivery tasks remain on their board. Opening a conversation does not create another workspace or workflow.

## Work from the Coordinator view

Open `/workspaces/<workspace-id>/coordinator`. The task view includes ordinary,
non-archived board tasks across the workspace's visible workflows. Office tasks,
hidden system workflows and private coordinator conversations stay out of this
list. Opening or refreshing the view starts no agent work.

Select a coordinator to use its configured profile and persistent conversation.
A workspace with one assignment selects it automatically; with several, choose
one. The page remembers a valid choice during your browser session. A missing
assignment or profile offers configuration instead of selecting another account.
Drafts remain separate for each conversation while switching coordinators or
mobile tabs. Drafts are held in browser memory and do not survive a full reload.

Tasks are grouped by current native status: input needed, problems, running,
review, done, queued and other. Follow a task to answer its native question or
permission request, inspect execution, or review its changes. Review and an open
or merged pull request do not imply that a task is completed. Unknown or idle
activity is not classified as stalled.

Use search, workflow and repository filters, or choose **Selected coordinator’s
tasks**. Status groups and totals describe the loaded tasks; load additional
pages for more coverage. The initial page contains up to 100 tasks. Pull request
and changed-file totals show how many loaded tasks have known data. A stale
notice means refresh failed; retry before relying on the displayed state.

On desktop, hide or show chat without changing the task selection. On a touch
device, Tasks and Chat share the same workspace and conversation. Expand
**Filters** for workflow, repository and coordinator scope controls. Live task
summaries update the groups, and reconnecting refreshes the loaded rows.

## Use the personal Assistant

With Orchestration and Personal assistant enabled, open **Assistant** in app
navigation, or visit `/assistant`. Choose an existing workspace orchestrator.
The page resumes its conversation and claims it for your user; it creates no
additional workflow or delivery task. Office is not required. Configuration
remains in workspace Orchestration settings. Unsupported execution profiles
show a configuration link and cannot send assistant turns.

Desktop keeps chat beside Attention and Details. Phones use Chat, Attention and
Details tabs. Drafts survive tab changes and temporary connection failures
within the page, but not a reload or change of assistant identity. Message
retries preserve their client identity to avoid duplicate accepted turns.

Attention shows native worker questions, permissions, failures and review or
result observations. **Review and respond** opens the original question form
or the provider's offered permission choices. Decisions use the same native
task services; stale requests cannot approve a newer request. **Open original
task** returns to the exact task/session. Assistant-authored answers are
attributed to the assistant in native task history.

Details includes objectives and acceptance evidence, delivery activity,
capabilities, credential-health metadata and memory. Accepted messages,
finished assistant turns and review status do not prove objective completion.
Lists show loaded coverage and offer additional pages when available.

Add or edit a memory with an instruction you sent, its scope and optional
expiry. Confirmation permits its use as confirmed context. Concurrent edits
use revision checks. **Original instruction** shows provenance; **Forget**
removes the memory from future context. Neither action erases text already
sent to a provider or kept in history/backups. Keep secret values out of memory.

**Pause assistant** blocks new assistant turns while existing workers continue.
**Stop managed work** asks the native task service to stop managed sessions and
reports each result, including uncertain or partial results. Continue through
remaining pages for large sets. Stopping does not undo external changes. The
[assistant reference](personal-assistant-api.md) documents supported execution
profiles and qualification limits.

## Control workspace tasks from chat

The Orchestrator uses your configured execution profile and Kandev's injected
run credentials. It does not need a separate Kandev API key or private-assistant
setup to manage the current workspace. The managed Claude runtime preapproves
Kandev broker calls; backend task and workflow checks still apply. Worker
permissions continue to use the worker's configured policy.

Ask it to create a task, edit its title, description, priority or parent, assign
an execution profile, start or stop work, send a worker a message, move a task
between workflow steps, archive it, or delete it. Changes use the native task
services and appear on the board. For example: “Create a task called Update the
sample README, then move it to the backlog.”

Workflow moves respect configured manual-move rules, active-session restrictions
and required review decisions. Deletion uses native cleanup and does not discard
uncommitted work. Other workspaces, Office tasks and private conversations are
outside these controls. The Orchestrator inspects native task details after an
uncertain result before deciding whether an action needs retrying.

Tool discovery follows the conversation's scope. A workspace conversation gets
workspace task controls and its own memory; owner-scoped objectives, linked
workspace grants and maintenance tools belong to a selected private conversation.
They are not prerequisites for creating or managing a workspace task.

## Automatic conversation recovery

If a temporary provider failure occurs before the Orchestrator has produced a
response or called a tool, Kandev automatically restarts the execution and retries
the pending request. This includes Claude account-token refresh contention after
an idle period. The conversation shows a retry notice; use **Cancel** or **Stop**
to prevent the pending retry.

Recovery tries up to five times, waiting 15 seconds, 30 seconds, then 60 seconds
between the remaining attempts. Each attempt uses the same configured account
and freshly scoped Kandev access. Later requests wait behind the recovering turn.
You do not need to supply an API key for the Orchestrator's Kandev tools.

If recovery is exhausted, the normal failure and manual recovery controls appear.
Authentication that requires sign-in, turns with prior output or tool activity,
and interrupted turns whose effects are unknown are not automatically replayed.
Already scheduled safe retries survive a backend restart.

## Profiles and routing

The coordinator's execution profile determines its provider and account. For example, a personal Claude coordinator can direct Jira work to your existing work Claude profile and personal development to a personal Claude or Codex profile. Describe these choices in its routing context, including when to use each profile and where account-specific setup instructions live.

Profiles supply their configured environment. For separate Claude subscriptions, configure the appropriate `CLAUDE_CONFIG_DIR` in each profile and authenticate it on the chosen execution host. A profile name alone does not establish which subscription is authenticated. Keep credentials in the existing secrets/account setup, rather than in role instructions or routing context.

A coordinator does not automatically fall back to another account. Tasks retain their assigned profiles when adopted. Explicit reassignment is supported, including changes between Claude and Codex; task profiles are not pinned to the coordinator. Executors and remote-host access use Kandev's existing execution configuration.

## The task loop

The coordinator uses `kandev workspace` to discover workflows, repositories and execution profiles. It creates normal board tasks with `kandev task create --workflow <id> --assignee <execution-profile-id>` and a stable `--external-id` for retry-safe creation. It can adopt an existing task with `kandev task manage --id <id> --action adopt`, then inspect, assign, start, stop or message its existing session.

`kandev task inspect --id <id>` returns task/session status and bounded recent worker output. The full transcript remains on the task. Review, completion, failure and waiting-state events notify the owning coordinator; other coordinators in the workspace do not receive that task's notification. Normal workflow transitions, dependencies and human permission/question gates still apply.

Task updates notify the owning coordinator through a dedicated queued callback. Updates for separate tasks remain separate, and duplicate events for one committed transition are deduplicated. The coordinator inspects the current result and posts its reply in the existing conversation. Review means ready for review, not fully completed. Paused or disabled coordinators do not accept new callbacks; resuming does not replay updates rejected while paused.

For follow-ups on the same objective, the coordinator is instructed to reuse the existing task and session. Its configured role instructions are included in each turn. External issue references should use explicit provider URLs; plain Jira keys in orchestration chat are not automatically linked to Office tasks. If message forwarding times out, inspect the session before retrying: delivery may already have occurred.

The configuration page lists the latest 100 coordinated tasks. Follow a task to manage its profile and workflow, then use **Coordinating orchestrator** to return. Conversations show chat and execution feedback without task properties such as labels, priority, blockers or reviewers.

Creating an orchestrator does not schedule recurring work. Use workspace Automations to explicitly schedule a prompt for an existing orchestrator. External service repair still requires the configured executor, access and permissions; an agent inside Kandev cannot restart a stopped Kandev process.

## Context and compatibility

Native conversation turns use fresh provider context with bounded recent conversation excerpts: up to four comments at 1,000 bytes each and up to 6,000 bytes from the triggering comment. Routing includes at most 12 profile names/IDs and 2,000 bytes of guidance. Memory selection now prioritizes confirmed, applicable preferences within a byte budget rather than selecting eight recent entries. Instructions and anything the agent retrieves also consume context; these bounds are not a total token budget.

The [personal assistant backend reference](personal-assistant-api.md) documents the in-development objective, intake and shared-context additions, including retained private ownership across default changes and restrictions on private automation targets. The personal assistant has a restricted broker and an app-level interface for testing; combined qualification remains in development. Ordinary workspace coordinators retain their existing tool access.

Orchestration owns its runtime, conversation API, instructions, memory and conversation registry. It uses core execution profiles, task sessions, authentication, run queue and chat rendering. Office is independently feature flagged and is not required to configure or run an orchestrator. Office APIs cannot operate on registered orchestrators or their conversations. Disabling Orchestration hides its navigation and rejects its API/run paths without deleting configuration. The scoped `POST /api/v1/orchestration/workspaces/:wsId/import/:id` endpoint can explicitly register an existing assistant using a complete orchestrator configuration while retaining its identity and conversation history.


### Recovery and upgrades

Failed conversation turns can be retried from their error entry. Retry queues a new run with fresh credentials for the configured execution profile. Each turn starts with bounded context. After a backend restart, interrupted turns are marked failed instead of automatically repeating possibly completed external actions. Inspect the latest task results before retrying.

Upgrading from the earlier shared implementation copies registered personas' instructions, memory and conversation mappings into Orchestration's own tables. Existing persona IDs, conversation URLs and core comment history are preserved; later Office changes do not overwrite that state. Back up the database before upgrading. Disabling a flag keeps the stored data.

The Orchestration runtime CLI provides workspace discovery, task creation and management, comments and memory. Office-specific projects, skills, budgets and routines are not part of this feature. Use core workflow and executor configuration for delivery work.


### Global roles and workspace assignments

Define identity and behavior in global role settings, then add that role to a workspace. The workspace form contains only role selection, execution profile, execution environment and local context/delegation guidance. It does not duplicate the name, icon or instructions. Assignments retain separate conversations, memory, task ownership and pause controls. The same role can run under a personal profile in one workspace and a work profile in another.

Upgrading preserves existing assignment identities and history. If assignments previously had different names, icons or instruction snapshots, those configurations become distinct global roles. Repeated upgrades do not overwrite subsequent global role edits. Old instruction snapshots are retained for compatibility but are no longer included in runtime prompts.


## Scheduled orchestrator prompts

In workspace **Automations**, create an automation, choose **every day** (or another existing schedule) and its time zone, then select your workspace's code reviewer under **Run with**. Enter the recurring instruction, for example:

> Check open PRs assigned to me or requesting my review. Identify what needs to move forward, delegate detailed reviews to the appropriate task agents, and report actionable findings with PR links. Reuse existing review tasks and avoid duplicate comments or reviews.

The target is a workspace assignment, so it inherits the role's current instructions, its selected account/execution environment, local context and memory. No duplicate profile or repository selection is needed. **Run now** uses the same delivery path as the schedule.

Each firing records **Delivered to orchestrator** once the prompt is queued and links to the existing conversation. This status confirms delivery, not completion of the review. Follow chat for execution progress, results and errors. Turns are processed in order. Paused, deleted, unavailable or feature-disabled targets record a delivery failure; there is no fallback to another account. A repeated scheduled firing is deduplicated. Deleting automation history does not delete the shared conversation. Kandev must be running when the schedule is due, and the configured execution account must have access to the PR provider.
