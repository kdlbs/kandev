---
created: 2026-09-25
status: done
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
legacy_specs: []
---

# Implementation Plan: Workflow entry saved-prompt delivery

## Overview

Preserve saved-prompt definitions when workflow entry starts or resets a session.
One sequential work order carries trusted context through composition, recording,
and launch. It then verifies the profile-switch round trip and recovery paths.

Issue: [#3922](https://github.com/kdlbs/kandev/issues/3922).
The task system owns prompt delivery. Existing criteria cover persistence,
trust, lookup failure, and passthrough behavior. Added criteria 001.12 and
001.13 make workflow-entry and recovery outcomes explicit.

## Evidence and root cause

Source inspected at `3aa3233c78`:

1. `task_operations.go:buildWorkflowEntryPrompt` calls `buildWorkflowPrompt`,
   which discards the trusted-context return from `buildWorkflowPromptWithContext`.
2. `event_handlers_workflow.go:autoStartStepPrompt` injects runtime context for
   CREATED sessions and context resets. Its trusted arguments omit saved-prompt context.
   Canonicalization therefore removes the generated expansion before recording.
3. `task_operations.go:startCreatedSessionWithComposedPrompt` passes an empty
   context to `startCreatedSession`. That method skips composition but still
   calls `wrapCreatedSessionPrompt`, creating another removal boundary.
4. Running-session dispatch usually bypasses these injectors. This explains
   why the same workflow prompt works without a session switch.

A temporary `TestReproIssue3922` used the real SQLite prompt service and
`buildWorkflowEntryPrompt` with `Follow @demo-rule`. The expansion existed before
canonicalization and disappeared afterward. A control passed the resolver's exact
trusted value and retained the saved definition once. The diagnostic passed:

```bash
(cd apps/backend && go test ./internal/orchestrator -run '^TestReproIssue3922$' -count=1 -v)
```

This proves the composition/canonicalization defect, not the complete profile
routing flow or a working fix. The temporary file was removed. Permanent
integration regressions remain part of implementation.

## Scope

### In scope

- Preserve generated context on explicit auto-start and implicit profile-switch launches.
- Cover P1 to P2 and P2 to P1, existing-session controls, and context resets.
- Preserve context through composed launches and missing-execution recovery.
- Compare persisted and dispatched expansions, including Office context injection.
- Preserve existing handoff, entity-reference, mode, and queue behavior.
- Update the workflow saved-prompt public guidance with the implemented guarantee.

### Out of scope

- Literal system-tag truncation or prompt-editor validation from the issue's secondary report.
- New UI warnings, fail-closed resolution, permissions, or prompt matching rules.
- Profile routing changes, queue schema changes, and queued-message edit/merge semantics.
- Passthrough expansion, frontend changes, release flags, or database migrations.

## Technical approach

Change `buildWorkflowEntryPrompt` to return prompt, trusted expansion context,
and error. Use `buildWorkflowPromptWithContext` after the existing fallback claim.
Update every production caller and affected test caller.

Carry the context explicitly through `autoStartStepPrompt`, both runtime-context
injectors, and `startCreatedSessionWithComposedPrompt`. Pass it into the existing
`startCreatedSession` context argument. Do not recover trust from tagged text.
Use a small private input/options structure if needed to avoid excessive positional arguments.

Audit `StartSessionForWorkflowStep`, `promptTaskOptions`, and
`fallbackFreshLaunchOnMissingExecution`. Preserve the matching context across
already-composed retries and final launch wrapping. Replacement branches that
rebuild the prompt must use their newly returned context.

Keep queued replay on its existing backend-authoritative preparation path.
Add a queue/recovery regression to detect stripping or duplicate expansions.
Do not add persisted trust metadata or expand queue semantics for this repair.
Keep mode transforms single-pass and completion handoff text last.

The existing [server-owned expansion ADR](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)
remains authoritative. No new authority boundary or ADR is needed.
The completed Quick Chat and launch-fallback packages retain their completed scope
and recorded results; this package owns the additional workflow regressions.

## Tests

New tests belong in
`apps/backend/internal/orchestrator/workflow_entry_saved_prompt_test.go`.
Use the real prompt service fixture from `prompt_launch_fallback_test.go`.
Do not append new cases to oversized existing test files.

All criterion suffixes below refer to `AC-TASKS-SAVED-PROMPT-DELIVERY-001`.

| Criteria | Planned regression |
| --- | --- |
| .6, .12 | `TestWorkflowEntrySavedPrompt_ProfileSwitchRoundTrip`: drive P1/P2/P1 through `processOnEnter` with no `auto_start_agent`; capture stored messages and asynchronous implicit-launch requests. |
| .6, .12 | `TestWorkflowEntrySavedPrompt_DispatchModes`: CREATED, reused WAITING_FOR_INPUT, ordinary reset, and Office reset-context sessions; plan mode. The Office case asserts the Office tool context and saved expansion in both prompt boundaries. |
| .6, .12, .13 | `TestProcessOnEnterImplicitProfileSwitchTerminalizedGuard`: terminalize the reused profile session at the deterministic pre-dispatch barrier, then verify the production replacement launch's session identity, stored and dispatched saved definition, and completion handoff. `TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff` remains a same-session shared-claim control. |
| .6, .13 | `TestWorkflowEntrySavedPrompt_Recovery` and `TestStartSessionForWorkflowStep_ComposedHandoffSurvivesLazyResumeFallback`: missing execution and composed workflow-step recovery. |
| .4, .5, .8 | `TestWorkflowEntrySavedPrompt_TrustGuards`: forged blocks, unknown references, lookup failure, absent expander, passthrough, and empty prompt. |
| .12, .13 | `TestWorkflowEntrySavedPrompt_Composition`: nested references and workflow-level references. `TestWorkflowEntrySavedPrompt_DispatchModes` covers plan mode; the recovery test covers completion handoff ordering. |
| .13 | `TestSendQueuedNowConsumesCeilingLaunchAndPreservesWorkflowPrompt`: queued Send Now preserves the accepted expansion in the deferred launch. |

Run the CREATED-session regression before production edits. It must fail because
the saved definition disappears, not because the fixture fails to launch.
Assert visible references remain and each canonical prompt contains exactly one
expansion block. Compare the saved definitions in persistence and dispatch.
For prepared retries, mutate the saved record between preparation and wrapping
and assert that the carried definition is not silently replaced.

## End-to-end evidence

Backend service integration tests exercise routing, composition, message storage,
and the agent-manager dispatch boundary. Mock external agent execution only.
Use existing profile-switch and handoff fixtures with deterministic synchronization.
No browser control or rendered UI changes, so no new Playwright test or UI preview is required.

## Work orders

- [x] [Task 01: Preserve workflow expansion context](task-01-preserve-expansion-context.md) (done)

## Verification results

Diagnostic reproduction passed on 2026-09-25 and was removed. The implementation
and its verification completed on 2026-09-25:

- The focused orchestrator race suite passed, including workflow-entry
  dispatch, profile routing, composed launch, and fresh-runtime recovery tests.
- Review follow-up race tests passed for the Office reset injector, the
  no-`auto_start_agent` P1/P2/P1 asynchronous launches, and terminalized-session
  replacement with the carried handoff.
- The prompt service and system-prompt race tests passed.
- Additional recovery, ceiling-deferral, and queued Send Now preservation tests
  passed under the race detector.
- `go build ./...` passed from `apps/backend`.
- `python3 scripts/list-docs.py validate` validated 305 decisions and 1151
  specifications; `python3 scripts/lint-spec-files.py --all` passed.
- The public-doc validator tests passed (62 tests), and the validator accepted
  all 47 published pages.
- `gofmt` and `git diff --check` passed.

The public workflow guidance now documents the preserved-context behavior. The
issue remains assigned to `carlosflorencio`. Implementation, tests, docs, and
plan files remain unstaged and uncommitted.

## Risks

- Fixing only the first injector still loses the expansion at the second injector.
- Re-reading mutable definitions after recording can make persistence differ from dispatch.
- Trust inferred from prompt text would bypass the existing security boundary.
- Signature changes touch retry and replacement paths with cancellation and admission guards.
- Broad recomposition can duplicate plan-mode context, references, or handoff text.
