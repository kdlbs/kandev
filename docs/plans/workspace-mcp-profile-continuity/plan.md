---
created: 2026-11-04
status: done
requirements:
  - REQ-AGENTS-MCP-PROFILE-CONTINUITY-001
system_design:
  - ../../specs/agents/system-design/mcp-profile-continuity.md
legacy_specs: []
---

# Implementation plan: Workspace MCP profile continuity

## Overview

The original restart sequence reproduces on v0.98.0 and passes on baseline `5d6adcbca18`.
The existing package binding path supplies the complete profile before the resumed process starts.
Cancel the proposed new profile endpoint and add focused regression coverage for that existing path.
Keep implementation sequential in the primary session.

The diagnostic is complete and the permanent regression coverage is implemented.
No production changes were needed.

## Evidence and root cause

Source: [issue #5044](https://github.com/kdlbs/kandev/issues/5044).
Detailed process evidence, commands, fixture identities, and limitations are in [the investigation](investigation.md).

The failure has a deterministic ordering:

1. After backend restart, workspace access creates a basic helper before agent resume.
2. Resume resolves the complete MCP profile for the same task session and saved native conversation.
3. v0.98.0 workspace promotion forwards providers but drops that complete profile.
4. The resumed agent attaches to the basic catalog, losing all seven canvas tools and the eligible title tool.

A resume that creates its own execution before workspace access keeps the canvas tools on v0.98.0.
The helper-first message and workflow reuse cases both lose the eight optional tools for a pending title owner.
The same cases pass on current main with the complete persisted binding.
Current main also preserves the binding, conversation, and native process through retained-process adoption.

Commit `a1013e89fc4`, from [PR #4762](https://github.com/kdlbs/kandev/pull/4762), added the existing preparation path.
The source trace and process results support that path as the correction for this reproduced failure.
This was not an isolated-commit bisect and did not use real Claude ToolSearch.
The reporter's original sessions and the effect of `/model` remain unverified.
Canvas tools are built-in capabilities; this reproduction needs no plugin-registration race.

## Investigation decision

Task 00 used actual native backend restarts and normal profile resolution, with external services and ACP replaced by local mocks.
It did not force `BuiltinPackages` to nil.
Its historical failure and current positive controls select regression coverage as the next work.

- Task 00 is done: the failure boundary is workspace-helper promotion on v0.98.0.
- Task 01 is cancelled: no new `PUT /api/v1/mcp/profile` operation is needed for the tested current production flow.
- Task 02 covers the existing complete-profile package binding and its ordering before process start.

The earlier manifest-free lifecycle diagnostic showed an omission outside the normal current package-bearing start path.
It does not establish another reachable defect that requires the cancelled endpoint.
Do not revive that work order without evidence of such a path.

## Reporter follow-up

The user authorized a targeted question, which was [posted and verified](https://github.com/kdlbs/kandev/issues/5044#issuecomment-6637018148).
After the reproduction, the same comment was updated and fetched to verify its new body.
It reports the historical cause, the successful current binding path, and the mock ACP limit.
It asks the reporter to retry a build containing PR #4762.
If the failure persists, it requests Kandev/Claude ACP versions, redacted restart/resume logs with a UTC time, and clarification of `/model`.
It does not request a full database, conversation transcript, or credentials.

That evidence can identify a remaining client-side problem or confirm the same historical boundary.
Do not claim the real Claude discovery problem is closed from a mock-provider result.

## Scope

### In scope

- Regression coverage through normal profile resolution, workspace-helper promotion, and actual local MCP transport.
- A positive control for a fresh/direct start and a negative mutation control for omitted preparation.
- Pending title ownership, settled/non-owner title exclusion, and canvas enabled/disabled cases.
- Agreement between delivered optional tools and generated guidance.
- Preservation of task/session identity, saved native conversation, and retained bindings.

### Out of scope

- A new endpoint, guard, fallback, package grant, schema change, or plugin retry.
- Automatic migration or restart of live legacy processes.
- A fixed numeric catalog size across releases, packages, or provider contexts.
- Repairs to the reporter's live database or provider-owned discovery cache.
- Issue closure before the remaining reporter evidence is assessed.

## Technical approach

Exercise the production profile resolver and backend adapter with the real lifecycle promotion and agentctl binding surface.
Begin with a workspace-only helper that lacks optional task capabilities.
The existing `prepareBuiltinWorkspaceExecution` operation must install the full resolved profile before command configuration.
Use a real initialized local MCP client for `tools/list` assertions.
Do not make the fixture install its expected profile directly.

Add only the smallest composition coverage missing from the existing wiring, resolver, promotion, retained-binding, and startup tests.
Reuse those tests for their established contracts rather than duplicating their entire matrices.
No production changes are planned unless the new regression reveals a current defect.

## Tests

| Acceptance criteria | Evidence or planned coverage |
| --- | --- |
| 001.1, 001.2, 001.6 | Task 00 native-process comparison: historical failure, current pass, unchanged conversation, retained-process control |
| 001.1, 001.2 | Task 02 `TestResumeWorkspaceHelperMCPToolInventory`: production resolution, workspace-first promotion, real `tools/list`, negative mutation control |
| 001.3, 001.4 | Task 02 guidance test captures the production message-add prompt for the same four eligibility contexts, checks canvas/title sections and advertised tool names, and checks package guidance; existing resolver tests cover question policy |
| 001.5 | Existing binding acknowledgement/failure tests; extend only where composition coverage finds a gap |
| 001.6 | Existing retained-binding tests plus Task 00 same-process evidence |

All criterion suffixes belong to `AC-AGENTS-MCP-PROFILE-CONTINUITY-001`.
Use explicit `@covers` annotations in permanent regression tests.
Task 02 asserts the generated startup context for the covered eligibility contexts.

## Work orders

- [x] [Task 00: Reproduce and classify the reported restart flow](task-00-restart-reproduction.md)
- Cancelled: [Task 01: Deliver the complete workspace MCP profile](task-01-complete-profile-preparation.md)
- [x] [Task 02: Prove resumed tool and guidance parity](task-02-resume-catalog-parity.md)

## Verification results

- Native v0.98.0 helper-first message resume: expected failure, all seven canvas tools absent.
- Native v0.98.0 pending-title message/workflow reuse: expected failure, all eight optional tools absent.
- Native v0.98.0 direct-resume control: all seven canvas tools restored in the same conversation.
- Native current-main replacement cases: canvas tools and the eligible title tool preserved.
- Native current-main retained-process case: same execution, native process, conversation, and canvas tools preserved.
- The six named wiring/resume/promotion/startup checks in Task 00 passed in all three Go packages.
- `TestResumeWorkspaceHelperMCPToolInventory`: four cases passed, including real initialized `tools/list`, direct-start parity, identity, and persisted binding.
- `TestResumeWorkspaceHelperMCPGuidance`: the same four eligibility contexts plus a configuration-package case; the production message-add prompt's canvas/title sections and advertised tool names matched the promoted inventory and profile.
- Guidance mutation control: forcing canvas/title guidance on failed the non-owner, settled-title, and canvas-disabled cases; the source was restored.
- Promotion binding mutation control: omitting the binding call failed the direct-start inventory comparison; the source was restored.
- Both disposable instances stopped. Their port ranges have no listeners and no process retains either disposable home.
- `python3 scripts/list-docs.py validate`: passed, 431 decisions and 1914 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check` plus link and whitespace checks on every changed document: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` over the complete changed-path and document map: status `exempt`, no errors.

Task 00 temporary scripts, binaries, and databases remain under `/tmp` for local evidence.
The implementation adds one permanent backend composition test and no production source.
The documentation package remains unstaged and uncommitted.

## Documentation impact

This package records an internal recovery contract and its regression coverage.
It changes no public command, API, setting, or workflow.
It does not promise repair of an already running legacy process.

## Risks

- Mock ACP proves backend/runtime catalog delivery, not real Claude's discovery cache.
- Package splitting changes raw core counts; compare eligible tool names within equivalent context.
- A settled title or non-owner session must not receive the title tool.
- Direct fixture profile injection would bypass the boundary this regression needs to protect.
- If a current helper catalog is correct but Claude search fails, investigate attachment and refresh separately.
