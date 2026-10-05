---
id: "02-agent-guidance"
title: "Index-aware project instructions"
status: complete
wave: 2
depends_on:
  - "01-starter-files"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-002
acceptance_criteria:
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-002.1
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-002.2
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-002.3
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-002.4
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-002.5
system_design:
  - ../../specs/projects/system-design/context-knowledge.md
---

# Task 02: Index-Aware Project Instructions

## Summary

Give every project role instructions for selective context reading and portable
knowledge authoring. Keep coordinator index maintenance and worker role boundaries
explicit on launch and resume.

## In scope

- Use TDD for coordinator, economy, and frontier instruction content.
- Describe index-first reading, notes/relevant documents, missing-index fallback,
  concept frontmatter, reserved files, and relative-link meaning.
- Keep coordinator notes concise and index descriptions current.
- Preserve unfamiliar metadata and prohibit invented source/verification claims.
- Extend the current lifecycle instruction-refresh test to prove fresh guidance
  and retain marker/path/role protections.

## Out of scope

- Reading entire context bundles into server prompts or executing metadata.
- New tools, providers, profiles, allowed roots, or delegation behavior.
- Model behavior evaluations or claims that instructions guarantee compliance.

## Acceptance

- All three project roles receive index-first guidance with the legacy fallback.
- Coordinator authoring guidance remains distinct from worker responsibilities,
  with current tools and trusted markers preserved.
- Launch/resume refresh replaces old guidance without inserting file bodies or
  granting metadata any authority.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/sysprompt)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test.*AgentProject')
git diff --check
```

## Files likely touched

- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, only if the
  focused refresh regression exposes missing integration.

## Dependencies

[Task 01](task-01-starter-files.md) supplies the new starter convention.

## Risks

- Changing the coordinator tool catalog or granting its role to a worker.
- Repeating stale instructions on resume or weakening quoted path handling.
- Suggesting that declared verification establishes permissions or factual truth.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/context-knowledge.md)
- [Agent guidance design](../../specs/projects/system-design/context-knowledge.md#agent-guidance)
- [Format decision](../../decisions/2026-10-05-project-context-knowledge-format.md)
- `AgentProjectInstructions`, `InjectAgentProjectInstructions`, and
  `TestApplyAgentProjectInstructionsAddsCurrentWorkspaceAndRefreshesMetadata`.
- Scoped backend instructions and the existing trusted-system-content tests.

## Results

Implemented index-first shared guidance for coordinator, economy, and frontier
roles. The common instructions cover the missing-index fallback, concept
frontmatter, reserved index/log files, link resolution, legacy and unfamiliar
metadata preservation, evidence-backed provenance, and the boundary between
project data and server authority. Coordinator-only guidance keeps notes short,
places detailed knowledge in separate files, and maintains index descriptions
and links. Existing role, path quoting, trusted marker, and lifecycle refresh
behavior remains intact; the server instruction builder receives paths only and
does not read context file bodies.

Verification on 2026-10-05:

- `cd apps/backend && go test ./internal/sysprompt -run 'TestAgentProjectInstructionsKnowledgeGuidance' -count=1`: passed after the implementation.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestApplyAgentProjectInstructionsAddsCurrentWorkspaceAndRefreshesMetadata$' -count=1`: passed.
- `cd apps/backend && go test -race ./internal/sysprompt`: passed.
- `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test.*AgentProject'`: passed.
- `git diff --check`: passed.
