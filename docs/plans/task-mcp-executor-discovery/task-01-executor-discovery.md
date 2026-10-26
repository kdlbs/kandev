---
id: "01-executor-discovery"
title: "Expose and verify executor discovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-TASK-DISCOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-TASK-DISCOVERY-001.1
  - AC-EXECUTORS-TASK-DISCOVERY-001.2
  - AC-EXECUTORS-TASK-DISCOVERY-001.3
  - AC-EXECUTORS-TASK-DISCOVERY-001.4
  - AC-EXECUTORS-TASK-DISCOVERY-001.5
  - AC-EXECUTORS-TASK-DISCOVERY-001.6
system_design:
  - ../../specs/executors/system-design/task-mcp-discovery.md
---

# Task 01: Expose and verify executor discovery

## Summary

Make the existing catalogue operation discoverable in task mode. Preserve the
per-executor tool and all ownership and mutation boundaries.

## In scope

1. Write `TestTaskExecutorDiscovery_CustomProfileSelection` first. Its initial
   failure must show that task mode cannot call `list_executors_kandev`.
2. Add shared read registration and task-mode guidance. Update the existing
   absent-tool assertion and exact counts after recording the failing regression.
3. Add real scoped integration coverage with generated custom executor IDs.
   Include shared, caller-owned, and foreign executors together, plus profiles
   with different owners on a shared executor. Verify ordinary callers,
   configuration managers, authentication-disabled mode, and invalid identity.
4. Pass a discovered profile to non-starting task creation. Revoke access and
   verify the existing creation/admission boundary rejects the foreign profile.
5. Cover empty listings and failures of either executor or profile reads.
6. Add a discovery subsection to `docs/public/automation-and-mcp.md` explaining
   the catalogue, nested profiles, per-executor lookup, and ownership filtering.

## Out of scope

New permissions, executor mutations, browser interfaces, provider provisioning,
and changes to unrelated MCP surfaces.

## Acceptance

- All referenced criteria have executable evidence, including a failing-first
  task-mode regression and a real identity-filtered protocol flow.
- Task mode gains only the read tool. Existing tool schemas, result DTOs, other
  surface membership, and admission checks remain compatible.
- Public reference documentation describes the implemented discovery sequence.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers -count=1 -timeout=30m)
(cd apps/backend && go test ./internal/task/service -run '^TestExecutorOwnership' -count=1)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/config_handlers.go apps/backend/internal/mcp/server/server_test.go apps/backend/internal/mcp/server/task_executor_discovery_integration_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If another Go file changes, include it in the changed-file lint command.
Run the PR-documentation coverage preflight against the final changed files.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/task_executor_discovery_integration_test.go` (new)
- `docs/public/automation-and-mcp.md`
- This work order and `plan.md` for execution results.

## Dependencies

None.

## Risks

Do not register the configuration executor group wholesale in task mode.
Use the integration fixture pattern without bypassing the real scope resolver.
Assert protected profile fields are absent, not only that totals differ.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/task-mcp-discovery.md).
- [Design](../../specs/executors/system-design/task-mcp-discovery.md).
- `apps/backend/internal/mcp/server/config_automation_integration_test.go`:
  dispatcher-backed MCP integration pattern.
- `apps/backend/internal/mcp/handlers/mcp_identity_scope_test.go`:
  trusted task identity fixture.
- `apps/backend/internal/task/service/executor_ownership_test.go`:
  ownership catalogue and admission assertions.

## Results

The implementation and verification are complete.

- The task-mode registration regression failed first because
  `list_executors_kandev` was not registered.
- The complete MCP server/handler run passed with `-timeout=30m`; the server
  package completed in 23.632s and handlers in 957.328s.
- `go test ./internal/task/service -run '^TestExecutorOwnership' -count=1`
  passed. The task-creation handler group passed in 220.510s, and focused
  executor-list and self-placement inheritance tests passed.
- An initial implementation run exposed that inherited profile IDs were being
  treated as explicit selections. Task MCP now marks only caller-supplied
  profile IDs for the deferred ownership recheck. The inheritance fixture uses
  a real profile for its explicit-selection case.
- Changed-Go lint passed with zero issues. `make build` passed for the backend
  and agentctl targets.
- PR fixup verification added an explicit read-only annotation for
  `list_executors_kandev`; its metadata regression test failed before the
  annotation and passed after it. The MCP server package test passed.
- The sidebar presentation E2E now waits for saved task-row settings to reach
  the API. The active-task storage E2E has a 180-second test deadline for its
  bounded setup, activity, and cleanup waits, and the SSH cleanup E2E waits
  75 seconds against the runtime's 60-second cleanup bound. All three affected
  E2E cases passed locally with retries disabled.
- Public-doc tests passed (63/63), public-doc validation passed (54 pages),
  catalogue validation passed (403 decisions and 1,742 specs), specification
  lint tests passed (36/36), all-spec lint passed, and `git diff --check`
  passed.
- Actual-path PR documentation coverage returned `covered` with no errors and
  selected this work order.

### PR fixup after base advancement

- CI rerun `40376689011`, attempt 2, failed shard 13 because
  `a breadcrumb too long to fit keeps the reveal control inside the popover`
  measured the breadcrumb before the final asynchronous directory navigation
  completed. The `test-results-13` artifact's
  `normal-13-4/task-mobile-directory-brow-f64cb--control-inside-the-popover-mobile-chrome/error-context.md`
  captured the failure. The retry summary recorded one flaky test and zero
  final test failures; `E2E_FAIL_ON_FLAKY=1` correctly failed the shard.
- `navigateIntoDeepDirectory` now waits for the final breadcrumb to become the
  disabled current-directory item before measuring layout. The focused mobile
  test passed four repetitions with retries disabled through the managed E2E
  runner on the merged `main` tree:
  `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts -- --grep 'a breadcrumb too long to fit' --repeat-each=4 --retries=0`
  (4 passed, 46.0s). The complete spec also passed with retries disabled:
  `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts -- --retries=0`
  (2 passed, 18.0s).
- The affected backend packages passed on the merged tree:
  `go test -trimpath ./internal/mcp/handlers ./internal/mcp/server -count=1 -timeout=30m`
  (`handlers` 599.897s, `server` 24.942s). This verifies main's task-priority
  fields and the PR's explicit executor-profile selection flag together.
- Changed-Go lint reported zero issues, and `make -C apps/backend build`
  passed. Fresh PR checks for the pushed merge commit remain the final CI gate.
