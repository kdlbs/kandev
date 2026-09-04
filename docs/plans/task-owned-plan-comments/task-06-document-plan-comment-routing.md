---
id: "06-document-plan-comment-routing"
title: "Document plan-comment routing"
status: completed
wave: 4
depends_on: ["03-project-comments-across-task-sessions"]
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.1
  - AC-TASKS-PLAN-COMMENTS-002.1
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.2
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 06: Document plan-comment routing

## Summary

Update the public task how-to so the visible Plan-comment workflow matches the
new ownership and routing contract. Keep the existing page concise and avoid a
new navigation entry.

## In scope

- Explain that Add creates task-level pending feedback shown at every session
  composer.
- Distinguish ordinary Send to the selected session from Run to the current
  primary.
- State that accepted delivery clears the shared pending comment.

## Out of scope

- Internal WebSocket fields, schema details, or migration mechanics.
- A new public page or screenshot set.

## Acceptance

- `docs/public/tasks-and-workflows.md` accurately explains ownership,
  visibility, both routing actions, and accepted-delivery clearing without
  exposing implementation terminology.
- Public documentation validators pass and existing internal links remain
  valid.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 03.

## Risks

- Saying “the agent” would preserve the current ambiguity; the page must name
  selected and primary sessions explicitly.

## Parallelism

`parallel-safe` with Task 04.

## Inputs

- Requirements: task ownership, composer Send, and Run routing.
- System design: ownership decision and frontend projection.
- Existing Plan workflow in `docs/public/tasks-and-workflows.md`.

## Results

- Updated the public task workflow to describe task-level pending comments,
  all-session composer visibility, selected-session Send, primary-session Run,
  and clearing after accepted delivery.
- All 61 public-doc validator tests and validation of all 42 published pages
  pass.
