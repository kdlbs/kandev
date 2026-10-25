---
id: "05-public-docs"
title: "Cancelled turn public documentation"
status: completed
wave: 3
depends_on: ["01-step-contract-and-template", "02-configuration-surfaces"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-cancelled-turn-completion.md"
---

# Task 05: Cancelled turn public documentation

## Acceptance

- User documentation explains pause-in-place versus configured complete-and-advance behavior, explicit user-only scope, signal/clarification interaction, and destination auto-start consequences.
- Reference documentation defines `cancel_triggers_turn_complete`, its `false` schema/import default, the new standard Kanban template default, and the absence of an upgrade backfill.
- Workflow import/export and sync examples preserve the field and public-doc validation passes.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`
- `docs/public/workflow-import-export.md`
- `docs/public/workflow-sync.md`

## Dependencies

Tasks 01 and 02.

## Parallelism

Parallel-safe with Task 04 after Task 02: public Markdown files are disjoint from frontend implementation files.

## Inputs

- Entire feature spec and ADR.
- Existing completion-signal sections in each public page.
- Docs-maintainer classification: import/export and sync are reference; tasks/workflows and tips remain task-oriented guidance/explanation.

## Risks

- Do not imply that every runtime cancellation advances or that existing standard workflows are automatically migrated.
- Distinguish the database/import default (`false`) from the new embedded `simple` template value (`true` on two steps).

## Output Contract

Report pages changed, each page's primary content type, exact validator results, blockers, and residual documentation risks. Update this task and `plan.md` status in the same conversation.

## Results

Updated the public workflow guides to document the explicit-cancellation policy, its narrow eligibility boundary, the shared completion pipeline, destination `on_enter` auto-start implications, built-in Kanban defaults, and portable import/sync behavior.

Verification:

- `rtk node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `rtk node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
