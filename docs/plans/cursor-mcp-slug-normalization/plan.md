---
created: 2026-09-25
status: done
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-003
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
legacy_specs: []
---

# Implementation plan: Cursor MCP project slug normalization

## Overview

Kandev links shared MCP credentials into a project directory whose name can differ from the directory Cursor Agent CLI opens. The repair updates the shared slug helper and its regression tests in one work order. No profile, launch, or UI contract changes are needed.

## Root cause and reproduction

`DeriveCursorProjectSlug` replaces only four punctuation classes and preserves adjacent dashes. For `/Users/cfl12/.kandev/tasks/hello_hj2srhm6/master`, it returns `Users-cfl12--kandev-tasks-hello-hj2srhm6-master`. Cursor's supplied rule replaces every non-ASCII-alphanumeric character with a dash, collapses dash runs, and returns `Users-cfl12-kandev-tasks-hello-hj2srhm6-master`. Kandev writes `mcp-auth.json` under the first directory; Cursor opens the second. The current `TestDeriveCursorProjectSlug` passes because it expects internal dash runs.

## Scope

In scope: use Cursor's ASCII-alphanumeric slug rule for project links and current task-root exclusion, preserve exclusion for task directories created with the previous slug rule, and prove the worktree link lands at the expected directory.

Out of scope: OAuth login, credential aggregation precedence, profile settings, remote executors, migration or deletion of old project directories, and Cursor versions with a different naming rule.

## Technical approach

Update `DeriveCursorProjectSlug` in `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge.go` to emit ASCII letters and digits unchanged, emit one dash for each run of other characters, and trim leading and trailing dashes. Keep it pure and retain the empty-slug guard at its callers. Link destinations use the current helper. Task-root exclusion checks both the current root slug and the previous root slug, keeping pre-normalization task project directories out of credential aggregation.

Update `TestDeriveCursorProjectSlug` in `cursor_auth_bridge_test.go` with literal expected slugs for the task dot-folder example, Unix and Windows paths, punctuation runs, boundary trimming, empty result, and a non-ASCII case. In `TestCursorMCPAuthWorktree`, derive only the randomized temporary root prefix and keep the expected `.kandev/tasks/...` suffix literal.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| AC-AGENTS-CURSOR-AUTH-001.1 | `TestCursorMCPAuthWorktree` verifies the linked auth file exists where Cursor's project slug points. |
| AC-AGENTS-CURSOR-AUTH-001.4 | `TestDeriveCursorProjectSlug` verifies canonical path spelling, punctuation-run collapse, and Windows forms. |
| AC-AGENTS-CURSOR-AUTH-003.1 | Current and legacy configured task-root slugs are excluded from credential aggregation. |

First run the changed slug and worktree tests against the old helper and confirm the intended failure. Then apply the helper change and run the full work-order commands.

## Work orders

- [x] [Task 01: Match Cursor project slugs](task-01-match-cursor-project-slugs.md)

## Verification results

The added legacy-root regression failed against the fixup baseline because the regular auth file was aggregated, then passed after exclusion was extended to the previous slug. The full `mcpconfig` race suite, backend lint, backend build, specification catalog validation, spec lint, and `git diff --check` passed. The earlier broad backend test run had four failures outside this work order: two real-process-tree probe assertions and two launcher config-discovery tests selecting `/root/.kandev/config.yaml` instead of temporary configs.

The regression tests first failed against the old helper with extra dash runs and preserved non-ASCII characters, then passed after normalization changed.

- `go test -race -v -run 'Test(DeriveCursorProjectSlug|CursorMCPAuthWorktree|AggregateCursorMCPAuthExcludesConfiguredTaskRoot|AggregateCursorMCPAuthAllowsMissingExcludedRoot)$' ./internal/agent/mcpconfig/...` — passed.
- `go test -race -v ./internal/agent/mcpconfig/...` — passed.
- `go test -race -v ./internal/agent/runtime/lifecycle/...` — passed.
- `make -C apps/backend lint test` — lint passed with 0 issues; the full test target reported four unrelated failures. Isolated reruns reproduced `TestProbeRealTree_AllDescendantsPreTurn_Settled`, `TestProbeRealTree_NewDescendantAfterTurnStart_Live`, `TestInstallSystemdDiscoversFlagHomeConfiguration`, and `TestInstallSystemdDiscoversSystemHomeConfiguration`.
- `make -C apps/backend build` — passed.

## Risks

- Slug collisions and old double-dash project directories remain possible artifacts of Cursor's naming rule. The repair must preserve existing regular auth files and avoid deleting old directories.
- Cursor Agent CLI's bundled rule is supplied for this report. A future CLI version could change it; automated tests prove the stated rule, not every released Cursor version.
- The broad backend test target had unrelated failures during the original bridge PR; record any repeat failures separately from this repair.
