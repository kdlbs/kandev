---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001
created: 2026-09-05
owners:
  - Kandev
---
# GitHub PR Auto-Fix Conflicts System Design

## Purpose and boundaries

The integration system owns GitHub mergeability state and auto-fix signal
classification. This design adds ordinary merge conflicts to the existing
per-PR auto-fix evaluator.

The design reuses the current poller, prompt renderer, session dispatcher,
round limit, and automation controls. Merge-queue removal behavior remains in
[GitHub PR Merge Queue Recovery](github-pr-merge-queue-recovery.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001` | [Conflict classification](#conflict-classification), [Checkpoint and prompt](#checkpoint-and-prompt), [Control help](#control-help) |

## Components and responsibilities

`TaskPR.MergeableState` carries the normalized GitHub mergeability state from
the latest lightweight pull-request sync.

`handleTaskPRCIAutoFix` in
`apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
combines full feedback with pull-request state. It then builds the actionable
delta and current checkpoint.

The existing auto-fix dispatcher owns session selection, direct delivery,
durable queueing, coalescing, and round accounting. This change does not add a
second dispatch path.

## Conflict classification

The evaluator classifies only normalized `dirty` mergeability as an ordinary
merge conflict. It does not classify `blocked`, `behind`, `draft`, `unknown`,
or an empty state as a conflict.

The existing settled-check gate applies before conflict evaluation. This gate
keeps one auto-fix round focused on the final check, review, and conflict
snapshot.

## Checkpoint and prompt

`ciAutomationCheckpoint` adds an optional conflict item. The item contains:

- normalized mergeability state.
- head commit.
- head branch.
- base branch.

The JSON field is additive. Existing checkpoint rows decode with no conflict
item, so the change needs no database migration.

The current checkpoint contains one conflict item while the pull request is
`dirty`. The delta contains that item when no identical item exists in the
previous checkpoint.

An authoritative non-conflict observation removes the item during the existing
prompt-free checkpoint refresh. An empty or `unknown` state preserves the
prior item because GitHub can report these states during recomputation. A later
`dirty` observation is actionable after an authoritative clear. A changed head
commit or base branch also creates a new snapshot while the state remains
`dirty`.

When `{{pr.feedback}}` is present, `ciAutomationRenderSnapshot` adds a merge
conflict section. It names the sanitized head and base branches and the head
commit. The saved prompt remains hidden system context.

## Queue-removal compatibility

The same evaluator can include an ordinary conflict item and a queue-removal
item in one delta. The dispatcher sends one combined round for that evaluation.

For failed-check queue removals, current-head ownership requires both a
matching durable `last_queue_attempt_head_sha` and a non-empty
`last_merge_signature` written by an attempted or adopted queue entry. A
passive removal-only baseline does not prove which pull-request head produced
the removal, so it cannot start an auto-fix round. The removal event ID remains
the deduplication identity, and same-head automatic requeue protection remains
unchanged.

## Control help

`PRCIAutomationControls` keeps the existing switch label and responsive
surfaces. English and translated help state that auto-fix handles these inputs:

- settled failed checks.
- review comments.
- ordinary merge conflicts.
- actionable merge-queue removals.

The existing desktop popover and mobile drawer use the same localized copy.
No layout, navigation, touch target, or scroll behavior changes.

## Failure and recovery

- If mergeability is unknown, Kandev preserves prior conflict checkpoint state
  and waits for a later authoritative state.
- If checks are unsettled, Kandev waits and does not use a round.
- If prompt delivery fails, the existing per-PR error and retry behavior apply.
- If a conflict clears, the checkpoint refresh is prompt-free.
- If Kandev restarts, the persisted JSON checkpoint preserves deduplication.

## Security

Branch names and commit identities are untrusted provider data. Snapshot
rendering uses the existing field sanitizer and `sysprompt.Wrap` boundary.

The change does not expand GitHub write permissions. The agent uses its current
task and repository permissions to repair the branch.

## Observability

Existing CI automation logs identify the task, repository, pull request, and
dispatch result. Focused tests cover conflict classification, checkpoint
transitions, queue-removal eligibility, and prompt rendering.

## Related decisions and designs

- [PR agent notifications](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md)
- [Bind automatic merge attempts to the reviewed head](../../../decisions/2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head.md)
- [Task PR automation design](../../ui/system-design/ci-pr-automation-02.md)
- [GitHub PR Merge Queue Recovery](github-pr-merge-queue-recovery.md)
