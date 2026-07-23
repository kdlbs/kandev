---
id: "05-public-docs"
title: "Document Office agent project commands"
status: done
wave: 3
depends_on: ["02-project-cli", "03-office-capability-context"]
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 05: Document Office agent project commands

## Acceptance

- Public docs distinguish the restricted Office MCP surface from the Office runtime CLI.
- Project list/create syntax, `task create --project`, permissions, current-workspace scoping, and the absence of agent workspace creation are documented.
- Public-doc validation passes without adding an unnecessary new navigation page.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md` only if a cross-link is needed

## Inputs

- Delivered CLI syntax from Task 02.
- Exact Office prompt/tool inventory from Task 03.
- Follow `.agents/skills/docs-maintainer/SKILL.md`.

## Output Contract

Act as docs-maintainer. Report public docs changed, validation commands/results, unresolved documentation gaps, blockers, risks, and task status. Do not edit `plan.md`.

## Completion Evidence

- Updated `docs/public/automation-and-mcp.md` to distinguish the exact nine-tool
  Office MCP surface from permission-checked Office runtime CLI mutations.
- Documented `projects list`, `projects create` with repeatable `--repository`,
  optional project flags, and `task create --project` using returned project IDs.
- Documented `can_create_projects` defaults, validated-run current-workspace
  scoping, and the prohibition on Office-run workspace creation or administration.
- Clarified that Kanban/configuration MCP tools and `step_complete_kandev` are
  task-mode only and are not registered in Office mode.
- `node --test scripts/validate-public-docs.test.mjs` passed (1 test).
- `node scripts/validate-public-docs.mjs` passed (40 published pages).
- `git diff --check` passed.
