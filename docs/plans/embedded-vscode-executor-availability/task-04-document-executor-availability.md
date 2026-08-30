---
id: "04-document-executor-availability"
title: "Document executor-specific availability"
status: done
wave: 3
depends_on: ["01-add-session-capability", "02-wire-editor-availability"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-executor-availability.md"
---

# Task 04: Document Executor-Specific Availability

## Acceptance

- Public editor guidance distinguishes native Windows Local/Worktree sessions from supported Linux
  Docker and remote task environments on a Windows Kandev host.
- The feature-status boundary and executor documentation agree with the approved spec and retain
  the code-server network-isolation/startup caveats.
- Public-doc validation passes, and the spec/plan/task statuses accurately reflect the completed
  implementation and recorded verification.

## Verification

From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Cross-check terminology and stale host-wide claims:

```bash
rg -n "Embedded VS Code|code-server|backend host is Windows|backend runs on Windows" docs/public docs/specs
```

## Files likely touched

- `docs/public/developer-tools.md`
- `docs/public/feature-status.md`
- `docs/public/executors.md`
- `docs/specs/ui/requirements/embedded-vscode-executor-availability.md`
- `docs/specs/INDEX.md`
- `docs/plans/embedded-vscode-executor-availability/plan.md`
- `docs/plans/embedded-vscode-executor-availability/task-*.md`

## Dependencies

- Tasks 01 and 02 establish the final API and user-visible behavior.

## Parallelism

Parallel-safe with Task 03 only when the user explicitly authorizes subagents. Otherwise run
sequentially in the primary conversation.

## Inputs

- Spec sections: **What**, **Failure modes**, and **Out of scope**.
- Existing public guidance:
  - `docs/public/developer-tools.md#files-and-editor-integrations`
  - **Embedded VS Code** row in `docs/public/feature-status.md`
  - executor boundaries in `docs/public/executors.md`

## Risks

- Do not claim that capability guarantees a successful download or startup.
- Do not imply native Windows code-server support.
- Keep external host-editor discovery separate from the embedded task-runtime editor.

## Output contract

Report public pages changed and validator results. After all implementation and E2E checks pass,
mark this task and every plan checkbox `done`, set the plan to `complete`, and set the spec to
`shipped`.

## Completion record

- Updated `developer-tools.md`, `feature-status.md`, and `executors.md` to distinguish native
  Windows Local/Worktree from Linux-backed Docker, Sprites, and supported SSH execution, while
  retaining the code-server isolation and startup caveats.
- `node --test scripts/validate-public-docs.test.mjs` passed (58 tests) and
  `node scripts/validate-public-docs.mjs` validated 41 published pages.
