---
id: "01-project-cursor-shell-watches"
title: "Track Cursor shell watches in Background Work"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-011
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-011.1
  - AC-AGENTS-BACKGROUND-WORK-011.2
  - AC-AGENTS-BACKGROUND-WORK-011.3
  - AC-AGENTS-BACKGROUND-WORK-011.4
  - AC-AGENTS-BACKGROUND-WORK-011.5
  - AC-AGENTS-BACKGROUND-WORK-011.6
  - AC-AGENTS-BACKGROUND-WORK-011.7
  - AC-AGENTS-BACKGROUND-WORK-011.8
system_design:
  - ../../specs/agents/system-design/cursor-shell-watches.md
---

# Task 01 Track Cursor shell watches in Background Work

## Summary

Recognize the captured Cursor ACP watch lifecycle. Publish the corresponding
Monitor observations through the existing projection. Preserve the original
owner and let the shared viewer show the running and terminal states.

## In scope

- Add the raw issue capture to `testdata/cursor-shell-watch.jsonl`. Construct
  the test adapter with `AgentBackgroundWorkEnabled: true` and a bound session.
- Write `TestCursorShellWatchLifecycle` first. Replay the notifications through
  `handleACPUpdate`. Use the first Go command to reproduce the empty observation
  result before production edits.
- Add the Cursor parser hook and one watch map for the bound native session.
  Emit the existing Background Work event with the inspection-only capability
  helper. Resolve the launch origin with `promptTurnIDForGeneration`.
- Add the focused projection and ownership regressions. Update **Inspect
  background work** in `docs/public/tasks-and-workflows.md` with the availability
  of Cursor watches and the meaning of terminal and unresolved watch states.

## Out of scope

- Provider control RPCs, command execution, output capture, and history backfill.
- Changes to prompt admission, browser components, or rollout settings.

## Acceptance

- The feature-enabled raw replay yields one running and one terminal Monitor
  observation per watch. Pending alone creates no running count. Sparse
  completion keeps the launch title, source call, and origin generation.
- The real task repository retains the completed row and run while the sibling
  stays running. Duplicate, delayed, and wrong-owner updates preserve that
  result. The published projection and subsequent read agree.
- The normalized transcript remains generic and the active Cursor turn remains
  busy. The shared viewer retains its capability fallback on desktop and phone.
- Matching `failed` updates settle only their own watch and late progress keeps
  the failed record in History.
- A cancelled prompt interrupts only its still-running watches after queued
  notifications drain; terminal updates remain authoritative and exit status is
  empty.
- A committed native binding replacement moves unresolved running watches to
  Unknown through the reset event fence. Terminal watches are preserved, and a
  reused raw call ID creates an independent successor record.

## ASCII UI preview

