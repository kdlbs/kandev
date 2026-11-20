---
status: draft
system: workspaces
created: 2026-11-16
requirements:
  - REQ-WORKSPACES-IMPROVE-VALIDATION-001
  - REQ-WORKSPACES-IMPROVE-VALIDATION-002
owners:
  - kandev
---

# Improve Kandev validation design

## Ownership and mapping

Workspace ownership follows the Improve workflow's persisted instructions.
Shared engineering skills supply the validation procedure. Office retains its delivery contract.

| Requirement | Design sections |
| --- | --- |
| REQ-WORKSPACES-IMPROVE-VALIDATION-001 | Lifecycle evidence, Office boundary proof |
| REQ-WORKSPACES-IMPROVE-VALIDATION-002 | Stored instruction upgrades, Failure and compatibility |

## Evidence and current behavior

The [original blocker requirements](https://github.com/kdlbs/kandev/blob/5c9848451a57aa5927105496cb58bb01ffa8dc68/docs/specs/office/requirements/blocker-terminal-policy.md#L119)
required active-run merging and pause/replay before the maintainer fixup.
The completed work order did not provide corresponding scenario proof.
The original pause test exercised a tree hold rather than failure during a workspace pause.

`config/workflows/improve-kandev.yml` currently requests general TDD and broad checks.
`improvekandev.Handler.findAndHealWorkflow` reuses instances and only heals visibility.
`workflow/repository.seedSystemTemplates` uses `ON CONFLICT DO NOTHING`.
`workflow/service.CreateStepsFromTemplate` copies prompts from stored template rows.
Thus both stored templates and materialized steps can retain older instructions.

A temporary SQLite close/reopen probe confirmed retention of a stored step prompt.
That probe did not alter the stored template. It did not prove template refresh across a binary upgrade.

## Lifecycle evidence

Add one conditional reference under `.agents/skills/tdd/references/lifecycle-evidence.md`.
Link it from the Improve prompt, `/plan`, `/tdd`, and the existing review guidance.
Keep the scenario definitions in that reference rather than copying checklists across skills.

The checklist starts with three questions: what happens while paused, after restart, and after duplicate delivery?
Add the relevant regression test. Do not create a universal test matrix for changes that do not need it.
Use existing work-order results to record the proof:

| Scenario | Required proof |
| --- | --- |
| Pause | No prohibited effect occurs. Required intent remains durable, or the contract explicitly permits discard. |
| Resume and recovery | A missed hook does not lose required work. Repeated replay does not duplicate its effect. |
| Restart | Reopen the same persisted state with fresh service instances. A direct recovery-method call alone does not prove restart survival. |
| Duplicate and stale input | Stable occurrence identity survives metadata edits. Old owners cannot change successor work. |
| Active and merged work | Exercise eligible active runs and preserve every required payload detail. |
| Entry paths | Exercise each distinct API, UI, runtime, and event path that implements the contract. |

The mapping includes the owning AC, exact test name, command, result, or exclusion reason.
Existing work-order results remain the evidence record. No second verification ledger or generic local review gate is added.
Harness tests protect instruction routing and reference availability. They do not certify the actual feature's behavior.

The Improve verification step follows the current work order's checks.
The Test phase uses the existing `scripts/dev-isolated` procedure and reports the relevant user scenarios.
The PR phase carries the final evidence summary into the existing publication workflow.
No automatic phase transition or new approval prompt is introduced.

## Office boundary proof

Clarify `apps/backend/internal/office/AGENTS.md` at the existing run-ownership trap.
Distinguish durable event intent, shared wakeup dispatch, and low-level run admission.
Specify the producer's pause policy and replay owner before choosing an entry point.
Do not require retention for sources whose accepted contract intentionally skips work while paused.

Extend the real blocker fixture with an owned on-disk database and fresh reconstruction support.
Keep this helper in test code. Reuse the production task publisher, repositories, pause service, dispatcher, and Office service.
Cover failure during pause, close/reopen, resume, missed resume hook, repeated recovery, and stale-alert discard.
Existing occurrence-identity, active-run merge, and full-prompt tests remain the primary owners of their contracts.
The [Office requirement](../../office/requirements/blocker-terminal-policy.md) and
[Office design](../../office/system-design/blocker-terminal-policy.md) remain authoritative.

## Stored instruction upgrades

Implement a narrow reconciler in `internal/workflow/repository` during schema/default initialization.
Run it after system template insertion and before task intake can use those templates.
No application schema or migration-version table is required.

Keep a small fixture with the preceding shipped prompt text for the changed Improve steps.
Read it with the production YAML loader, which trims prompt text, and compute its SHA-256 hash after applying the same normalization to stored text.
Compute the current-prompt hash from the current embedded template. This is a bounded comparison against the known preceding revision, not a hash registry.
Start with the pre-change `0f072479006f` revision. Add older text only for a demonstrated upgrade need.
No hash registry, new version table, or general migration framework is needed.

Reconcile two storage layers:

1. For the system template with ID `improve-kandev`, replace only known historical step prompt fields in its stored `steps` JSON.
2. For workflow instances bound to that template, replace only known historical prompts on the corresponding named steps.

Improve instances are not reliably marked `is_system`. Do not use that flag alone to select materialized steps.
When the stored template changes, retain unknown JSON fields and unrelated step properties.
Preserve IDs, ordering, events, profiles, workspace bindings, hidden state, and creation timestamps.

Only refresh a template or workflow step whose `created_at` equals `updated_at`.
Each update also compares its original prompt or JSON value and timestamps in the write predicate.
If the template compare fails, reread the row once. Retry the prompt-only merge against the new JSON only when its timestamps still match and the target prompt still hashes to the known preceding revision.
Preserve unrelated fields from that reread. If the prompt changed, the row is marked edited, or the retry compare also fails, leave customized content untouched; an unresolved retry conflict fails repository construction.
Workflow-step compare conflicts do not retry.
Update timestamps only after an actual prompt change.
An already current prompt is a no-op. Unknown content stays unchanged with an ID-only diagnostic, never prompt text.

## Failure and compatibility

Required reconciliation errors propagate through repository construction and stop startup admission.
Unknown content is a preservation outcome, not a database error.
Repeated startup and repeated bootstrap reuse must converge without additional rows or writes.
Run SQLite close/reopen and environment-gated PostgreSQL tests for both storage layers.
Do not append new instructions to an accepted turn or mutate retained transcripts.

## Related sources

- [Improve workspace design](improve-kandev.md)
- [Prompt seeding precedent](../../../../apps/backend/internal/prompts/store/sqlite.go)
- [Implementation plan](../../../plans/improve-kandev-lifecycle-validation/plan.md)
