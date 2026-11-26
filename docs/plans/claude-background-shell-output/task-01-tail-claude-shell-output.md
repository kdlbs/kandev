---
id: "01-tail-claude-shell-output"
title: "Tail Claude background shell output"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-004.1
  - AC-AGENTS-BACKGROUND-WORK-010.4
  - AC-AGENTS-BACKGROUND-WORK-010.6
  - AC-AGENTS-BACKGROUND-WORK-010.10
system_design:
  - ../../specs/agents/system-design/claude-background-shell-output.md
---

# Task 01: Tail Claude background shell output

## Summary

Read the output file that a verified Claude detached shell's launch result
names, and stream it through the existing `background_work_output` contract so
the desktop and phone viewers show it live and after exit.

## In scope

- New `acp/claude_shell_output.go`:
  - `claudeShellOutputPath(stdout, nativeSessionID string) (string, bool)`:
    read to the first line break, find the first
    `tasks[/\\]<name>.output` suffix and its sentence/whitespace boundary,
    clean the path, and apply the
    [design checks](../../specs/agents/system-design/claude-background-shell-output.md#recognition-boundary).
  - `claudeShellOutputFollower`: a per-adapter map of followed jobs and at
    most one poller goroutine, running only while the map is non-empty. Every
    2 s it stats each file and reads only grown files, capped to the last
    200 KiB of unread bytes. Gate output until the matching launch update is
    queued; jobs that never activate still expire after 30 minutes without
    growth. Emit to the last complete UTF-8 rune, normalize malformed UTF-8,
    and count offsets in delivered output bytes. Do not wait for a newline;
    carry a split trailing rune to the next poll. Limits: 20 followed jobs,
    file gone/shrunk/non-regular. The clock is injectable for tests.
- `acp/adapter.go`: own the follower for the adapter lifetime and stop it
  during adapter close. The follower starts its poller when a job is registered.
- `acp/adapter_tools.go`: where the verified launch gets its `WorkID`, validate
  the path, set `Output: "stream"`, and register the job. On the verified
  terminal exit, do a final read and remove the job before sending the
  terminal observation.
- `acp/dialect_claude_terminal.go`: drop the whole map on native-session
  replacement.
- `acp/normalize.go`: keep the initial shell capability inspection-only until
  a valid output file has been accepted.
- `cmd/mock-agent/claude_background_terminal_fixture.go`: write a real output
  file and put the "Output is being written to:" sentence in the banner.
- `cmd/mock-agent/claude_background_terminal_fixture_test.go`: verify the
  fixture exposes and updates its output file.
- One desktop E2E, `claude-background-shell-output.spec.ts`, reusing
  `claude-background-terminal-exit.helpers.ts`.
- Phone rendering uses the same retained output contract; existing
  `mobile-opencode-background-work.spec.ts` verifies output in the phone
  detail. No UI composition changes or additional mobile spec are needed.

## Out of scope

AIR capabilities, ACP v2, timeout-moved shells, Subagent/Monitor files, web
code, locales, feature flags, and resuming reads after restart.

## Acceptance

1. `TestClaudeBackgroundShellOutputStreamsFromLaunchFile`: a verified 0.109.0
   launch advertises `Output: "stream"`, its update is queued before the first
   output chunk, and the final chunk precedes the terminal observation.
2. `TestClaudeShellOutputPath`: both launch sentences pass with a path that
   contains spaces; repeated separators are cleaned; relative, foreign-session,
   wrong-shape, and non-regular paths keep `Output: "none"`.
3. Follower tests emit progress without a newline, cap growth beyond 200 KiB
   between polls, carry a split UTF-8 rune to the next poll, and keep output
   offsets consistent after JSON replaces malformed UTF-8.
4. Follower limit tests cover an unchanged file, 30-minute expiry for activated
   and unactivated jobs, a 21st job keeping `Output: "none"`, and poller exit
   when no job remains.
5. The desktop E2E opens live detail, appends more output, observes it before
   releasing terminal frames, then verifies retained output after exit.
6. The existing desktop and mobile terminal-exit E2Es pass with the updated
   fixture signal protocol.
7. The existing mobile E2E verifies retained output in the phone detail.

## TDD and verification

Write regression tests first and confirm the launch-order and unactivated-job
tests fail against the implementation without their fixes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp/... -run 'ClaudeBackgroundShellOutput|ClaudeShellOutputPath|ClaudeShellOutputFollower|ClaudeBackgroundTerminal' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp/... ./cmd/mock-agent/... -count=1)
bash scripts/lint-go-changed apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go apps/backend/internal/task/service/background_work_test.go
(cd apps/backend && go test -trimpath ./internal/task/service -run '^TestBackgroundWorkOutputMergeUsesJSONDeliveredUTF8Offsets$' -count=1)
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
(cd apps && pnpm --filter @kandev/web exec playwright test e2e/tests/task/claude-background-shell-output.spec.ts e2e/tests/task/claude-background-terminal-exit.spec.ts)
make -C apps/backend build-e2e-delivery-fixture build-e2e-send-now-fixture
make -C apps/backend e2e-plugin-package
(cd apps && pnpm --filter @kandev/web exec playwright test --config=e2e/playwright.config.ts --project=mobile-chrome tests/task/mobile-claude-background-terminal-exit.spec.ts tests/task/mobile-opencode-background-work.spec.ts)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
git diff --check
```

## Likely files

- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_shell_output_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_background_terminal_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_terminal.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go`
- `apps/backend/cmd/mock-agent/claude_background_terminal_fixture.go`
- `apps/backend/cmd/mock-agent/claude_background_terminal_fixture_test.go`
- `apps/web/e2e/tests/task/claude-background-terminal-exit.helpers.ts`
- `apps/web/e2e/tests/task/claude-background-shell-output.spec.ts` (new)
- `apps/backend/internal/task/service/background_work_test.go`

## Results

Implementation complete. The review regression tests failed before the fix and
now pass with the focused ACP race suite. Full ACP and mock-agent package tests
pass. The changed-Go lint script reports zero issues. The backend build passes.
Desktop output and terminal-exit E2Es pass; the Pixel 5 terminal-exit and
retained-output E2Es pass. The web build and all three required E2E fixture
builds pass. Specification validation, spec lint, Prettier, `gofmt`, and
`git diff --check` pass.

PR fixup verification: the focused ACP race suite and full ACP/mock-agent
package tests pass after adding launch-queue activation, expiry for jobs that
never activate, and bounded event waits in the cancellation test. The service
regression test confirms JSON-delivered replacement characters and normalized
byte offsets merge without losing later output. Changed-Go lint reports zero
issues; `make -C apps/backend build`, specification validation, spec lint, and
`git diff --check` pass. `pnpm install --frozen-lockfile` succeeds, and
`pnpm e2e:run tests/task/claude-background-shell-output.spec.ts` passes with
the viewer observing output appended while the job is still running, before
terminal frames are released.

PR fixup (lock ordering): the blocked-send activation regression failed before
the fix and passes with the full ACP race suite. Output events are now built
under the job state lock and sent after it is released; a per-job delivery lock
preserves read and event order, including the final chunk. Changed-Go lint
reports zero issues, documentation validation succeeds, and all specification
files pass lint.
