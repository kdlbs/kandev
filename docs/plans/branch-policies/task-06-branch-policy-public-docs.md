---
id: "06-branch-policy-public-docs"
title: "Document branch-policy workflows"
status: complete
wave: 4
depends_on:
  - "04-task-create-policy-selector"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-002
  - REQ-WORKSPACES-BRANCH-POLICIES-003
  - REQ-WORKSPACES-BRANCH-POLICIES-004
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-001.1
  - AC-WORKSPACES-BRANCH-POLICIES-002.3
  - AC-WORKSPACES-BRANCH-POLICIES-003.1
  - AC-WORKSPACES-BRANCH-POLICIES-003.3
  - AC-WORKSPACES-BRANCH-POLICIES-004.2
  - AC-WORKSPACES-BRANCH-POLICIES-004.4
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 06: Document branch-policy workflows

## Objective

Teach users why duplicate local-path repositories are not supported, how to
configure policies, how the Gitflow starter maps branches, and what a policy
does when a task and pull request are created.

## Documentation scope

- Update Git-operation documentation with policy fields, supported template
  placeholders, task snapshot semantics, and raw-branch fallback.
- Update task/workflow documentation with the grouped selector and local
  `Fork a new branch` transition.
- Add a concise Gitflow example covering Feature, Bugfix, Hotfix, and Release,
  including development and production branch choices.
- Explain first-release exclusions and recovery for deleted policies or missing
  bases.
- Update repository or scoped engineering guidance only if implementation
  changes make an existing architecture statement inaccurate.
- Use Simplified Technical English, active voice, short procedures, and current
  screenshots only when a stable screenshot meaningfully improves the guide.

## Implementation acceptance

- Public guidance explains the canonical-repository model and complete Gitflow
  starter mapping.
- Task guidance distinguishes policies from branches and explains snapshot and
  local fresh-branch behavior.
- All changed public pages pass the repository documentation validators.

## Exclusions

- Duplicating internal database or API design in public docs.
- Documenting first-release exclusions as available features.
- Capturing screenshots that do not add durable instructional value.

## Files likely touched

- `docs/public/git-operations.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/use-kandev.md`
- `AGENTS.md`, `apps/backend/AGENTS.md`, or `apps/web/AGENTS.md` only when needed

## Verification

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check -- docs/public AGENTS.md apps/backend/AGENTS.md apps/web/AGENTS.md`

## Dependencies and parallelism

Depends on Task 04 so public names and behavior match the implemented product.
Parallel-safe with Task 05 because the files are disjoint.

## Output contract

Report the pages updated, user journeys documented, exclusions and recovery
guidance, exact validator results, changed files, and any screenshot decision.
Mark this task and the plan checkbox complete together.

## Results

Updated `docs/public/git-operations.md` and
`docs/public/tasks-and-workflows.md` with policy configuration, Gitflow
mapping, task snapshot behavior, raw-branch fallback, pull-request target
override, local fresh-branch behavior, and first-release exclusions.

Verification:

- `node --test scripts/validate-public-docs.test.mjs` passed: 61 tests.
- `node scripts/validate-public-docs.mjs` passed: 41 published pages.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
