---
id: "07-authoring-bundle-and-scaffold"
title: "Reduce authoring skill reads"
status: in_progress
wave: 7
depends_on:
  - "06-isolated-application-appearance"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.6
  - AC-CANVASES-AGENT-WEB-APPS-001.8
  - AC-CANVASES-AGENT-WEB-APPS-001.9
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 07: Reduce authoring skill reads

## Summary

Return one complete core authoring bundle and generate the exact minimal
scaffold in the assigned source directory. Keep detailed references optional
and executor-portable.

## In scope

- Define one canonical ordered skill and scaffold inventory.
- Return the compact core bundle when the skill-read path is absent.
- Keep allowlisted detailed path reads.
- Materialize manifest, HTML, script, styles, and appearance bootstrap files.
- Return the exact scaffold inventory from `create_canvas_kandev`.
- Update create and edit prompts to avoid repeated core reads.
- Keep host paths private and preserve local, Docker, and SSH response parity.
- Add golden inventory, manifest, route, appearance, and response-shape tests.
- Record one ACP evaluation with at most one core read and no invalid path call.

## Out of scope

- Copying the canvas skill into task workspaces or Office skill discovery.
- Mounting or returning the Kandev host's absolute skill path.

## Acceptance

- The normal workflow needs no more than one core skill-read call.
- Every documented path exists in the canonical inventory.
- Agents edit the generated scaffold with native file tools on every executor.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/canvasskill/... ./internal/mcp/server/... ./internal/backendapp/... ./internal/agentctl/server/api/... -count=1
make -C apps/backend lint
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/canvasskill/**`
- `apps/backend/internal/mcp/server/canvas_tools.go`
- `apps/backend/internal/mcp/server/canvas_tools_test.go`
- `apps/backend/internal/backendapp/canvas_authoring.go`
- `apps/backend/internal/backendapp/canvas_authoring_test.go`
- `apps/backend/internal/backendapp/canvas_edit_test.go`
- `apps/backend/config/prompts/**`

## Dependencies

- Task 06 provides the appearance protocol and CSS variable contract that the
  core bundle and scaffold document.

## Risks

- A large core response can exceed practical agent context budgets.
- Separate inventory builders can make responses and materialized files drift.
- Prompt wording can still cause an agent to repeat a read unnecessarily.

## Parallelism

`sequential`

## Inputs

- Agent authoring guidance and source-transfer sections.
- The normalized ACP evidence in `CANVAS-UX-08`.

## Results

Replaced repeated authoring reads with one compact core bundle and a canonical
ordered inventory. Added optional allowlisted reference reads, a generated
manifest/HTML/script/styles/appearance scaffold, authenticated agentctl
materialization with rollback, and response-shape coverage for local, Docker,
and SSH-compatible executors. Updated the authoring prompt and browser
reference to document the generated scaffold and appearance contract. The
authoring contract is versioned as skill version 2.

The handler-level ACP-compatible regression proves that the normal path makes
one pathless core read and does not issue an invalid path call. The response
parity regression covers local, Docker, and SSH executor labels. An external
ACP provider evaluation artifact has not been captured in this worktree, so
this work order remains in progress until that evaluation is run and recorded.

Focused backend coverage for the affected packages and the new rollback,
version, parity, and handler tests passes. The external ACP evaluation remains
the only outstanding acceptance item.
