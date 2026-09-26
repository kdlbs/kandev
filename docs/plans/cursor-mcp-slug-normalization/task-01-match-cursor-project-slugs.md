---
id: "01-match-cursor-project-slugs"
title: "Match Cursor project slugs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-001.1
  - AC-AGENTS-CURSOR-AUTH-001.4
  - AC-AGENTS-CURSOR-AUTH-003.1
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
---

# Task 01: Match Cursor project slugs

## Summary

Make Kandev's Cursor project directory name match the supplied Cursor Agent CLI rule. Prove a task worktree with a `.kandev` path receives its auth link in the directory Cursor opens.

## In scope

- Replace the slug helper with ASCII-alphanumeric preservation, dash-run collapse, and boundary trimming.
- Update slug table cases and the worktree link assertion, including Unix, Windows drive, Windows network, punctuation, empty, and non-ASCII paths.
- Keep task-root exclusion effective for current and pre-normalization project slugs.

## Out of scope

- Changing credential contents, profile or lifecycle policy, or filesystem cleanup beyond the helper's current callers.

## Acceptance

1. `TestDeriveCursorProjectSlug` fails against the old helper for the task dot-folder path and other dash-run cases, then passes with Cursor's expected single-dash names.
2. `TestDeriveCursorProjectSlug` supplies literal expected slugs for fixed paths; `TestCursorMCPAuthWorktree` derives only its randomized temporary root prefix, keeps the expected task-path suffix literal, and finds the symlink there.
3. Existing bridge and lifecycle tests pass; no regular auth file or source-exclusion safety behavior regresses.
4. A regular auth file under a task project directory created with the previous slug rule is excluded from the shared snapshot.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -race -v ./internal/agent/mcpconfig/...)
(cd apps/backend && go test -race -v ./internal/agent/runtime/lifecycle/...)
make -C apps/backend lint test
make -C apps/backend build
```

The build command covers the request's separate build check. Record any unrelated broad-suite failure with its failing test and isolate it before reporting the result.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge.go`
- `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge_test.go`

## Dependencies

None.

## Risks

- The directory name is an external Cursor contract; the supplied bundled rule is the compatibility target.
- Existing double-dash directories are left in place. Cursor will use the newly named directory on the next eligible launch.

## Parallelism

`sequential`

## Inputs

- `AC-AGENTS-CURSOR-AUTH-001.1`, `.4`, and `AC-AGENTS-CURSOR-AUTH-003.1` in the Cursor bridge requirements.
- The helper boundary, task-root exclusion, and file safety sections of the Cursor bridge system design.
- The existing `TestDeriveCursorProjectSlug`, `TestCursorMCPAuthWorktree`, and task-root exclusion tests.

## Results

Implemented Cursor's ASCII-alphanumeric slug rule with collapsed separators and boundary trimming. Added fixed slug cases for the supplied `.kandev/tasks` path, Windows drive and network paths, punctuation runs, empty output, and non-ASCII characters. The worktree assertion keeps the task-path suffix literal. PR fixup also adds legacy task-root slug exclusion and removes the duplicated current-slug normalizer from tests.

The slug regression and the added legacy-root regression failed against their respective prior implementations and passed after the fixes. The full `mcpconfig` race suite passed after fixup; the lifecycle race suite had passed during the implementation run. Backend lint passed with 0 issues, `make -C apps/backend build` passed, and specification catalog validation, spec lint, and `git diff --check` passed. The earlier repository-wide backend test run had four isolated failures outside this work order: two real-process-tree probe assertions and two launcher config-discovery tests selecting `/root/.kandev/config.yaml` instead of their temporary configs.
