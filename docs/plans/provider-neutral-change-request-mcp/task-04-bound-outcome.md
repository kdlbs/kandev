---
id: "04-bound-outcome"
title: "Bound outcome tool"
status: done
wave: 4
depends_on: ['03-automation-targets']
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.3
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 04: Bound outcome tool

## Summary

Expose the neutral outcome tool through the existing trusted GitHub reporting path.
Preserve the active-turn and attempt lifecycle without introducing GitLab reporting.

## In scope

- Add the neutral tool/action with only outcome and summary; retain old registration until task 05.
- Derive task/session from the principal and turn/contribution from the existing attempt service.
- Reuse first-write/replay and provider-progress semantics.
- Keep unmatched-turn guidance and fail closed on ordinary, stale, mismatched, or GitLab turns.
- Prepare outcome protocol generation to select the tool name supported by the bound runtime catalog; task 05 activates caller cutover.

## Out of scope

Provider-store redesign, new UI, cross-provider replacement, and GitLab outcome tracking.
Do not change unrelated provider algorithms or historical transcripts.

## Acceptance

- All three outcomes reach the same server-bound attempt and preserve duplicate/conflicting-report rules.
- Spoofed identity fields and reports from ordinary or GitLab turns cannot change GitHub attempt state.
- Protocol descriptions distinguish dispatched auto-fix from manual fixup and tool availability.

## Verification

Run from the repository root. Add failing tests first, then implement and run these checks.
Use existing package fixtures; keep all new tests inside this work order's listed suites.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/github -run 'AutoFix|CIAuto|Outcome' -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/pr_auto_fix_outcome_scope_test.go`
- `apps/backend/internal/mcp/handlers/task_pr_automation.go`
- `apps/backend/internal/mcp/handlers/task_pr_automation_outcome_scope_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/orchestrator/ci_automation_attempt.go`
- `apps/backend/internal/orchestrator/ci_automation_outcome_scope_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

Task 03. Use its DTOs and validated backend boundary.

## Risks

A neutral tool visible on a mixed task must not authorize a GitLab turn to report against a GitHub attempt.

## Parallelism

`sequential`. Shared schemas and registration files prevent parallel ownership.

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md), frontmatter IDs.
- [System design](../../specs/integrations/system-design/task-change-link-mcp.md), relevant contract sections.
- [Plan](plan.md), baseline, test matrix, and agent requests.
- Existing adjacent tests named in the plan and `apps/backend/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented the neutral bound outcome action and tool. It reuses the existing
active-turn and GitHub attempt semantics, rejects injected identity, keeps
GitLab outcome capability disabled, and selects the protocol wording from the
current execution catalog while preserving queued and edited prompt content.

Validation passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers -count=1`
- `go test ./internal/orchestrator ./internal/github -run 'AutoFix|CIAuto|Outcome' -count=1`
