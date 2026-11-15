---
id: "00-restart-reproduction"
title: "Reproduce and classify the reported restart flow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PROFILE-CONTINUITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.1
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.2
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.3
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.4
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.6
system_design:
  - ../../specs/agents/system-design/mcp-profile-continuity.md
---

# Task 00: Reproduce and classify the reported restart flow

## Summary

Establish whether the reported restart/resume symptom remains on current main before selecting a production repair.
The investigation is complete, with native-process evidence recorded in [investigation.md](investigation.md).

## Completed scope

- Built isolated native backend, agentctl, and mock-agent binaries at v0.98.0 and baseline `5d6adcbca18`.
- Used disposable data, a local worktree repository, and normal profile resolution.
  The current backend used its production policy reader and package binding path.
- Restarted the actual backend while preserving the task session and saved native conversation.
- Exercised message resume before/after workspace access, workflow reuse, pending title ownership, and a current retained-process control.
- Captured execution identity, title ownership, persisted binding, actual initialized `tools/list`, and a real provider-side MCP call.
- Compared the failure boundary with the path added by PR #4762.
- Posted and verified the targeted information request under the user's explicit authorization.
- Stopped both owned instances and checked their port ranges and disposable process homes.

Temporary Node drivers outside the repository replaced the proposed temporary Go composition test.
They exercised actual process restart without adding permanent test files.
Outgoing generated prompt text and real Claude ToolSearch were not asserted in the native-process runs.
Task 02 implements the guidance assertion.

## Acceptance and outcome

1. The historical helper-first restart flow loses all seven canvas tools and the eligible title tool.
   Message resume and workflow reuse preserve the same native conversation despite the missing tools.
2. Direct resume before workspace access succeeds on v0.98.0.
   Current main succeeds in both orderings and preserves the binding during retained-process adoption.
3. The observed boundary is full-profile delivery during workspace-helper promotion.
   Task 01 is cancelled; Task 02 is scoped to the existing package binding path.

These diagnostic results do not mark every linked acceptance criterion permanently covered.
Real Claude discovery remains an explicit limit; Task 02 covers generated-guidance agreement and tool delivery composition.

## Verification

The following component command passed in all three packages:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/backendapp ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run '^(TestBuiltinMCPManagementRoutesAreWired|TestBuiltinFreshResumeAndModelSwitchResolveCurrentPolicy|TestExistingWorkspacePreparesBuiltinPackagesBeforeStart|TestBuiltinPromotionBindsWorkspaceBeforeCommand|TestBuiltinPromotionVerifiesRetainedBindingWithoutReplacingIt|TestStartupMCPReadyBeforeRecoveryAndLaunch)$' -count=1)
```

Actual process commands, matrix results, version pins, identities, artifacts, and teardown are recorded in [the investigation](investigation.md).
Historical missing-tool cases returned the expected diagnostic exit code 2.
Current counterparts and the historical direct-resume control returned exit code 0.

## Files touched

- This work order and its plan, investigation, and paired internal design documents
- Temporary scripts and runtime data under `/tmp`; no repository source or permanent test files

## Dependencies

None. Investigation preceded selection of the regression scope.

## Risks

- Mock ACP does not certify real Claude discovery or the effect of `/model`.
- No historical reporter logs were available for comparison with the reproduced sequence.
- A manifest-free component fixture alone does not establish a current production defect.

## Parallelism

`sequential`

## Inputs

- [Issue #5044](https://github.com/kdlbs/kandev/issues/5044).
- [Investigation decision](plan.md#investigation-decision).
- [Requirements](../../specs/agents/requirements/mcp-profile-continuity.md).
- [System design](../../specs/agents/system-design/mcp-profile-continuity.md).
- Production `configureBuiltinMCPServices`, `applyResumeMCPSettings`, and `prepareBuiltinWorkspaceExecution`.

## Results

Done. The current package binding path corrects the reproduced historical failure.
The user-authorized [reporter question](https://github.com/kdlbs/kandev/issues/5044#issuecomment-6637018148) is posted.
Permanent regression coverage is delivered by Task 02; real Claude evidence remains pending.
