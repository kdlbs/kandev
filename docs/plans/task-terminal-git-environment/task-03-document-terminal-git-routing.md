---
id: "03-document-terminal-git-routing"
title: "Document terminal Git routing"
status: done
wave: 3
depends_on: ["01-runtime-terminal-environment", "02-agentctl-shell-process-environment"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Document and Verify Terminal Git Routing

## Acceptance

- Public executor and terminal documentation state that a newly opened terminal in a managed task
  uses the task's managed Git/`gh` routing, while executor-inheritance mode does not.
- Documentation states that a live terminal keeps its initial environment and must be reopened
  after a new launch/resume or policy change.
- Public-doc validation passes without exposing broker variables, helper paths, or credentials.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/executors.md`
- `docs/public/developer-tools.md`
- `docs/specs/integrations/requirements/github-authentication.md`
- `docs/plans/task-terminal-git-environment/plan.md`

## Dependencies

Tasks 01 and 02 must define and verify the final observable behavior.

## Parallelism

Sequential.

## Inputs

- Spec: `docs/specs/integrations/requirements/github-authentication.md` — What and Scenarios.
- Plan: Public documentation section.
- Existing docs: `docs/public/executors.md` and `docs/public/developer-tools.md`.

## Output contract

Report documentation changes and validation output. Public executor and developer-tool guidance
now covers managed terminal routing and reopening live PTYs; both public-doc validation commands
pass.
