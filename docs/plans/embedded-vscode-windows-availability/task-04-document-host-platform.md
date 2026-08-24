---
id: "04-document-host-platform"
title: "Document host-platform availability"
status: done
wave: 3
depends_on: ["02-filter-task-topbar-editors"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-windows-availability.md"
---

# Task 04: Document host-platform availability

## Acceptance

- Public editor-integration guidance states that the task-detail topbar does not offer
  **VS Code (Embedded)** when the Kandev backend host is Windows, independent of visitor platform.
- The feature-status table carries the same host boundary without implying that all editor
  integrations are unavailable.
- Public-doc validation passes.

## Verification

```bash
rg -n "VS Code \\(Embedded\\)|Embedded VS Code" docs/public/developer-tools.md docs/public/feature-status.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/developer-tools.md`
- `docs/public/feature-status.md`

## Dependencies

- Task 02 defines the final observable wording.

## Parallelism

Parallel-safe with Task 03 after Task 02. The files are disjoint and no shared schema, package
configuration, generated contract, or lockfile is involved. User authorization is still required
before using subagents.

## Inputs

- Spec sections: **Why**, **What**, and **Out of scope**.
- Plan section: **Public documentation**.
- External references:
  - `https://github.com/coder/code-server/releases/tag/v4.130.0`
  - `https://github.com/coder/code-server/blob/main/docs/install.md`
- Existing documentation:
  - `docs/public/developer-tools.md`
  - `docs/public/feature-status.md`

## Risks

- Keep the wording version-independent because Kandev's pinned code-server version can change.
- Say “Kandev backend host,” not browser, visitor device, or task executor.

## Output contract

Report the public-doc wording changed, files changed, validation commands/results, blockers, and
update this task to `done` plus its checkbox/status in `plan.md`.