**UI-01 Cursor watch inspection** uses the [full preview](plan.md#ascii-ui-preview).
`AC-AGENTS-BACKGROUND-WORK-011.1`, `.2`, and `.4` through `.8` apply.

```text
Desktop: [1 Background Job] -> popover [Open] -> central Monitor tab
Phone:   [1 Background Job] -> summary [Open] -> full-height Monitor [Back]
Detail:  Wait for build (/tmp/build.sh) | Output unavailable
After:   + panel menu / Panels -> Background Work -> History (1)
```

## Lifecycle evidence

| Scenario | Invariant and test | Criteria |
| --- | --- | --- |
| Two interleaved watches | `TestCursorShellWatchLifecycle` and `TestCursorShellWatchProjection` exercise shared shell references and distinct watch IDs. One completion leaves the sibling running. | `011.1`, `011.2` |
| Duplicate and delayed frames | The replay repeats registration, progress, and completion, then sends late progress. The projection keeps one terminal workload and run. | `011.2` |
| ACP failure | `TestCursorShellWatchFailureKeepsSiblingRunningAndRejectsLateProgress` and `TestCursorShellWatchFailureProjectionRetainsHistoryAndSibling` retain only the matching failure in History. | `011.6` |
| Confirmed prompt cancellation | `TestCancelledPromptInterruptsOnlyItsOpenCursorWatches` verifies notification ordering, prompt-generation ownership, and preservation of a drained completion. | `011.7` |
| Native binding replacement | `TestCursorShellWatchNativeResetRetiresRunningBinding`, `TestContextResetAcceptsRetiredWatchProjectionForCommittedSession`, `TestContextResetRetainsWatchRetirementsBeyondSetupBufferLimit`, and `TestCursorShellWatchResetProjectionSeparatesReusedCallID` verify retained-process reset, lossless retirement past the setup cap, and independent successor identity. | `011.8` |
| Session or execution replacement | `TestCursorShellWatchIdentity` sends the same raw ID across session and native generation boundaries. A new adapter assigns a fresh opaque ID. | `011.2` |
| Origin and admission | `TestBackgroundWorkObservationOriginTurn` resolves the original generation while a successor is current. `TestCursorShellWatchProjection` verifies that inspection leaves the Cursor turn busy. | `011.1`, `011.5` |
| Unusable evidence | `TestCursorShellWatchRejectsUnsupportedFrames` checks the disabled setting, foreign provider, misleading title, and a shell list with one missing ID. | `011.3` |
| Recovery and viewer entry | The existing teardown test covers confirmed owner exit. The two Monitor browser tests verify reload, independent History navigation, and the sibling count. | `011.1`, `011.2`, `011.4` |

Cursor supplies no live discovery snapshot in this capture. Follow the existing
process-end settlement path for lost updates.

## Verification

Run the commands from the repository root. On a fresh worktree, install the web
dependencies from `apps/` with `pnpm install --frozen-lockfile`.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestCursorShellWatchLifecycle$' -count=1 -v)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/lifecycle ./internal/orchestrator -count=1)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestContextResetRetainsWatchRetirementsBeyondSetupBufferLimit|TestContextResetAcceptsRetiredWatchProjectionForCommittedSession|TestFinishContextResetReturnsEventsBufferedDuringFinalization)$' -count=1 -v)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -count=1)
make -C apps/backend build
(cd apps && pnpm --filter @kandev/web exec vitest run components/task/chat/background-work/background-work.test.tsx)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/claude-monitor-lifecycle.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-claude-monitor-lifecycle.spec.ts)
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch_prompt_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_plugin_claims.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_plugin_claims_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go \
  apps/backend/internal/agent/runtime/lifecycle/execution_context_reset.go \
  apps/backend/internal/agent/runtime/lifecycle/execution_context_test.go \
  apps/backend/internal/agent/runtime/lifecycle/types.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_events.go \
  apps/backend/internal/agent/runtime/lifecycle/events_background_work_test.go \
  apps/backend/internal/orchestrator/cursor_shell_watch_projection_test.go
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 golangci-lint run \
  ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agent/runtime/lifecycle ./internal/orchestrator \
  --new-from-rev=8aab9447b35f1137485105bf06c8c27d0e0ef426 \
  --timeout=15m --concurrency=2)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/cursor_shell_watch_prompt_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_plugin_claims.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_plugin_claims_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/claude_monitor_completion_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/cursor-shell-watch.jsonl`
- `apps/backend/internal/agent/runtime/lifecycle/execution_context_reset.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_context_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_background_work_test.go`
- `apps/backend/internal/orchestrator/cursor_shell_watch_projection_test.go`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None.

## Risks

- Associate progress and completion with the captured launch generation. A
  lookup of the active turn can misattribute a delayed event to a successor.
- Keep each terminal state in the same map entry for duplicate handling. Match
  the watch call ID because two watches can share a shell reference.

## Parallelism

`sequential`

## Inputs

- [Issue #5485 notification parameters](https://github.com/kdlbs/kandev/issues/5485)
- [Cursor watch requirements](../../specs/agents/requirements/cursor-shell-watches.md)
- [Cursor watch design](../../specs/agents/system-design/cursor-shell-watches.md)
- `claude_monitor_lifecycle_projection_test.go` for the durable fixture pattern.
- `event_handlers_background_work.go` for the observation path.

## Results

The initial raw replay failed on the baseline at the empty Background Work
observation assertion. After implementation, the same replay produced one
running and one terminal observation for each interleaved watch. Ownership,
unsupported-frame, identity-boundary, duplicate/delayed-update, and projection
regressions passed. ACP failure, cancelled-prompt, and native-reset regressions
verify terminal precedence, prompt ownership, reset-fence delivery, and call-ID
reuse. The PR review regression first reproduced the reset-buffer loss with 64
setup events and 71 retirements; the focused reset tests now retain every
retirement while preserving the setup cap.

- The complete ACP, lifecycle, and orchestrator package suites passed with the
  race detector enabled. The backend build passed for the host and helper
  binaries.
- The shared Background Work viewer suite passed all 27 tests.
- Desktop Chromium and mobile Chromium Monitor lifecycle E2E each passed one
  test. Both E2E runs built the host backend and web assets successfully.
- After rebasing onto `8aab9447b35f`, the ACP, lifecycle, and orchestrator race
  suites and backend build passed. The changed-file lint wrapper reported zero
  issues but exceeded its five-minute limit; the package-level `golangci-lint`
  invocation with a 15-minute timeout passed with zero issues.
- The complete lifecycle package passed with the race detector after the reset
  buffer fix. Changed-file Go lint passed with zero issues.
- `make -C apps/backend build` passed after the reset buffer fix.
- `011.8` reset-buffer regression: the focused reset tests passed after first
  reproducing the dropped retirements with 64 setup events and 71 watches.
- Public documentation validation passed all 63 tests and validated 56 pages.
  Documentation index validation passed for 464 decisions and 2,110 specs;
  all specification lint checks passed.
- `git diff --check` passed.
