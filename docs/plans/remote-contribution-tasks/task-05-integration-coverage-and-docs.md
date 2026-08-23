---
id: "05-integration-coverage-and-docs"
title: "Integration coverage and docs"
status: completed
wave: 5
depends_on: ["04-credential-and-push-routing"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 05: Integration Coverage and Public Docs

## Acceptance

- Focused integration coverage proves GitHub and GitLab URL creation, exact source checkout, existing
  change association, commit/push to the contributor branch, target branch immutability, restart-safe
  reconstruction, and unchanged ordinary repository behavior.
- The external MCP catalog test proves no new create-task input properties and provider remote title/body
  never appears in trusted prompt/context.
- Public reference and coordination docs explain supported URL forms, collaboration and credential
  prerequisites, outcome, and recovery; public-doc validation plus full backend test/lint pass.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- focused MCP/backend integration tests under `apps/backend/internal/mcp/` and `internal/backendapp/`
- focused temporary-Git integration tests under `apps/backend/internal/worktree/` or
  `internal/agentctl/server/process/`
- trusted-context/sysprompt regression tests where contribution guidance is projected
- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`

## Dependencies

Tasks 01–04 complete behavior and test seams.

## Parallelism

Final sequential audit. It validates the assembled cross-package flow and documents shipped behavior.

## Inputs

- Entire feature spec and ADR.
- Docs-maintainer guidance: `automation-and-mcp.md` remains reference; `coordination.md` remains how-to.
- Existing fake GitHub/GitLab provider clients, temporary Git remote helpers, MCP catalog tests, and
  public-doc validators.

## Risks

- Integration fixtures must not require network access or real provider credentials.
- Assert remote target refs directly; a successful command alone does not prove the correct fork/branch.
- Keep required collaboration/security limitations visible rather than hiding them in optional details.

## Output contract

Report scenarios covered, public docs updated and their content types, all exact command results,
remaining risks/blockers, divergence, and final task/plan status updates.

## Completion

Added hermetic provider-resolution tests for GitHub and GitLab, temporary-Git coverage for exact source
checkout and source-branch push/preflight routing using both provider binding shapes, schema regression
coverage, ordinary repository regression coverage, and public guidance in the MCP reference and task
coordination how-to.

Validation completed successfully:

- `make -C apps/backend test` — passed.
- `make -C apps/backend lint` — passed with zero issues.
- `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.

The provider fixtures and temporary Git remotes are intentionally network-free; a live GitHub/GitLab API
and remote-host end-to-end test remains outside the hermetic backend suite.
