---
id: "03-office-capability-context"
title: "Make Office capability instructions truthful"
status: done
wave: 2
depends_on: ["01-project-runtime-capability", "02-project-cli"]
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 03: Make Office capability instructions truthful

## Acceptance

- Office first turns advertise exactly the tools registered by `ModeOffice` and direct mutations to `$KANDEV_CLI`.
- Office instructions never advertise `step_complete_kandev`, workspace creation, or Kanban/config MCP tools.
- CEO agents receive a project-management skill containing verified project and task-project commands.

## Verification

```bash
cd apps/backend && rtk go test ./internal/mcp/server ./internal/sysprompt ./internal/orchestrator ./internal/office/skills
```

## Files Likely Touched

- `apps/backend/config/prompts/office-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/internal/office/configloader/skills/kandev-projects/SKILL.md`
- `apps/backend/internal/office/skills/system_sync_test.go`

## Inputs

- Exact ModeOffice set in `internal/mcp/server/server_test.go`.
- CLI syntax delivered by Task 02.
- Keep `step_complete_kandev` exclusively in the task-mode context.

## Output Contract

Report prompt inventory, skill assignment behavior, files changed, tests, blockers, risks, and task status. Use TDD and do not edit `plan.md`.

## Implementation Evidence

- Added a dedicated Office first-turn context with exactly the nine `ModeOffice`
  tools: four plan tools, `ask_user_question_kandev`,
  `list_related_tasks_kandev`, and the three task-document tools.
- Routed direct launches, created-session WebSocket starts, and workflow
  auto-starts through the Office context while preserving task/config context
  selection.
- Added the `kandev-projects` bundled system skill with
  `default_for_roles: [ceo]`. Existing explicit skill assignment remains
  available for other roles; they do not receive it by role default.
- Verified project commands against the delivered CLI:
  `projects list`, `projects create --name ... --repository ...`, and
  `task create --project ...`.

### Tests

- Red tests reproduced generic task-context injection in all three Office
  first-turn paths and the missing bundled project skill.
- Focused prompt, MCP inventory, WebSocket, orchestrator, and skill tests pass.
- `go test ./internal/orchestrator ./internal/office/skills ./internal/sysprompt`
  passes (the combined command continued after the MCP package failed).
- Full `internal/mcp/server` and `internal/task/handlers` package runs are
  blocked in this sandbox by existing `httptest` cases that cannot bind a local
  TCP socket (`operation not permitted`). Non-socket suites reached 62 passing
  MCP tests and 46 passing handler tests; all focused changed-path tests pass.

### Review Correction

- Added an agent-service creation regression that proves a default CEO receives
  `kandev-projects`, every non-CEO role does not, and a caller-provided skill
  set remains unchanged instead of being augmented by role defaults.
- Added `kandev-projects` to the spec's durable CEO system-skill inventory.
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/agents -run 'TestCreateAgentInstance_(CEOReceivesProjectSkillByDefault|NonCEORolesDoNotReceiveProjectSkillByDefault|PreservesExplicitSystemSkills)' -count=1`
  passes all nine subtests.

### Prompt Mode Correction

- Root cause: first-turn paths treated `HasKandevContext` as a compatibility
  check, so a prompt carrying task-mode context was left unchanged when the
  same task launched as Office.
- Added explicit task, Office, and none context classification. Mode-aware
  injection now replaces only incompatible Kandev context blocks, preserves
  unrelated system blocks and user content, and remains idempotent for an
  already-compatible context.
- Covered persisted WebSocket messages, direct created-session launches, and
  workflow auto-starts from task context into Office context. Office output
  continues to omit `step_complete_kandev` and task-mode tools.
- Focused regression tests pass; the full sysprompt package passes 48 tests and
  the full orchestrator package passes 972 tests. The handler package reaches
  46 passes before the existing sandbox socket restriction in
  `TestStopProcessRejectsDifferentSession`.

### Canonicalization Security Correction

- Root cause: mode compatibility was inferred only from the marker inside an
  existing `<kandev-system>` block. A same-mode block with stale or spoofed
  task/session IDs, completion gating, or coordinator controls was therefore
  retained by direct launch and workflow auto-start paths.
- Both task and Office injectors now rebuild the runtime block from current
  server state on every first-turn launch/recording pass. The first runtime
  block is replaced, additional task/Office runtime blocks are removed, and
  unrelated system blocks plus visible user content are preserved.
- Removed the separate public `ReplaceOfficeContext` and
  `ReplaceKandevContextWithOptions` helpers; all call sites use the same
  canonicalizing injectors. Empty-prompt and passthrough guards remain at the
  launch/recording boundaries.
- Added same-mode stale-ID and duplicate-block regressions for the shared
  Office helper, Office workflow auto-start, and Office WebSocket recording.
  The focused canonicalization suite passed all 16 tests across `sysprompt`,
  `orchestrator`, and `task/handlers`.
- The broad owned-package run reached 1,064 passing tests. Its only failure was
  the unrelated `TestStopProcessRejectsDifferentSession`, where this sandbox
  rejects `httptest` loopback binding with `listen tcp6 [::1]:0: socket:
  operation not permitted`.

### Canonical Office Ownership Correction

- Root cause: launch, resume, first-message, and runtime-state paths inferred
  Office ownership from `AssigneeAgentProfileID` or a session profile. Those
  fields select an agent but do not identify the task's owning surface.
  Unassigned project/Office-workflow tasks consequently received task-mode
  context and MCP access, while assigned Kanban tasks could receive Office
  behavior.
- All Office context, `McpModeOffice`, error handling, turn completion, and
  runtime-state decisions now use the repository's canonical
  `Task.IsFromOffice` projection. Per-agent session reuse still additionally
  requires an assignee because that field is needed to select the session.
- Added paired regressions for unassigned Office and assigned non-Office tasks
  across created-session launch, prepared workspaces, workflow auto-start,
  WebSocket first-message persistence, resume/model switch, runtime state,
  cancel/turn completion, failure handling, and advanced-mode session lookup.
- Focused `-race` suites pass in `internal/orchestrator`,
  `internal/orchestrator/executor`, and `internal/task/handlers`. The full
  orchestrator suite passes in 30.226s and the full executor suite passes. The
  broad handler suite remains limited only by the existing sandbox loopback
  restriction in `TestStopProcessRejectsDifferentSession`.
