---
created: 2026-10-29
status: done
requirements:
  - REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001
system_design:
  - ../../specs/office/system-design/routine-trigger-update.md
legacy_specs: []
---

# Implementation Plan: Update an existing routine trigger in place

## Overview

Changing a routine's schedule replaced the whole trigger. That discarded the
trigger ID and made a claimed cron slot difficult to edit safely. This plan
records the delivery of an in-place schedule update:

- `PATCH /api/v1/office/routine-triggers/:triggerId` changes
  `cron_expression`, `timezone`, and/or `enabled`.
- The trigger ID, routine, kind, webhook authentication, creation time, last
  fire, and past runs are preserved.
- The repository update compares the observed identity and schedule, including
  `next_run_at`, with the stored row, so a scheduler claim or concurrent edit
  returns 409 instead of overwriting a claimed fire.
- Disabling clears the next fire but does not cancel an already claimed fire.

The authoritative pair is
[REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001](../../specs/office/requirements/routine-trigger-update.md)
and the
[Office routine trigger update design](../../specs/office/system-design/routine-trigger-update.md).

## Scope

In scope:

- The PATCH route, request DTO, handler, and 400/404/409 mapping.
- Cron-only schedule fields; webhook triggers may only toggle `enabled`.
- The compare-and-set repository update from the writer connection.
- Public routine documentation for the endpoint.

Out of scope:

- Replacing, deleting, or replaying triggers and runs.
- Changing scheduler claim, re-arm, or concurrency behavior.
- Review-thread changes or any public PR update.

## Verification

- This delivery package adds documentation only. Coverage is proven by the
  repository's `.github/scripts/pr-docs.cjs` validator reading this work order,
  its sibling plan, `REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001`, and the referenced design at
  the PR head.
- The non-documentation checks on this head pass, including the fork preview
  build, review checks, and the existing Office route scope checks. No new
  permanent test is added by this delta.

## Work orders

- [Task 01: Update an existing routine trigger in place](task-01-update-trigger-in-place.md)
