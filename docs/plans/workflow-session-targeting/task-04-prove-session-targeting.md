---
id: "04-prove-session-targeting"
title: "Prove initial session targeting"
status: complete
wave: 4
depends_on:
  - "03-expose-session-choices"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 04: Prove initial session targeting

## Summary

Prove the author's Plan/Implement/Review flow through real UI and mock-agent
execution. Complete the first product milestone with public guidance for initial
routing and version 2. Earlier-step support follows in Tasks 05-08.

## In scope

- Desktop authoring with the initial profile selected only at task creation;
  assert actual session IDs and unique Review prompt ownership for reuse/new.
- Terminal-source fallback and unchanged legacy profile-only agent switching.
- Mobile authoring with a long profile catalog, no-results search, lifecycle
  controls visible at the top, save/reload, and receiving-conversation inspection.
- Phone touch hitboxes, viewport containment, keyboard/dismiss/focus behavior,
  safe-area spacing, and zero document horizontal overflow. Inspect rendered
  desktop and phone captures from the focused tests.
- Public how-to/explanation for parking Sol and returning from Luna; reference
  docs for version 2 targets and version 1 compatibility.

## Out of scope

- Earlier-step targets and their binding/repair behavior.
- Broad QA, full test suite, PR publication, actual paid provider execution.

## Acceptance

- Desktop and mobile flows prove which conversation received the Review prompt,
  not only which profile label appeared in the editor.
- Lifecycle navigation remains reachable throughout long-list scrolling and
  filtering; the mobile surface follows the planned drawer geometry.
- Public docs describe implemented behavior and tests/results are recorded in
  the plan and work orders, including any environment limitations.

## Verification

Load `/e2e` before writing tests and use its causal waits and isolated fixtures.
From `apps/web`, run these sequentially. The managed runner builds the current
backend and web before each invocation; do not use stale binaries:

```bash
rtk pnpm e2e:run --host --shards 1 --project chromium -- tests/workflow/workflow-session-targeting.spec.ts tests/workflow/workflow-agent-profile.spec.ts
rtk pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/workflow/mobile-workflow-session-targeting.spec.ts tests/workflow/mobile-workflow-agent-switch.spec.ts
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Use `/docs-maintainer` when updating the public pages. No public page should
present an unimplemented fallback or target as available.

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-session-targeting.spec.ts` (new)
- `apps/web/e2e/tests/workflow/mobile-workflow-session-targeting.spec.ts` (new)
- `apps/web/e2e/tests/workflow/workflow-agent-switch-helpers.ts`
- `apps/web/e2e/helpers/api-client.ts` and workflow helper types as needed
- `docs/public/tasks-and-workflows.md` (explanation/how-to section)
- `docs/public/workflow-import-export.md` (reference)
- `docs/public/workflow-sync.md` (reference to supported target versions)
- This plan and work-order Results sections

## Dependencies

Tasks 01-03 supply the complete initial-target slice. Follow the E2E resource guards.
Complete and verify this milestone before beginning Task 05. Do not describe
source-step targets as available in public docs yet.

## Risks

A test that checks only primary-profile labels can miss routing to the wrong
conversation. Large-catalog tests need enough rows to cause internal scrolling.
Mock-agent evidence verifies routing, not provider-specific resume reliability.

## Parallelism

`sequential`

## Inputs

- Requirement example and design Verification design section.
- Existing workflow agent-switch helpers and mobile session picker test.
- `/e2e`, `/mobile-parity`, and `/docs-maintainer`.

## Results

Added desktop and mobile workflow targeting coverage and public documentation
for the initial target behavior. The desktop workflow spec passed four
scenarios, including reuse, fresh routing, source targeting, and repair; the
mobile spec passed its lifecycle/drawer scenario. Public docs and spec
validation passed.
