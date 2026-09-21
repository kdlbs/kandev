---
id: "02-enforce-tree-request-scope"
title: "Enforce tree request scope"
status: done
wave: 2
depends_on:
  - "01-normalize-tool-file-paths"
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-PATH-SCOPE-001
acceptance_criteria:
  - AC-UI-FILE-TREE-PATH-SCOPE-001.2
  - AC-UI-FILE-TREE-PATH-SCOPE-001.3
  - AC-UI-FILE-TREE-PATH-SCOPE-001.4
  - AC-UI-FILE-TREE-PATH-SCOPE-001.5
system_design:
  - ../../specs/ui/system-design/file-tree-path-scope.md
---

# Task 02: Enforce Tree Request Scope

## Summary

Admit only workspace-relative identities to reveal, stored expansion, and refresh requests, then
enforce the same tree contract before backend execution lookup and filesystem access.

## In scope

- Return early from active-file reveal for absolute, URI-shaped, or traversing paths.
- Sanitize restored expansion entries before deriving ancestors and retain valid siblings.
- Filter refresh fan-out and specific change paths before calling `requestFileTree`.
- Reject invalid tree paths in the WebSocket handler as validation failures without acquiring an
  execution or writing an ERROR log.
- Reject invalid tree paths in `WorkspaceTracker.GetFileTree` before join, stat, or filesystem
  failure telemetry.
- Preserve root, valid nested directory, symlink, missing relative path, and subtree merge behavior.

## Out of scope

- Editor path normalization delivered by Task 01.
- Changing file-content path rules.
- Suppressing logs for genuine backend dependency or relative filesystem failures.

## Acceptance

- The reported `/home/jcfs/...` active path never adds absolute ancestors, loads a tree node, or
  reaches `requestFileTree`, including after a generic refresh.
- Stored invalid entries are removed while valid relative expansion restores and persists normally.
- Direct absolute or parent-escaping tree requests fail validation before lifecycle or filesystem
  work and produce no ERROR log.

## TDD start

Add these failing regressions before production changes:

1. `file-tree-reveal.test.ts` supplies the reported absolute active file and expects no expansion or
   child load.
2. `file-browser-restore-expanded.test.tsx` stores valid relative paths beside the absolute chain and
   expects only the valid paths to be requested and retained.
3. `file-browser-apply-changes.test.ts` sends a generic refresh with absolute expanded paths and
   expects only `""` plus valid relative folders.
4. `workspace_files_test.go` calls `GetFileTree` with an existing absolute directory and expects a
   validation error rather than a joined lookup.
5. `workspace_file_handlers_test.go` supplies an absolute tree path with no lifecycle and expects
   `ErrorCodeValidation` and zero ERROR entries.

## ASCII UI preview

Relevant state transition from [UI-01](plan.md#ui-01-open-a-workspace-file-from-an-agent-tool-card):

```text
Workspace file active: public/.../case.json -> Files reveals public > ... > case.json
External file active:  /opt/reference.md     -> Files keeps its current expansion
Stored mixed state:    src, src/ui, /home/... -> restore src and src/ui only
```

No rendered layout or control changes. Desktop and phone consume the same sanitized Files-tree
state.

## Verification

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/file-tree-reveal.test.ts \
  components/task/file-browser-restore-expanded.test.tsx \
  components/task/file-browser-restore-loader.test.tsx \
  components/task/file-browser-apply-changes.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/file-tree-reveal.ts \
  components/task/file-browser-restore.ts components/task/file-browser-hooks.ts \
  --max-warnings 0)
(cd apps/backend && go test ./internal/agentctl/server/process \
  -run 'TestGetFileTree' -count=1)
(cd apps/backend && go test ./internal/agent/handlers \
  -run 'TestWorkspaceFileHandlers.*Tree' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/process \
  ./internal/agent/handlers -count=1)
make -C apps/backend lint
```

After both work orders:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

## Files likely touched

- `apps/web/components/task/file-tree-reveal.ts`
- `apps/web/components/task/file-tree-reveal.test.ts`
- `apps/web/components/task/file-browser-restore.ts`
- `apps/web/components/task/file-browser-restore-expanded.test.tsx`
- `apps/web/components/task/file-browser-restore-loader.test.tsx`
- `apps/web/components/task/file-browser-hooks.ts`
- `apps/web/components/task/file-browser-apply-changes.test.ts`
- `apps/backend/internal/agent/handlers/workspace_file_handlers.go`
- `apps/backend/internal/agent/handlers/workspace_file_handlers_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_files_test.go`

## Dependencies

- Task 01 supplies the shared frontend path classifier and prevents new tool opens from contaminating
  expansion state.

## Risks

- The empty root path is valid and must not be filtered with invalid empty segments.
- Hidden directories beginning with `.` are valid; only standalone `.` and `..` segments are
  rejected.
- A repository-scoped refresh must keep its current sibling-subtree preservation behavior.
- The handler guard is for invalid client input. Dependency failures after a valid request must keep
  their operational error signal.

## Parallelism

`sequential`

## Inputs

- `AC-UI-FILE-TREE-PATH-SCOPE-001.2` through `.5`.
- Tree-state admission, refresh flow, and backend validation in the system design.
- Existing persisted-expansion and cross-repository refresh regressions.

## Results

Implemented canonical tree-path admission for active-file reveal, stored expansion restoration,
generic refresh fan-out, and specific file-change refreshes. A shared Go validator now rejects the
same invalid identities before WebSocket execution lookup and before agentctl joins or stats a
filesystem path. Validation failures produce no operational ERROR log.

Validation passed:

- The combined focused frontend suite passed 90 tests across eight files.
- `go test ./internal/common/workspacepath -count=1` passed.
- Focused `TestGetFileTree` and `TestWorkspaceFileHandlers.*Tree` runs passed.
- The complete handler package passed, and `make -C apps/backend lint` reported zero issues.
- The full process package reached two unchanged runner-fixture timing failures on this host:
  `TestProcessRunnerCapturesOutput` and `TestProcessRunnerStopLogsSignalAttempts`. The branch has no
  changes to those tests or their production code; both fail identically when run alone.
