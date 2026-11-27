---
id: "04-expansion-recovery"
title: "Integrate explicit workspace expansion"
status: blocked
wave: 4
depends_on: ["03-nested-materialization"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.8
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.9
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.6
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 04: Integrate explicit workspace expansion

## Summary

Make explicit expansion use the shared workspace-aware native restore policy.
Compatible sessions can resume at the parent root. Incompatible sessions require a separate, explicit continuation action.

## In scope

- All-session idle and compatibility preflight, including already-promoted environment/live-CWD divergence.
- Expanded-root batch compensation, target-root identity, and no-op rescan when roots already match.
- Explicit continuation integration with the shared recovery UI and authoritative preview revision.
- Preserve existing nested paths during expansion. New repositories become siblings without relocating older attachments.

## Out of scope

Reimplementing PR #3598, automatic replacement, restarting an active MCP caller, and broad session-recovery repair.

## Acceptance

1. All affected sessions are idle and checked before adoption. A changed or busy session rejects a stale preview without partial mutation.
2. Incompatible native resume never creates a new native session until the user explicitly selects continuation for this operation and target root.
3. Cancel/error retains native identity and prior sources. Successful expansion preserves existing worktree locations and publishes the adopted root.

## ASCII UI preview

### UI-06: Recovery, validation, and submission states

Excerpt from the [full preview](plan.md#ui-06-recovery-validation-and-submission-states):

```text
Unsupported native resume:
  This agent cannot resume its conversation in the expanded root.
  Continue with recorded messages and the task plan in a new
  agent conversation. Private agent context will not carry over.
  Existing repositories remain in place.
  [Cancel]              [Continue with history and add]
```

Desktop: inline recovery in the existing dialog. Phone: same information in the drawer scroll body with fixed actions.
Do not stack a new dialog over the source picker. Keep source rows and selected placement available after cancellation or error.
Task 05 owns the full placement surface; this task owns recovery action integration and shared state contracts.

## Tests and TDD

Add proposed `TestWorkspacePlacementExpansion` lifecycle/backendapp cases for compatible, incompatible, cancelled, busy, and rollback paths.
Cover multiple live sessions, including one incompatible or active session among otherwise eligible sessions.
Spy on native-session creation: it must remain zero before the explicit action and on stale or cancelled continuation.
Reuse the recovery coordinator's own tests rather than duplicating native configuration logic.
Add component tests for explicit continuation dispatch and retained source form state.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/task/service ./internal/orchestrator -run 'WorkspacePlacement|WorkspaceRebind|WorkspaceRestore|WorkspaceSources|Continuation' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'WorkspacePlacementExpansion' -count=1)
(cd apps/web && pnpm exec vitest run components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx)
```

Task 05 owns the full desktop/mobile expansion E2E through the completed placement surface.
Record at least one live compatible and one known-incompatible provider outcome when the environment permits it.
Never infer provider support from the mock registry flag alone.

## Files likely touched

- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`
- Shared restore coordinator and workspace-identity contracts supplied by PR #3598
- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/web/components/task/add-workspace-sources/use-submit-workspace-sources.ts`
- Shared session recovery service/actions supplied by PR #3598
- Focused source-dialog recovery component/state integration and tests

## Dependencies

Task 03 and PR #3598's workspace-aware native-restore contract.
Before implementation, resolve the current PR state/head and compare it with the recorded head in plan.md.
If that contract is unavailable, mark this work order blocked with the exact dependency. Do not implement an automatic fallback.
This dependency blocks execution of this work order, not completion of the design package.

## Inputs

Design: Expansion and recovery; Preview contract; Failure behavior.
PR #3598 requirement AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.1/.2 is an external dependency reference, not copied ownership.

## Risks

Open recovery work can change action shapes or compensation semantics. Reconcile this work order with the landed contract before changing production code.


## Parallelism

`sequential`

## Results

Blocked before implementation. Refreshed PR #3598 on 2026-09-15: it is still open, non-draft, with head `62851c51fa2d347bbc0d162634c25c541c441d21`. Its workspace-aware native restore and explicit continuation contracts are the required dependency. The current implementation keeps `expand_root` explicitly unavailable and never silently creates a replacement native session. Expansion tests and recovery UI remain pending until that contract is available.
