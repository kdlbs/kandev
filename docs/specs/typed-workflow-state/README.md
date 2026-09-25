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

A backend enabler that lets a board review loop keep its round count in typed
storage instead of task-plan prose: a server-computed step-entry number injected
into workflow prompt templates, replacing an agent counting headings in its own
plan.

**This system changes no workflow prompt and no workflow YAML.** The workflow
rewrite that consumes the number is a separate card starting only after this one
merges.

This system was originally specified in two parts. Part 2 — MCP read and resolve
tools over `task_review_findings` — was withdrawn on upstream review and is
recorded under Out of scope 7; it is not deferred work owned by this system.

## Documents

| Document | Owns |
|---|---|
| [requirements/step-entry-number.md](requirements/step-entry-number.md) | REQ-TWS-001, REQ-TWS-002 |
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
  called from inside `buildWorkflowPromptWithTrustedContext` — the function
  `buildWorkflowPromptWithContext` delegates to, and the one that also interpolates
  the step prompt — one prompt build renders two
  templates, so a build in which both carry the token performs two reads. That is
  the bound. No cross-call-site cache is required and none shall be added: the read
  is a per-task indexed count, and memoising it would buy one saved query at the
  cost of an invalidation rule nothing else here needs.
- **NFR-2:** No schema change. `task_step_transitions` is used as it stands.
- **NFR-3:** No behaviour change for any template that does not contain
  `{step_entry_number}` — which today is every template in the live database.


## Out of scope

Named exclusions. Each is a contract, not an oversight.

1. **Every workflow prompt and workflow YAML.** No template is edited to use
   `{step_entry_number}` and no cap wording changes; that rewrite is a separate card
   starting only after this one merges. On merge this system therefore changes no
   observable board behaviour at all, because no live template contains the token.
   Intended.
2. **The plan write API** — the replace-only verb, the missing base fingerprint, the
   revision model, the write amplification. This system removes one consumer from the
   prose substrate; it does not fix it.
3. **Backfilling pre-ledger step entries.** The 503 tasks with no ledger rows are not
   reconstructed; AC-TWS-001.4 and AC-TWS-002.6 define how they behave.
4. **Terminating a run when a cap is exceeded.** This system makes the number true;
   acting on it stays the prompt's policy. The upstream review that withdrew Part 2
   concluded separately that the **workflow engine**, not the agent, should enforce
   the retry limit. Nothing in `internal/workflow` does so today, so with this system
   merged the agent still enforces its own cap — from a correct number rather than a
   miscounted one, which is the whole of what REQ-TWS-001 claims. Engine-side
   enforcement is tracked as card `05996338` and is not in scope here.
5. **A new index on `to_workflow_step_id`.** Per-task counts are small (busiest live
   task: 31 rows) and the `task_id` index prefix suffices.
6. **Frontend changes of any kind.**
7. **MCP read and resolve access to `task_review_findings`.** Part 2 of the original
   specification added `list_review_findings_kandev` and
   `resolve_review_finding_kandev` to the existing `review` tool group, so a review
   step could read back and close the findings it publishes. That store remains
   write-only: `registerReviewTools` registers only
   `publish_review_findings_kandev`, the step a review bounces back to still has no
   MCP path to the findings, and the findings ledger therefore stays in plan prose.
   The upstream maintainer rejected both tools on review
   (`https://github.com/kdlbs/kandev/pull/3266#issuecomment-5537758182`) in favour of
   a **generic task-artifact contract** — `post_task_artifact` / `list_task_artifacts`
   / `update_task_artifact`, with findings projected through
   `list_task_artifacts(kind="review.finding")` — rather than a review-specific pair.
   The decision is accepted, not appealed: REQ-TWS-003 and REQ-TWS-004 are
   **withdrawn** from this system rather than parked, and the successor owns its
   contract from scratch. Three constraints agreed in that review carry forward to it:
   build on the existing `task_documents` table, which already carries
   key/type/title/content/author_kind plus attachment columns and needs only `state`
   and cursor pagination, rather than standing up a second generic payload store;
   keep findings in `task_review_findings`, because `file_diff_hash`, `anchor_text`,
   `side` and the line range are what let a finding re-anchor when the diff moves
   under it; and widen the tool group from office-only to the kanban surface. The
   withdrawn requirements, their 20 verification cases and the sampled input shapes
   behind them remain recoverable from this repository's history at
   `docs/specs/typed-workflow-state/requirements/review-findings-access.md` (blob
   `b1eef82`, last touched in commit `755ba3830`). Tracked as card `fea202e7`.
