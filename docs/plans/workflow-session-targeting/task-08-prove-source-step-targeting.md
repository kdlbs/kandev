---
id: "08-prove-source-step-targeting"
title: "Prove source-step targeting"
status: complete
wave: 8
depends_on:
  - "07-expose-source-step-choices"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 08: Prove source-step targeting

## Summary

Prove exact source-step routing and author repair on desktop and phone.
Extend public docs and retain passing evidence for the initial-target milestone.

## In scope

- Two earlier source steps sharing a profile: verify the selected source's
  exact session ID receives one prompt; test reuse and fresh behavior.
- Source re-entry and skipped/terminal source fallback, with no source prompt
  replay. Backend tests cover remaining concurrency/transport combinations.
- Authoring, save/reload, duplicate/import remapping and synced inspection.
- Draft source reorder/removal, replacement target, explicit clearing, discard,
  failed-save preservation, and valid persistence after repair.
- Mobile drawer selection and stacked repair actions, containment, touch size,
  focus return, long-list scroll, and no horizontal document overflow.
- Public source-step eligibility, initial-versus-source distinction, lifecycle
  fallback, repair instructions and same-version unsupported-kind rejection.

## Out of scope

- Paid providers, full suites, broad QA and publication.
- A new UI composition; verify the plan's UI-01 through UI-04.

## Acceptance

- Runtime session IDs and unique prompt delivery prove source identity, including
  equal-profile sources; first-slice initial flows remain passing.
- Desktop and phone authors can repair invalid sources and persist valid targets.
  Inspect rendered captures for the planned drawer and repair geometry.
- Public docs match both completed slices; all work-order results and plan
  checkboxes accurately report evidence and environment limitations.

## Verification

Load /e2e and use isolated fixtures and causal waits. From `apps/web`, run
sequentially with the managed build enabled:

```bash
rtk pnpm e2e:run --host --shards 1 --project chromium -- tests/workflow/workflow-step-session-targeting.spec.ts tests/workflow/workflow-session-targeting.spec.ts
rtk pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/workflow/mobile-workflow-step-session-targeting.spec.ts tests/workflow/mobile-workflow-session-targeting.spec.ts
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Use /docs-maintainer. No generic verification or PR-publication task is added.

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-step-session-targeting.spec.ts` (new)
- `apps/web/e2e/tests/workflow/mobile-workflow-step-session-targeting.spec.ts` (new)
- Existing workflow agent-switch helpers and E2E API client types
- `docs/public/{tasks-and-workflows.md,workflow-import-export.md,workflow-sync.md}`
- Plan and work-order results

## Dependencies

Tasks 05-07 supply source targets. Tasks 01-04 remain the regression baseline.

## Risks

Profile labels alone do not prove conversation identity. A test must exercise
the visible repair action and saved result, not seed an already-repaired target.

## Inputs

- Requirement 002, design Source-step bindings and explicit repair flow.
- Plan UI-01 through UI-04; initial-slice E2E fixtures and existing tests.

## Parallelism

`sequential`

## Results

Added source-step runtime and repair coverage to the workflow targeting E2E
specs, with the initial behavior rerun in the same desktop suite. Desktop
workflow targeting passed four tests and mobile targeting passed one test.
Public docs, full spec lint, and whitespace validation passed.
