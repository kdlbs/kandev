---
created: 2026-09-09
status: done
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
legacy_specs: []
---

# Implementation Plan: Saved prompt expansion without workflow steps

## Overview

Make initial structured prompts expand saved references when workflow
composition does not run. One sequential work order implements the fallback
and verifies delivery through both launch entry points.

The task system owns this repair because it owns launch prompt preparation,
message persistence, and agent dispatch. Its existing saved-prompt requirement
now includes acceptance criteria 001.9 through 001.11 for the missing behavior.
The existing server-owned expansion ADR remains applicable; no new ADR is needed.

## Evidence and root cause

`applyWorkflowAndPlanModeWithPromptContext` resolves references only inside
its successful workflow-step branch. An empty step ID leaves the original
prompt and an empty trusted-context value.

A temporary test used the real SQLite prompt service, workflow preparation,
and final launch-context injection. With two saved definitions, plain
`Follow @principles and @operator-voice` delivered both with a step and neither
without one. Six diagnostic cases passed; 21 existing resolver tests passed.
The temporary test was removed. These results establish the defect, not a fix.

## Scope

### In scope

- Expand structured launch prompts whenever workflow composition is skipped.
- Preserve accepted direct-message definitions and trusted provenance.
- Verify persisted and dispatched context through new and prepared launches.
- Document the supported no-step behavior with the implementation.

### Out of scope

- Changes to mention matching, including Markdown and punctuation boundaries.
- Workflow-step inference, routing, profile selection, or launch scheduling.
- Terminal expansion, queue editing, schema changes, or frontend changes.

## Technical approach

Update `apps/backend/internal/orchestrator/task_operations.go`, primarily
`applyWorkflowAndPlanModeWithPromptContext`. Track whether workflow composition
ran, then prepare the base prompt on the fallback path before plan-mode injection.
Use `expandPromptReferencesWithContext` when no trusted context exists; otherwise
preserve and return the accepted context without another repository lookup.

Keep the successful workflow path intact. Do not infer a failed preparation
from an empty expansion, since unknown references legitimately produce one.
Carry generated context through the existing launch injectors. Never trust a
request-supplied expansion block by its shape alone.

Use a focused helper if needed to remain within Go complexity limits. Keep new
tests in `prompt_launch_fallback_test.go` rather than extending the large
`task_operations_test.go`. Use a temporary SQLite store and real prompt service
for lookup and sanitization evidence. Add a sentence to the saved-prompt section
of `docs/public/developer-tools.md` when the behavior is implemented.

## Tests

All new tests live in
`apps/backend/internal/orchestrator/prompt_launch_fallback_test.go`.

| Acceptance criteria | Planned evidence |
| --- | --- |
| 001.9, 001.10 | `TestApplyWorkflowAndPlanMode_ExpandsWithoutWorkflowComposition`: empty step ID, ephemeral task, nil getter, failed lookup; assert both visible references and real saved definitions. |
| 001.11 | `TestApplyWorkflowAndPlanMode_PreservesAcceptedContextWithoutWorkflow` and `TestStartCreatedSession_PreservesAcceptedPromptContextWithoutWorkflowStep`: change saved content after acceptance; assert the original definition and no duplicate expansion through preparation and dispatch. |
| 001.4, 001.5, 001.8 | `TestApplyWorkflowAndPlanMode_WithoutWorkflowGuards`: forged blocks, missing references, lookup failure, passthrough, absent expander, and empty prompts. |
| 001.6, 001.9 | `TestLaunchSession_ExpandsSavedPromptsWithoutWorkflowStep` and `TestStartCreatedSession_ExpandsSavedPromptsWithoutWorkflowStep`: capture the outgoing prompt and stored message through production entry points. |

Run the first regression before editing production code. It must fail because
the saved definitions and trusted context are absent. Include a normal workflow
control to detect duplicate lookups and preserve existing composition behavior.

## End-to-end evidence

The launch integration tests traverse preparation, canonicalization, persistence,
and the agent-manager dispatch boundary. Use the existing orchestrator service
harness and mock only external agent execution. No new browser interaction is
needed for the missing launch argument; this repair changes no UI control.

## Work orders

- [x] [Task 01: Expand launch prompts without workflow composition](task-01-expand-launch-prompts.md)

## Verification results

Implementation and regression checks passed on 2026-09-09. Diagnostic evidence
is recorded above. Package validation passed on 2026-09-09:

- `rtk python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `rtk python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `rtk git diff --check`: passed.
- `rtk go test -race ./internal/orchestrator -run 'Test(ApplyWorkflowAndPlanMode|BuildWorkflowPrompt|LaunchSession_ExpandsSavedPromptsWithoutWorkflowStep|StartCreatedSession_ExpandsSavedPromptsWithoutWorkflowStep|StartTask_PreservesOnlyResolvedWorkflowPromptExpansion|StartCreatedSession_PreservesOnlyResolvedWorkflowPromptExpansion)' -count=1`: 38 tests passed.
- `rtk go test -race ./internal/prompts/service ./internal/sysprompt -count=1`: 95 tests passed.
- `rtk go test -race ./internal/task/handlers -run 'TestWSAddMessage_(PreparesSavedPromptBeforePersistenceAndDispatch|PassesTrustedPromptContextToCreatedSessionStart)' -count=1`: 2 tests passed.
- `rtk node --test scripts/validate-public-docs.test.mjs`: 61 tests passed.
- `rtk node scripts/validate-public-docs.mjs`: 46 published docs pages validated.

## Risks

- Resolving accepted context again can make history differ from agent input.
- Dropping the trusted return value lets canonicalization remove the expansion.
- An unconditional second expansion can duplicate lookups on workflow launches.
- Applying hidden expansion to passthrough sessions exposes it in terminal input.
