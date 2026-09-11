---
created: 2026-09-10
status: implemented
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
legacy_specs: []
---

# Implementation plan: Remote clone branch correction

## Overview

One sequential work order corrects the shared repository branch placeholder and adds regression coverage.
The executor system owns this contract because it converts repository selection into executable preparation scripts.

## Evidence

Task `37e3d37c-12c1-42dd-b186-3362b0a47722` was created on 2026-09-10 at 20:49:41 UTC.
Session `f1dfa237-0ea8-4dd6-bd01-b76e6d20f4c3` failed at 20:49:56 UTC on backend commit `796bf5853`.
The prepare script requested `origin/main`; Git returned exit 128 and `Remote branch origin/main not found in upstream origin`.
The diagnostic archive was partial but contained these launch events.

Before the correction, `RepositoryProvider` quoted the base reference without conversion.
Current Sprites defaults use fetch, while the observed script used clone.
The smallest regression uses the actual provider with `base_branch: origin/main` and a temporary upstream repository with branch `main`.

## Scope

- Normalize the known origin reference forms once at the shared placeholder boundary.
- Preserve metadata, worktree references, nested branch names, and shell escaping.
- Cover both current and saved preparation templates.

UI changes, database changes, remote discovery, live relaunch, deployment, and unrelated launch recovery are excluded.

## Technical approach

Add a private branch conversion helper in `apps/backend/internal/scriptengine/providers.go` or a small sibling file.
Call it after metadata precedence resolution and before `shellQuote`.
Use one-prefix conversion from the [design](../../specs/executors/system-design/repository-branch-resolution.md).
Do not reuse the worktree helper: its repeated prefix removal has a different behavior.

## Tests

- `TestRepositoryProvider_NormalizesOriginBranch`: all conversion and precedence cases (AC-001.1, .2, .3, .4).
- `TestSpritesPrepareScript_OriginBranch`: current fetch and saved clone scripts with a real temporary Git remote (AC-001.1, .3, .4).
- `TestSpritesPrepareScript_MissingBranch`: failed preparation without default-branch fallback (AC-001.5).
- Existing `TestProviders_ShellEscapeDataPlaceholders` and `TestBranchNameCommandInjection_Regression` preserve literal shell data (AC-001.4).

All abbreviated AC references above belong to `AC-EXECUTORS-REPOSITORY-BRANCH-001`.

## End-to-end evidence

The real-Git preparation test covers metadata, script resolution, shell execution, and the resulting checkout.
No rendered UI changes require a browser test. External Sprites provisioning is outside this correction.

## Public documentation

The Git operations reference now explains branch formats in executor scripts.
The addition documents the one-prefix rule and preserved worktree base reference.

## Work orders

- [x] [Task 01: Normalize repository branch placeholders](task-01-normalize-repository-branch.md)

## Verification results

Implementation and checks completed on 2026-09-10:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- `go test ./internal/scriptengine -count=1`: passed.
- The work order's exact lifecycle selection: passed, including both real-Git script forms and command-injection regressions.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- The executor index is 3,115 bytes, below its size limit.

The [work order](task-01-normalize-repository-branch.md#results) records RED and GREEN evidence.
No deployment, live task mutation, commit, push, or PR operation occurred.

## Risks

- The shorthand `origin/topic` denotes an origin reference. `refs/heads/origin/topic` preserves a literal upstream branch with that name.
- Saved custom scripts that consume `repository.branch` receive the corrected branch name.
- The running backend needs the completed fix deployed before the failed task can benefit from it.
