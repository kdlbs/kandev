---
status: draft
system: office
created: 2026-08-09
updated: 2026-08-09
owners:
  - nova28
---
# Automations — "Pull request merged" trigger Requirements

## Overview

A task whose pull request has merged is finished, but it stays on the board until somebody notices and archives it by hand. Kandev already knows the PR merged — it polls every linked PR once a minute — but the Automations engine has no condition that reacts to it, so the one piece of tidying every user does after every merge is the one thing they cannot automate.

## Requirements

### REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001: Automations — "Pull request merged" trigger

**Intent:** A task whose pull request has merged is finished, but it stays on the board until somebody notices and archives it by hand. Kandev already knows the PR merged — it polls every linked PR once a minute — but the Automations engine has no condition that reacts to it, so the one piece of tidying every user does after every merge is the one thing they cannot automate.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.1:** The Automations engine gains a **sixth** trigger type, **`github_pr_merged`**, labelled **"Pull request merged"** in the condition picker, in the same `github` category as the existing GitHub conditions. `scheduled`, `github_pr`, `github_push`, `github_ci` and `webhook` already exist, so afterwards there are **6** `TriggerType` constants and **6** registry entries. The condition picker filters out the `schedule` category, so it shows **5** entries after this change rather than 6.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.2:** **Count and position are different facts, and both are pinned.** The new entry is the 6th by *count* but is inserted at **array index 2** in `triggerTypeRegistry`, between `github_pr` and `github_push` — see [Editor surface](#editor-surface) for why position matters. Appending it instead still yields 6 entries and still compiles; it simply renders in the wrong place, which is why an adjacency scenario exists rather than only a count.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.3:** **The registry test that pins these counts does not exist yet and is part of this change.** No test in `apps/backend` currently reads `GetTriggerTypes()` or `triggerTypeRegistry` for length or membership, so today a row can be added or lost silently. This spec requires adding one; it is specified under [API surface](#trigger-type-registry) and observed by a scenario. Without it the counts above are prose nobody checks.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.4:** The trigger fires when Kandev observes that a pull request **linked to a task in the automation's workspace** has reached the `merged` state. Detection rides the PR poller that already runs (`github.task_pr.updated`), so the expected latency between the merge on GitHub and the firing is **under ~60 seconds**, not instant.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.5:** The trigger's configuration is a repository filter and an optional base-branch filter. Nothing else.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.6:** The firing carries the **id of the task the merged PR was linked to** into the trigger data, reachable in the prompt and title templates as `{{data.task_id}}`.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.7:** The automation's default prompt instructs the spawned agent to archive exactly that task through the `archive_task_kandev` MCP tool. Archiving stays an agent action and this feature adds no native action type, but the target is enforced structurally: the run task persists the validated event target and the backend rejects an archive request for any other task.
- **AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.8:** Because the archive is performed by an LLM reading a prompt, the trigger data is deliberately **narrow**: it carries identifiers and single-token git/GitHub values only, and never PR title, PR body, or author login. See [Prompt-injection surface](#prompt-injection-surface).

## System design

The migrated technical source is split into [part 1](../system-design/automations-pr-merged-trigger-01.md), [part 2](../system-design/automations-pr-merged-trigger-02.md), [part 3](../system-design/automations-pr-merged-trigger-03.md), [part 4](../system-design/automations-pr-merged-trigger-04.md), [part 5](../system-design/automations-pr-merged-trigger-05.md), [part 6](../system-design/automations-pr-merged-trigger-06.md), [part 7](../system-design/automations-pr-merged-trigger-07.md), [part 8](../system-design/automations-pr-merged-trigger-08.md).
