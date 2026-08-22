# GitLab MR Lifecycle Notifications

- **Status:** accepted
- **Area:** backend, frontend
- **Date:** 2026-08-01

## Context

ADR 0051 added task PR lifecycle notifications for GitHub (`github_task_ci_options`, a
one-minute PR poller, and a durable lifecycle message queue) but explicitly deferred
GitLab parity as an independent follow-up. Without it, a GitLab review task goes idle
once its agent finishes: nothing wakes the task when review is re-requested, or when
the linked MR merges or closes.

Two structural facts shaped the design:

1. **`gitlab_mr_watches` is dead in production.** `Service.CreateMRWatch` has no
   production caller; only `gitlab_task_mrs` (written by the link dialog and the
   agent `pr` skill) is a durable, task-scoped record of a linked MR.
2. **At the time of this ADR, GitLab had no auto-fix/auto-merge automation and
   no per-PR review-request API event.** `Client.GetMR` already returns
   `Reviewers`, so "review requested" is modeled as assignment-as-request (the
   authenticated user appearing in `MR.Reviewers`), not a separate signal
   fetch the way GitHub's requested-reviewers endpoint works. Auto-fix and
   auto-merge automation for GitLab MRs landed as a follow-up on the same
   `gitlab_task_mr_options` / `gitlab_task_mr_state` tables and
   `/tasks/:taskID/mr-automation` endpoint this ADR introduced — see
   `docs/specs/gitlab-integration/spec.md`'s "MR automation" section. This
   item's absence-of-automation framing describes the state at the time of
   writing, not the current feature set.

## Decision

Mirror the GitHub surface (`gitlab_task_mr_options` / `gitlab_task_mr_state` tables,
`get/update_task_mr_automation_kandev` MCP tools, GitLab-owned HTTP endpoints, and a
`MRAutomationControls` frontend component) but drive lifecycle evaluation off the
**linked-MR poll**, not a watch:

- The existing one-minute `mrMonitorLoop` gains a `runMRLifecycleSync` pass over
  `ListLifecycleSubscribedTaskMRs` (every `gitlab_task_mrs` row whose task has at
  least one switch enabled). Each row is re-synced via the existing `SyncTaskMR` and
  publishes `gitlab.task_mr.updated`.
- The orchestrator subscribes to that event and runs a pure decision function
  (`decideTaskMRAgentPrompt`) against the MR's normalized state, the task's
  switches, and a per-MR checkpoint (`gitlab_task_mr_state`), single-flighted per
  `(task, repository, iid)`.
- Delivery reuses ADR 0051's durable lifecycle message queue
  (`QueueLifecycleMessageWithCoalesceKey`) verbatim — no changes to
  `messagequeue/`.

This is a second, structurally different poll driver from GitHub's (link-driven vs.
watch-driven). The alternative — reviving `gitlab_mr_watches` and mirroring GitHub's
watch-driven poller exactly — was rejected because it requires first shipping
MR-watch creation as a production feature (currently non-existent), and because watch
rows are session-scoped, the wrong lifetime for a task-level subscription.

A provider-agnostic lifecycle core shared by GitHub and GitLab was also considered
and rejected: `internal/gitlab` is documented as mirroring `internal/github`'s
surface, not sharing it, and the GitHub lifecycle code had just landed with four
rounds of hardening — rewiring it for a second consumer during that window would
have been high-risk for low reward. The narrow exception is
`queueAndDrainLifecyclePrompt` (queue + drain mechanics only, no decision logic),
extracted to satisfy the repo's duplicate-code lint after the GitHub and GitLab
dispatch functions turned out byte-for-byte identical below the decision layer.

## Consequences

- **Positive:** no new poller goroutine (the existing one-minute tick already runs);
  manual link/sync also triggers evaluation, matching GitHub's "any status refresh
  evaluates" property; zero changes to GitHub's lifecycle files beyond the mechanical
  extraction above.
- **Negative:** a future reader auditing the two providers' poll drivers side by side
  will see GitHub keyed off watches and GitLab keyed off links — this ADR is that
  explanation.
- **`locked` state.** GitLab's MR state machine has a fourth value with no GitHub
  analogue. It is treated as non-terminal (fires neither `merged` nor `closed`) and
  is exercised as its own regression case in the decision-function tests.
- **Amended: the five switches are now per linked MR, not per task.** As written,
  this ADR put `prompt_on_review_requested` / `prompt_on_merged` / `prompt_on_closed`
  (and later auto-fix / auto-merge) on `gitlab_task_mr_options`, keyed by `task_id`
  alone. That is wrong for a task with more than one linked MR: enabling a switch on
  one MR silently enabled it on all of them. The five switches moved to
  `gitlab_task_mr_automation_options`, keyed by the same
  `(task_id, repository_id, project_path, mr_iid)` identity as
  `gitlab_task_mr_state`, seeded once from the legacy task-wide values by an
  `mr_scope_migrated_at`-guarded fan-out. `gitlab_task_mr_options` keeps only what is
  genuinely task-level — the auto-fix prompt override and the server-resolved
  reviewer username — so a reviewer-identity change still clears every linked MR's
  review-request baseline at once, while a switch flip clears only its own MR's
  checkpoints. A `PATCH` / `update_task_mr_automation_kandev` call naming an MR
  targets it alone; omitting MR identity fans out to every linked MR, which preserves
  the behavior of agents that have no MR identity to send. See
  `docs/specs/gitlab-integration/spec.md`'s "MR automation" section.
- **Discovered while integrating:** `executeQueuedMessage`'s lifecycle-prompt
  detection (`event_handlers_agent.go`) was hardcoded to the GitHub PR automation
  origin string. A GitLab-originated durable lifecycle entry was queued but never
  reached `AcknowledgeQueued`, leaving the delivering goroutine to hang indefinitely.
  Generalized via `isLifecycleAutomationOrigin` so both providers share the same
  durable delivery contract — this is a bug fix to already-shared plumbing, not new
  provider-specific logic in it.

## Alternatives Considered

See "Decision" above — reviving `gitlab_mr_watches`, and a shared provider-agnostic
lifecycle core — both rejected for the reasons given.
