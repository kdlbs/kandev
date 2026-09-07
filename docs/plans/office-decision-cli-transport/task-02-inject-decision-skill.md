---
id: "02-inject-decision-skill"
title: "Inject the decision skill"
status: done
wave: 2
depends_on:
  - "01-task-bound-decision-transport"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUORUM-RECORDING-001
acceptance_criteria:
  - AC-TASKS-QUORUM-RECORDING-001.1
  - AC-TASKS-QUORUM-RECORDING-001.3
  - AC-TASKS-QUORUM-RECORDING-001.9
system_design:
  - ../../specs/tasks/system-design/workflow-quorum-decision-recording.md
---

# Task 02: Inject the Decision Skill

## Summary

Teach decision-seat holders the CLI contract through a bundled system skill.
Select that skill from the seat-derived advisory action and make the review and
approval prompts require the command as their final action.

## In scope

- Add `kandev-step-decision/SKILL.md` with no static role default.
- Append it to the run manifest only when the launch-time seat lookup grants
  `record_step_decision` as an advisory action.
- Update review and approval prompts from the MCP tool to the CLI command.
- Prove normal Kanban sessions receive no decision CLI skill or instruction.

## Out of scope

- Treating launch-time `AvailableActions` or JWT claims as authorization.
- Removing the MCP transport before the replacement guidance is green.

## Acceptance

- A reviewer or approver seat run receives the decision skill and final-action
  CLI prompt; a non-seat Office run does not receive the skill.
- The skill explains accepted verdicts, required reason, superseding repeats,
  structured output, and that comments or approval-inbox commands do not count.
- Kanban context and prompts contain no Office decision CLI instruction or
  decision metadata.

## Verification

```bash
cd apps/backend && rtk go test ./internal/office/configloader ./internal/office/skills ./internal/office/service -run 'Test.*(DecisionSkill|DecisionContract|ReviewStage|ApprovalStage|SkillManifest|SystemSkills)' -count=1
```

## Files likely touched

- `apps/backend/internal/office/configloader/skills/kandev-step-decision/SKILL.md`
- `apps/backend/internal/office/skills/system_sync_test.go`
- `apps/backend/internal/office/service/skill_manifest.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/office/service/prompt_builder_test.go`
- `apps/backend/internal/office/service/scheduler_integration_routing_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`

## Dependencies

- Task 01 provides the command that the skill teaches.

## Risks

- Static role defaults do not match workflow participant seats; selection must
  use the existing live-seat advisory lookup at launch.
- Prompt wording must not imply that launch-time skill availability grants
  authority after the participant slate changes.

## Parallelism

`sequential`

## Inputs

- The accepted advisory-actions ADR.
- Existing bundled skill synchronization and scheduler manifest pipeline.

## Results

Implemented the bundled `kandev-step-decision` skill without static role
defaults. The scheduler now builds the signed run context before resolving the
manifest, injects the skill only when the launch-time advisory decision action
is present, and filters the skill from non-seat runs. Review and approval
prompts use the task-bound CLI command as the final action and omit the Office
decision contract when the action is absent.

Verification:

```text
cd apps/backend && rtk go test ./internal/office/configloader ./internal/office/skills ./internal/office/service -run 'Test.*(DecisionSkill|DecisionContract|ReviewStage|ApprovalStage|SkillManifest|SystemSkills)' -count=1
```

Passed: 36 tests. The full affected package run also passed 564 tests.
