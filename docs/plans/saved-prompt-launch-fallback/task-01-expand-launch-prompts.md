---
id: "01-expand-launch-prompts"
title: "Expand launch prompts without workflow composition"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.4
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.5
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.6
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.8
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.9
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.10
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.11
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
---

# Task 01: Expand launch prompts without workflow composition

## Summary

Make saved-prompt preparation independent of workflow composition on structured
launches. Preserve accepted definitions and pass their exact context through
canonicalization to message storage and agent dispatch.

## In scope

- The fallback in `applyWorkflowAndPlanModeWithPromptContext` and a focused
  private helper if necessary.
- The test matrix and named regression tests in the plan, using the real
  prompt service and production launch entry points.
- Public saved-prompt reference documentation, in the existing explanation
  section of `docs/public/developer-tools.md`.

## Out of scope

Mention syntax changes, workflow selection, frontend changes, terminal
expansion, queue changes, and new persistence contracts.

## Acceptance

1. The no-step regression fails before the correction and passes afterward.
   All skipped-composition cases deliver real saved definitions.
2. Launch and prepared-session tests show matching persisted and dispatched
   expansion content, retained accepted context, and excluded forged definitions.
3. Workflow controls and passthrough guards pass; public docs describe the
   no-step behavior only after implementation.

## Verification

Run from `apps/backend`. First run the named red test before production edits:

```bash
rtk go test ./internal/orchestrator -run '^TestApplyWorkflowAndPlanMode_ExpandsWithoutWorkflowComposition$' -count=1
```

After implementation:

```bash
rtk go test -race ./internal/orchestrator -run 'Test(ApplyWorkflowAndPlanMode|BuildWorkflowPrompt|LaunchSession_ExpandsSavedPromptsWithoutWorkflowStep|StartCreatedSession_ExpandsSavedPromptsWithoutWorkflowStep|StartTask_PreservesOnlyResolvedWorkflowPromptExpansion|StartCreatedSession_PreservesOnlyResolvedWorkflowPromptExpansion)' -count=1
rtk go test -race ./internal/prompts/service ./internal/sysprompt -count=1
rtk go test -race ./internal/task/handlers -run 'TestWSAddMessage_(PreparesSavedPromptBeforePersistenceAndDispatch|PassesTrustedPromptContextToCreatedSessionStart)' -count=1
```

Run from the repository root for documentation and formatting:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/prompt_launch_fallback_test.go` (new)
- `docs/public/developer-tools.md`
- This work order and `plan.md` for status and results.

## Dependencies

None.

## Risks

Preserve the exact-content trust rule from the existing ADR. Treat an accepted
definition as a snapshot. Detect skipped composition explicitly; an empty
context string does not prove the resolver was skipped.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/saved-prompt-delivery.md),
  acceptance criteria listed above.
- [Design](../../specs/tasks/system-design/saved-prompt-delivery.md),
  Launches without workflow composition and Security.
- [Server-owned expansion ADR](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).
- `workflow_prompt_test.go` for composition and accepted-context tests.
- `task_operations_test.go` for launch fixtures and captured agent prompts.
- `internal/prompts/service/service_test.go` for a real SQLite prompt service.

## Results

Implemented the fallback in `applyWorkflowAndPlanModeWithPromptContext`.
Skipped workflow composition now expands saved-prompt references for structured
launches, while accepted trusted context is preserved without re-reading mutable
saved records. Added real SQLite-backed coverage for empty, ephemeral, missing,
and failed workflow-step preparation, launch persistence and dispatch equality,
trust preservation, forged-content guards, missing references, and passthrough
exclusion. Documented the no-step launch behavior in the developer tools guide.

Verification passed on 2026-09-09:

- `rtk go test -race ./internal/orchestrator -run 'Test(ApplyWorkflowAndPlanMode|BuildWorkflowPrompt|LaunchSession_ExpandsSavedPromptsWithoutWorkflowStep|StartCreatedSession_ExpandsSavedPromptsWithoutWorkflowStep|StartTask_PreservesOnlyResolvedWorkflowPromptExpansion|StartCreatedSession_PreservesOnlyResolvedWorkflowPromptExpansion)' -count=1`
- `rtk go test -race ./internal/prompts/service ./internal/sysprompt -count=1`
- `rtk go test -race ./internal/task/handlers -run 'TestWSAddMessage_(PreparesSavedPromptBeforePersistenceAndDispatch|PassesTrustedPromptContextToCreatedSessionStart)' -count=1`
- `rtk go test -race ./internal/orchestrator -run '^TestStartCreatedSession_PreservesAcceptedPromptContextWithoutWorkflowStep$' -count=1`
- `rtk node --test scripts/validate-public-docs.test.mjs`
- `rtk node scripts/validate-public-docs.mjs`
- `rtk python3 scripts/lint-spec-files.py --all`
- `rtk git diff --check`
