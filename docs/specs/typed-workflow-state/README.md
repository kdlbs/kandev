---
status: draft
system: typed-workflow-state
specification_version: 1
migration: complete
owners:
  - kandev
---

# Typed workflow review state

## Purpose

Two backend enablers that let a board review loop keep its round count and its
findings ledger in typed storage instead of task-plan prose.

- **Part 1** — a server-computed step-entry number injected into workflow prompt
  templates, replacing an agent counting headings in its own plan.
- **Part 2** — MCP read and resolve tools over `task_review_findings`, which is
  write-only today.

Neither is independently motivated; the workflow rewrite that consumes them needs
both. **This system changes no workflow prompt and no workflow YAML.**

## Documents

| Document | Owns |
|---|---|
| [requirements/step-entry-number.md](requirements/step-entry-number.md) | REQ-TWS-001, REQ-TWS-002 |
| [requirements/review-findings-access.md](requirements/review-findings-access.md) | REQ-TWS-003, REQ-TWS-004 |
| [requirements/concurrency-and-idempotency.md](requirements/concurrency-and-idempotency.md) | REQ-TWS-005 |
| [system-design/typed-workflow-state.md](system-design/typed-workflow-state.md) | Prior art, input inventory, E2E decision |

The non-functional constraints and the named exclusions below are **system-wide**
and are stated here once rather than repeated per document.

## Terminology

- **Step entry** — one committed change of `tasks.workflow_step_id` whose new value
  is the step in question; exactly one `task_step_transitions` row.
- **Entry number** — the 1-based ordinal of the current step entry.
- **Recorded entry count** — `COUNT(*)` of ledger rows for `(task, step)`; equals the
  entry number for a task whose history postdates the ledger.


## Non-functional

- **NFR-1:** The count query adds at most one indexed read **per template that
  contains the token** (AC-TWS-001.6). Because `workflowInstructionsBlock` is
  called from inside `buildWorkflowPromptWithContext`, one prompt build renders two
  templates, so a build in which both carry the token performs two reads. That is
  the bound. No cross-call-site cache is required and none shall be added: the read
  is a per-task indexed count, and memoising it would buy one saved query at the
  cost of an invalidation rule nothing else here needs.
- **NFR-2:** No schema change. `task_step_transitions` and
  `task_review_findings` are used as they stand.
- **NFR-3:** No behaviour change for any template that does not contain
  `{step_entry_number}` — which today is every template in the live database.
- **NFR-4:** The new tools are registered in the existing `review` group, so the
  surface gating is inherited rather than restated.
- **NFR-5:** When `ReviewService` is not wired, the new tools shall behave exactly
  as `publish_review_findings_kandev` does today: their backend actions are not
  registered and the call fails as an unknown action. No bespoke
  feature-unavailable path shall be added.


## Out of scope

Named exclusions. Each is a contract, not an oversight.

1. **Every workflow prompt and workflow YAML.** No template is edited to use
   `{step_entry_number}` and no cap wording changes; that rewrite is a separate card
   starting only after this one merges. On merge this card changes no observable
   board behaviour except the one named in exclusion 10. Intended.
2. **The plan write API** — the replace-only verb, the missing base fingerprint, the
   revision model, the write amplification. This card removes two consumers from the
   prose substrate; it does not fix it.
3. **Backfilling pre-ledger step entries.** The 503 tasks with no ledger rows are not
   reconstructed; AC-TWS-001.4 and AC-TWS-002.6 define how they behave.
4. **Terminating a run when a cap is exceeded.** This card makes the number true;
   acting on it stays the prompt's policy.
5. **A new index on `to_workflow_step_id`.** Per-task counts are small (busiest live
   task: 31 rows) and the `task_id` index prefix suffices.
6. **Reading review *runs* or a run's `summary` through MCP.** The new tool returns
   findings; `GetTaskReview` continues to serve the UI with both.
7. **Bulk or filter-based resolution** (AC-TWS-004.7), and any change to
   `PublishFindings`' supersede-by-anchor behaviour.
8. **Adding authorisation to the existing `task.review.get` and
   `task.review.finding.update` actions.** The new MCP paths authorise (AC-TWS-003.3,
   AC-TWS-004.3); whether those older UI actions should is a separate question about a
   different trust boundary, and widening them here risks the Review panel for no
   gain. AC-TWS-004.10 changes that action's **stamping**, a different concern from
   its authorisation.
9. **Frontend changes of any kind.**
10. **Preserving the UI's current repeated-resolve re-stamping.** AC-TWS-004.10 ends
    it on both surfaces rather than forking the service method. Nothing in the tree
    was found to depend on `resolved_at` moving on a repeated identical resolve.
11. **Distinguishing a reach denial from the task authorizer's own failure.** The
    authorizer is a `func(ctx, taskID) error` seam returning one undifferentiated
    error for both, so AC-TWS-004.12's persistence rule stops at the repository and
    any authorizer error stays a denial. Separating them means changing that
    signature and every caller of `SetTaskAuthorizer`, which is a wider change than
    this card carries.

