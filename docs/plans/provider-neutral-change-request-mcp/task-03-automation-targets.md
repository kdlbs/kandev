---
id: "03-automation-targets"
title: "Explicit automation targets"
status: done
wave: 3
depends_on: ['02-read-capabilities']
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.5
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 03: Explicit automation targets

## Summary

Implement one neutral automation update tool with explicit association and task/provider targets.
Reuse provider transactions and report cross-provider partial application.

## In scope

- Add strict target/patch schemas and typed backend validation.
- Map canonical GitHub identity to PRNumber and unique GitLab identity to ProjectPath/MRIID internally.
- Enforce principal binding, origin checks, live membership, provider capabilities, and workspace credentials.
- Preflight every selected provider before writes; reject empty switch targets and all invalid fields.
- Apply providers in lexical order; stop after failure and preserve provider-specific events.
- Expose affected identities and applied/failed/not_attempted results, including unknown state after readback failure.
- Keep prompt scope at task/provider and reject prompt fields with association targets.

## Out of scope

Provider-store redesign, new UI, cross-provider replacement, and GitLab outcome tracking.
Do not change unrelated provider algorithms or historical transcripts.

## Acceptance

- Tests cover one association, one provider, both providers, prompt-only empty tasks, mixed patches, and false/empty-string values.
- Unsupported, ambiguous, foreign-workspace, partial-identity, and missing-credential requests cause no preflight writes.
- Second-provider failure preserves the first provider's result without rollback or a false all-or-nothing claim.

## Verification

Run from the repository root. Add failing tests first, then implement and run these checks.
Use existing package fixtures; keep all new tests inside this work order's listed suites.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -count=1)
(cd apps/backend && go test ./internal/github ./internal/gitlab -run 'TaskCIOptions|TaskMRAutomation|UpdateTaskMRAutomation|UpdateTaskCIOptions' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'ChangeRequestAutomation|ChangeRequestGitLab' -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/task_change_request_tools_test.go`
- `apps/backend/internal/mcp/handlers/task_change_request.go`
- `apps/backend/internal/backendapp/task_change_request_automation.go (new)`
- `apps/backend/internal/backendapp/task_change_request_automation_test.go (new)`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

Task 02. Use its DTOs and validated backend boundary.

## Risks

Retries can reset provider checkpoints. Return precise partial state and avoid automatic compensation.

## Parallelism

`sequential`. Shared schemas and registration files prevent parallel ownership.

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md), frontmatter IDs.
- [System design](../../specs/integrations/system-design/task-change-link-mcp.md), relevant contract sections.
- [Plan](plan.md), baseline, test matrix, and agent requests.
- Existing adjacent tests named in the plan and `apps/backend/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented explicit association and task/provider automation targets. The
coordinator preflights every selected provider before writing, applies providers
in lexical order, preserves partial results after a later provider failure, and
keeps prompts at task/provider scope.

Validation passed:

- `go test ./internal/backendapp -run 'TestChangeRequestAutomation' -count=1`
- `go test ./internal/mcp/handlers -run 'Test(ParseTaskChangeRequestAutomationPayload|UpdateTaskChangeRequestAutomation)' -count=1`
- `go test -race ./internal/backendapp -run 'TaskChange|ChangeRequest' -count=1`
- `go test ./internal/backendapp -run 'TestChangeRequestAutomationAssociationReportsOnlyTargetedGitLabMR' -count=1`
  passed with a production-shaped task-wide provider response containing two
  MRs.
