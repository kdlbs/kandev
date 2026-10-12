---
id: "03-launch-integration"
title: "Launch integration and delivery verification"
status: complete
wave: 3
depends_on:
  - "02-profile-preference"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-002
  - REQ-AGENTS-CURSOR-AUTH-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-001.1
  - AC-AGENTS-CURSOR-AUTH-001.3
  - AC-AGENTS-CURSOR-AUTH-001.4
  - AC-AGENTS-CURSOR-AUTH-001.5
  - AC-AGENTS-CURSOR-AUTH-002.3
  - AC-AGENTS-CURSOR-AUTH-002.4
  - AC-AGENTS-CURSOR-AUTH-003.7
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
---

# Launch integration and delivery verification

## Summary

Apply the preference before eligible local Cursor launches and verify the complete feature.

## Scope and owned files

- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp.go` and its tests.
- A focused lifecycle bridge helper, plus `manager_passthrough.go` and `manager_passthrough_mcpfiles_test.go`.
- `manager_launch.go` for resolved-profile forwarding at launch and workspace promotion.
- Existing execution-profile, environment, and runtime classification helpers as required for eligibility.
- `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge_test.go` for the temporary-worktree integration case.
- `docs/public/agents-and-profiles.md`, this package's results/statuses, and paired specifications after verified implementation.

Exclude remote credential transport, background refresh, process termination, and automatic OAuth login.

## Implementation acceptance

1. Both Cursor launch paths prepare auth before process start, including empty profile MCP configuration and workspace promotion.
2. Locality and resolved-profile tests prove opt-out, non-Cursor no-op, remote no-op, and failure policy.
3. A temporary git worktree receives the expected link. All required backend and desktop/phone checks pass.

## TDD and verification

Write lifecycle RED tests before launch changes.
Inject a temporary home and use synthetic source credentials.
Cover disabled cleanup failure, missing Cursor home, malformed sources, canonical path aliases, and preserved regular destinations.
Use the actual resolved execution profile in an Office/dynamic-profile case.
Cover resumed process launch and prove an existing running process is not reconfigured.
Add `TestCursorMCPAuthWorktree` with a temporary repository and real git worktree.
Assert `Readlink`, master JSON, and absence of writes to the original source files.

From `apps/backend`:

```bash
go test -v ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...
go test -race ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...
go test -v ./internal/agent/mcpconfig/... -run TestCursorMCPAuthWorktree
```

From the repository root:

```bash
make -C apps/backend test
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

From `apps/web`, after backend integration:

```bash
pnpm e2e:run --host --project chromium -- tests/settings/cursor-mcp-auth.spec.ts
pnpm e2e:run --host --project mobile-chrome -- tests/settings/mobile-cursor-mcp-auth.spec.ts
```

Run commands sequentially with the repository worker limits.
Record results in this work order and the plan.
Public docs use a short reference section in the existing agents guide.
Explain local eligibility, newest-file precedence, next-launch opt-out, and token-refresh limitations.

## Dependencies and risks

Depends on Task 02 and its profile resolver field.
Promotion updates execution identity after part of launch preparation. Pass the incoming resolved profile explicitly.
Do not append persistent bridge files to session-owned MCP cleanup lists.
The fixture proves filesystem behavior, not successful OAuth against live Atlassian or Figma.

## Results

Completed on 2026-09-25.

- Passed `make -C apps/backend build`.
- Passed `go test -v ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...` and `go test -race ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...`.
- Passed `pnpm run typecheck`, `pnpm run i18n:check`, and the five focused Vitest files: 104 tests passed.
- Passed desktop Playwright: 2 tests; mobile Chrome Playwright: 1 test.
- Passed public-doc validation (62 validator tests and 47 published pages), specification validation/lint (305 decisions, 1144 specifications), and `git diff --check`.
- `make -C apps/backend test` ran but failed in checks outside this change. `internal/agentctl/server/process/probe` failed `TestProbeRealTree_AllDescendantsPreTurn_Settled` and `TestProbeRealTree_NewDescendantAfterTurnStart_Live` both in the full run and in isolation. `internal/common/config` and `internal/launcher` failed during the broad run because the host environment injected `KANDEV_INTERNAL_CONFIG_FILE=/root/.kandev/config.yaml`; both packages pass when run in isolation with `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` unset.
- Review follow-up: `resolvePassthroughAgent` now obtains agent and profile together and propagates resolver failures; eligible local Cursor preparation rejects unresolved profile data. A resume regression verifies opt-out cleanup happens before a later resolver error can prevent process start. Expected test paths resolve the workspace independently, with deterministic symlink-parent coverage. Public documentation now limits unrelated-symlink preservation to disabled cleanup.
- Re-ran `go test -v ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...`, the matching `-race` command, `make -C apps/backend build`, public-doc tests/validation, `gofmt`, and `git diff --check`; all passed. The broad backend suite was not rerun during review follow-up.

PR review follow-up on 2026-09-25 also normalizes Windows Cursor project slugs and tests them in the Windows workflow; excludes the configured task-worktree root; recognizes HOME aliases by filesystem identity; and omits unchanged Cursor auth values from profile-save patches. It regenerates both settings discovery snapshots that the initial PR's frontend CI found stale, strengthens the settings-default assertion, and guards symlink-dependent tests on platforms without symlink support. The public guide and authoritative requirements/design now disclose that credentials are shared by exact server name without URL or issuer comparison. A synthetic test records that accepted trust boundary. Existing links remain when a refresh has no valid source, as required by the approved system-design compatibility limit.

The initial PR-head frontend check failed because its generated settings snapshot was stale. The generated files are now current. Backend results from the original implementation remain as documented above; the broad backend suite was not rerun for this review follow-up.


Local review-fixup verification passed on 2026-09-25:

- `go test ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/... ./internal/settingscatalog/...`
- `go test -race ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/... ./internal/settingscatalog/...`
- `GOOS=windows GOARCH=amd64 go test -c -o /tmp/cursor-mcpconfig-windows.test.exe ./internal/agent/mcpconfig`
- `golangci-lint run ./... --new-from-rev=b88aea31ad49b2cda40e8ad84452356888e36a42 --timeout=5m`
- `make -C apps/backend build`
- Cursor profile and agent-level save Vitest suites (49 tests combined), web typecheck, targeted ESLint, and Prettier check.
- Settings contract CI test (3 tests), public-doc validation (62 tests, 47 pages), specification catalog validation/lint, and `git diff --check`.

The generated settings snapshots now pass `node --test scripts/settings-contract-ci.test.mjs`. Refreshed PR checks will be recorded after pushing the fixup commit.
