---
id: "08-public-runtime-credentials-docs"
title: "Document Office runtime credentials"
status: done
wave: 5
depends_on: ["06-office-launch-context-guard", "07-cli-context-diagnostic"]
plan: "plan.md"
spec: "../../specs/office/requirements/agents.md"
---

# Task 08: Document Office runtime credentials

## Acceptance

- Public docs define `KANDEV_API_URL`, `KANDEV_API_KEY`, and `KANDEV_CLI` as
  automatically injected, run-scoped Office values rather than user
  configuration.
- Troubleshooting distinguishes a scheduler-launched Office run from a regular
  task-mode session and gives the supported action for each.
- Documentation does not expose token-generation instructions or imply that a
  long-lived Office API key exists.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/automation-and-mcp.md`

## Inputs

- Completed Tasks 06 and 07.
- `docs/specs/office/requirements/agents.md`, section **Environment variables**.
- Follow `.agents/skills/docs-maintainer/SKILL.md`.

## Output Contract

Act as docs-maintainer. Report the public section changed, validation results,
links checked, unresolved documentation gaps, and task status. Update this task
to `done` only after validation passes.

## Completion Evidence

- Added **Runtime credentials** to `docs/public/automation-and-mcp.md`.
- Documented that the API URL, signed key, and CLI path are automatic,
  run-scoped Office values; regular sessions use their injected MCP tools.
- Added missing-context troubleshooting without exposing token-generation or
  persistence instructions.
- `node --test scripts/validate-public-docs.test.mjs` passed (58 tests).
- `node scripts/validate-public-docs.mjs` passed (41 published pages).
