---
id: "03-update-queue-automation-documentation"
title: "Update Queue Automation Documentation"
status: completed
wave: 3
depends_on:
  - "01-persist-session-auto-merge-overrides"
  - "02-deliver-queue-automation-pills"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
  - REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.7
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.8
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.11
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.17
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.18
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.19
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.24
system_design:
  - ../../specs/ui/system-design/message-queue-automation-controls.md
---

# Task 03: Update Queue Automation Documentation

## Summary

Reconcile public queue guides, configuration reference, and WebSocket reference
with the delivered session override and compact header controls. Remove stale
claims that capacity is always rejected before a compatible automatic fold.

## In scope

- Explain global inheritance and permanent per-session override precedence in
  user-facing queue guidance.
- Describe the compact Auto-run and Auto-merge pills in the expanded queue
  header without confusing message merging with pull-request Auto-merge.
- Correct capacity automatic-fold behavior wherever public docs describe queue
  admission, including the staged-attachment exception.
- Add `queue_incarnation_id` to task-session fields. Document required
  `task_id`, `session_id`, and `session_incarnation_id` on status get and every
  queue mutation; the gateway/handler authorize that pair and pass the exact
  triplet unchanged. Document stale non-retargeting errors, operation-bound
  response/status snapshots, epoch/generation, availability, and effective
  Auto-merge value/source/revision. Task-only recounts remain internal.
- Keep configuration, operations, coordination, and session guidance consistent
  without copying detailed protocol text between pages.
- After implementation and behavioral checks pass, perform one lifecycle
  cutover: activate both replacement requirements; deprecate the two superseded
  requirements without changing their IDs or acceptance text; amend
  `message-queue-management.md` with the delivered compatible-fold and
  staged-attachment capacity exceptions; mark the system design current; and
  update the UI specification indexes.

## Out of scope

- New screenshots, diagrams, or a new public page.
- Changes to GitHub or GitLab pull-request automation documentation.
- Production implementation details already owned by Tasks 01 and 02.

## Acceptance

- Public docs state that untouched sessions follow later global changes and an
  explicitly changed session remains independent for that session's lifetime.
- Queue guidance names both compact pills, eligible direct folding at or above
  capacity, and staged-attachment rejection, with no contradictory claim in
  affected pages.
- The WebSocket reference documents exact action/field names, pair
  authorization, required task/session/incarnation payloads for get and every
  mutation, non-retargeting errors, operation-bound status snapshots,
  unavailable ordering, task-only scope, incarnation matching,
  source/revision, and effective-value semantics. Task-session responses expose
  immutable queue incarnation. The final cutover leaves
  replacement requirements active, superseded requirements deprecated with
  forward links, the management capacity text aligned to delivered behavior,
  and the system design current.

## Verification

```bash
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs
git diff --cached --check -- docs
git ls-files -z --others --exclude-standard -- docs \
  | xargs -0 -r -n1 sh -c 'test -z "$(git diff --no-index --check /dev/null "$1" 2>&1)"' sh
```

Run all commands from the repository root.

## Files likely touched

- `docs/public/coordination.md`
- `docs/public/operations.md`
- `docs/public/sessions-and-review.md`
- `docs/public/configuration.md`
- `docs/specs/ui/requirements/message-queue-automation-controls.md`
- `docs/specs/ui/requirements/message-queue-auto-merge-session-overrides.md`
- `docs/specs/ui/requirements/message-queue-run.md`
- `docs/specs/ui/requirements/message-queue-management.md`
- `docs/specs/ui/requirements/message-queue-auto-merge.md`
- `docs/specs/ui/system-design/message-queue-automation-controls.md`
- `docs/specs/ui/README.md`
- `docs/public/websocket-api.md`

## Dependencies

- Task 01 defines the final protocol and persistence behavior.
- Task 02 defines the final user-facing labels and responsive composition.
- This task performs the lifecycle cutover only after Tasks 01 and 02 have
  passed their behavioral verification; drafts must not become active earlier.

## Risks

- Existing pages describe both queued-message Auto-merge and pull-request
  Auto-merge; edits must remain scoped to message queues.
- Repeating the complete precedence contract across every guide would create
  another set of drift-prone sources instead of linking to the owning guidance.
- Activating replacements without deprecating and forward-linking the old
  contracts would leave implementers with conflicting authoritative behavior.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001`
- `REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001`
- `docs/specs/ui/system-design/message-queue-automation-controls.md`
- `docs/decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md`
- Current public queue sections in the files listed above.

## Results

Completed. Activated the replacement requirements and design, deprecated the
superseded queue contracts with forward links, recorded the inheritance
decision, and updated public configuration, coordination, operations, session,
and WebSocket documentation. Specification and public-doc validation pass.
