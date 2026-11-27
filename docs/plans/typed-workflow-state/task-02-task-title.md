---
id: "02-task-title"
title: "Task title placeholder in workflow prompts"
status: done
wave: 2
depends_on:
  - "01-step-entry-number"
plan: "plan.md"
requirements:
  - REQ-TWS-006
acceptance_criteria:
  - AC-TWS-006.1
  - AC-TWS-006.2
  - AC-TWS-006.3
  - AC-TWS-006.4
  - AC-TWS-006.5
  - AC-TWS-006.6
  - AC-TWS-006.7
  - AC-TWS-006.8
  - AC-TWS-006.9
system_design:
  - ../../specs/typed-workflow-state/system-design/typed-workflow-state.md
---

# Task 02: Task title placeholder in workflow prompts

## Scope

Substitute `{task_title}` in step and workflow-level prompt templates with the
task's title, looked up only when the token is present. The title is spliced in
after every other placeholder so its text is never scanned for tokens.

## Files touched

- `apps/backend/internal/orchestrator/task_title_prompt.go`
- `apps/backend/internal/orchestrator/task_title_prompt_test.go`
- `apps/backend/internal/orchestrator/task_title_prompt_paths_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `docs/public/workflow-tips.md`

## Verification

- The token is replaced at every occurrence (AC-TWS-006.1) on both call sites
  (AC-TWS-006.2). A template without it issues no lookup and renders
  byte-identically (AC-TWS-006.3).
- A lookup error or an empty task identifier leaves the token literal without
  failing prompt building (AC-TWS-006.4, AC-TWS-006.5).
- A literal token in the base prompt is never substituted (AC-TWS-006.6). On
  the step path the token is first replaced with a per-build sentinel and the
  title is swapped in after `{task_id}` and `{{task_prompt}}` substitution; on
  the workflow-level path the title is substituted last. Title text is
  therefore never expanded (AC-TWS-006.7), and the other placeholders keep
  their behaviour (AC-TWS-006.8). Saved-prompt references in the title expand
  as they do in the base prompt (AC-TWS-006.9).
- `go test ./apps/backend/internal/orchestrator/...` green. `gofmt`, `go vet`,
  and `golangci-lint` clean.
