---
id: "02-canonical-tool-naming"
title: "Clarify canonical MCP tool names"
status: done
wave: 2
depends_on: ["01-task-only-eager-tool"]
plan: "plan.md"
spec: "../../specs/workflow/explicit-completion-signal/spec.md"
---

# Task 02: Clarify canonical MCP tool names

## Acceptance

- Task context describes `_kandev` names as canonical MCP protocol names and allows client-qualified display aliases.
- The completion instruction appears only for signal-gated Kanban steps.
- Office context remains separate and contains no completion-tool reference.

## Verification

```bash
cd apps/backend && rtk go test ./internal/sysprompt ./internal/mcp/server -run 'Test.*(KandevContext|Sysprompt|Office)' -count=1
```

## Files Likely Touched

- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md`

## Inputs

- Office capability context and public docs from `docs/plans/office-agent-project-management/task-03-office-capability-context.md` and `task-05-public-docs.md` must land first.
- Frontend parser already recognizes `mcp__kandev__` qualification; do not make that Claude-specific alias the canonical MCP name.

## Output Contract

Report wording and public-doc changes, mode assertions, files changed, commands/results, blockers, risks, and task status. Use TDD, follow `/docs-maintainer`, and do not edit `plan.md`.

## Implementation Evidence

- Task context now defines `_kandev` names as the canonical MCP protocol
  names and explains that server-qualified registry forms are client-specific
  aliases, not separate capabilities or universal names.
- The concrete `mcp__kandev__step_complete_kandev` example lives inside the
  signal-gated completion section, so ordinary Kanban steps and Office context
  advertise neither form of the completion tool.
- Prompt-sync extraction normalizes the known qualified alias back to its
  canonical tool name before comparing references with the task-mode MCP
  inventory. The Office inventory assertion continues to require an exact
  match and explicitly rejects both completion forms.
- Public docs explain canonical and qualified names in
  `automation-and-mcp.md` and `agent-communication.md`, including that the
  qualified prefix varies by client.

### TDD and Verification

- Red: the new canonical/qualified context test failed because the original
  task prompt contained neither canonical-name guidance nor a qualified alias.
- Green: `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/sysprompt
  ./internal/mcp/server -run 'Test.*(KandevContext|Sysprompt|Office)'
  -count=1` passed (22 tests).
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/mcp/server -run
  'TestExtractKandevTools_NormalizesClientQualifiedAliases' -count=1` passed
  (1 test).
- `node --test scripts/validate-public-docs.test.mjs` passed (1 test).
- `node scripts/validate-public-docs.mjs` passed (40 published pages).
- `git diff --check` passed.

Residual risk: clients may choose a qualifier other than `mcp__kandev__`.
The prompt and docs intentionally present that form only as an example and
direct agents to use the active client's exposed registry name.

### Prompt Mode Correction

- A prompt carrying Office context is now replaced with task context when it
  launches in task mode. The shared replacement regression preserves unrelated
  system blocks and user content, while the launch regression proves a
  signal-gated task receives the conditional `step_complete_kandev` wording.
- Compatible task context remains byte-for-byte idempotent and no launch path
  creates duplicate Kandev context blocks.
- Focused sysprompt, orchestrator, and message-handler regressions pass; full
  sysprompt and orchestrator package results are 48 and 972 passing tests.

### Canonicalization Security Correction

- A same-mode task block is no longer trusted solely because it contains the
  task-context marker. Current task/session IDs, completion-signal gating, and
  coordinator/config controls are regenerated from server state before launch
  or first-message persistence.
- Duplicate and incompatible task/Office runtime blocks are removed while
  unrelated system blocks and user content remain intact. This covers direct
  created-session launch, workflow auto-start, and WebSocket message recording
  without changing the empty-prompt or passthrough guards.
- Added task-mode regressions for stale IDs and stale capability sections in
  the shared helper, direct launch, and message recording. Together with the
  Office cases, all 16 focused canonicalization tests pass.
- The broad owned-package run reached 1,064 passing tests before the unrelated
  sandbox-only failure in `TestStopProcessRejectsDifferentSession`
  (`listen tcp6 [::1]:0: socket: operation not permitted`).

### Canonical Mode Ownership Correction

- Mode selection no longer treats the presence of an assigned agent profile as
  proof of Office ownership. `Task.IsFromOffice` is now the source of truth for
  task versus Office context and MCP mode across new launches, prepared
  workspaces, resumes, model switches, and first-message storage.
- This keeps `step_complete_kandev` available only to genuine Kanban task-mode
  sessions: an assigned Kanban task remains in task mode, while an unassigned
  project or Office-workflow task receives `McpModeOffice` and never advertises
  the completion tool.
- Paired ownership regressions pass with `-race` in the orchestrator, executor,
  and task-handler packages. Full orchestrator and executor package runs pass;
  the full handler package remains blocked only by the sandbox's existing
  `httptest` loopback restriction.
