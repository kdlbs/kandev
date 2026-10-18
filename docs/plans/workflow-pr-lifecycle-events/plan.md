---
created: 2026-10-18
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-PR-EVENTS-001
system_design:
  - ../../specs/tasks/system-design/workflow-pr-lifecycle-events.md
legacy_specs: []
---

# Implementation Plan: Workflow PR Lifecycle Events

## Overview

Add `on_pr_merged` and `on_pr_closed` step events that move a task when its
linked GitHub PR is merged or closed, without an agent turn. One work order
covers the model, the orchestrator consumer, the step editor, tests, and docs.

## Scope

### In scope

- [REQ-TASKS-WORKFLOW-PR-EVENTS-001](../../specs/tasks/requirements/workflow-pr-lifecycle-events.md),
  AC `.1` through `.8`.
- Step event model, engine triggers and payloads, serialization, reference
  cleanup, DTOs, settings catalog, plugin admin merge, template loader.
- Orchestrator consumer of `github.task_pr.updated`, history trigger mapping.
- Step editor selectors and locale copy.
- Public workflow docs.

### Out of scope

- GitLab merge requests, automation changes, and step-entry evaluation of an
  already merged PR.

## Verification

- Go: engine trigger dispatch, portable export/YAML round trip and validation,
  step reference cleanup, template loader, and orchestrator consumer tests
  (merge, all-linked-PRs rule, close, unlinked PR, step without event,
  completed-only session, redelivery, outcome before step arrival,
  compare-and-swap against a newer move, retry after an error).
- Web: helper, hook, and selector unit tests.
- Playwright: a merged mock PR moves the task card to the target column and
  records `on_pr_merged` history; the step editor saves both events.

## Work orders

- [Task 01: Workflow PR lifecycle events](task-01-workflow-pr-lifecycle-events.md)
