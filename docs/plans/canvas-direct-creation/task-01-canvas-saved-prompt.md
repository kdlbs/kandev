---
id: "01-canvas-saved-prompt"
title: "Seed the canvas saved prompt"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-009
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-009.8
  - AC-CANVASES-AGENT-WEB-APPS-009.9
  - AC-CANVASES-AGENT-WEB-APPS-009.10
system_design:
  - ../../specs/canvases/system-design/guided-canvas-task-launch.md
---

# Task 01: Seed the canvas saved prompt

## Summary

Provide an editable `create-canvas` saved prompt with the canvas authoring
instructions. Prove that structured task launch expands it through the existing
backend service while retaining the visible reference.

## In scope

- Add the embedded English definition and `builtin-create-canvas` seed entry.
- Preserve same-name user records, edits, existing seeding and deletion rules.
- Cover create/start and delayed start, workflow and no-step launch, prepared
  session, current saved content, removed reference, and exactly one expansion.
- Assert the canonical recorded message and agent prompt agree; task description
  stays short. Reuse current trusted-content plumbing and failure behavior.

## Out of scope

- Browser changes, new expansion services, schemas, reserved prompt names,
  runtime flag changes, and terminal/passthrough expansion.

## Acceptance

1. Fresh and existing databases obtain the built-in without replacing user data.
2. The default definition includes exact tools, discovery, one core skill read,
   source boundaries, authorized data, publication outcomes, and human promotion.
3. Structured launch delivers one trusted saved definition; removing the
   reference removes its expansion. Delayed start uses the current definition.

## Verification

Run from the repository root, following `/tdd` and backend scoped guidance:

```bash
(cd apps/backend && rtk go test ./internal/prompts/store ./internal/prompts/service)
(cd apps/backend && rtk go test ./internal/orchestrator -run 'TestCreateCanvasPromptLaunch|PromptReference|PromptLaunch|WorkflowPrompt' -count=1)
rtk git diff --check
```

Name new integration tests `TestCreateCanvasPromptLaunch` with subtests so the
command includes every new case. Inspect existing saved-prompt launch tests
before adding fixtures; use the real prompt repository/service. Add a narrow
repair only if a regression test proves the existing launch path is incomplete.

## Files likely touched

- `apps/backend/config/prompts/create-canvas.md` (new)
- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/store/sqlite_test.go`
- `apps/backend/internal/orchestrator/prompt_launch_fallback_test.go`
- `apps/backend/internal/orchestrator/workflow_prompt_test.go`

## Dependencies

None.

## Risks

Seeding has fixed-ID and unique-name conflicts; test both. Prompt instructions
do not grant canvas capabilities. User-customized definitions can intentionally
differ from the shipped workflow.

## Parallelism

`sequential`

## Inputs

- Canvas design: Guided canvas task launch and Agent authoring guidance.
- [Saved Prompt Delivery](../../specs/tasks/system-design/saved-prompt-delivery.md).
- [Server-owned expansion ADR](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).
- `config/prompts/changes-walkthrough.md`, current seed and expansion tests.

## Results

- Added `apps/backend/config/prompts/create-canvas.md` and registered the
  `builtin-create-canvas` seed without changing saved-prompt resolution.
- Added store coverage for the complete authoring workflow, same-name user
  collisions, and user edits across repository reopen.
- Added orchestrator coverage for new-task, prepared-session, workflow-step,
  current-definition, exactly-once expansion, and removed-reference launches.
- `cd apps/backend && rtk go test ./internal/prompts/store ./internal/prompts/service`:
  55 passed.
- `cd apps/backend && rtk go test ./internal/orchestrator -run
  'TestCreateCanvasPromptLaunch|PromptReference|PromptLaunch|WorkflowPrompt' -count=1`:
  26 passed.
- `rtk git diff --check`: passed.
