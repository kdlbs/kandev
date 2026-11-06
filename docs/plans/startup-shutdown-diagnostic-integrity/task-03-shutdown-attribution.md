---
id: "03-shutdown-attribution"
title: "Attribute planned and unexpected shutdown"
status: completed
wave: 2
depends_on:
  - "02-safe-child-diagnostics"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-004
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.4
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 03: Attribute planned and unexpected shutdown

## Summary

Deliver the planned graceful signal while the parent-liveness writer remains open.
Describe unexpected pipe closure truthfully and preserve orphan cleanup.

## In scope

- Move owned writer closure after graceful signal and the existing child-exit/forced-cleanup wait.
- Close descriptors on success, signal failure, context timeout, and forced-exit failure without adding another wait budget.
- Use accurate closure wording in `waitForShutdown`. Preserve info-level signal handling and ownership-shutdown handling.
- Keep nil monitors and survival-enabled launches unchanged.
- Add helper-process evidence for planned Stop and abrupt loss, with explicit resource ownership and bounded joins.

## Out of scope

- A new pipe byte protocol, acknowledgement, longer timeout, disabled parent monitor, or changes to shell/agent escalation.
- Changing Unix process groups, Linux parent-death primitives, Windows containment, or survival policy.

## Acceptance

- A real planned Stop emits signal-triggered child shutdown without the false parent-death warning. The writer remains open until exit/wait completion.
- EOF without a planned signal still invokes cleanup and records the observed pipe closure. Abrupt parent loss releases the child and its owned descendants.
- Existing timeouts, forced cleanup, exit callbacks, error returns, survival-enabled no-op Stop, and nil-monitor handling retain their behavior.

## Regression evidence

Add `TestLauncherPlannedStopKeepsLivenessUntilChildExit` with a helper that distinguishes signal receipt from EOF.
The recorded base fails because Stop closes the writer first.
Add `TestParentLivenessEOFDoesNotClaimProcessDeath` through real pipe monitoring and a logger observer.
The recorded base fails the message assertion while its cleanup callback still runs.
Add `TestAbruptParentLossCleansOwnedChild` using owned helper processes and explicit termination channels.
Unix-only process assertions use build constraints. Platform-neutral classification and nil-monitor tests stay untagged.

## Verification

Run the new regressions first and record their expected failures.
After implementation, run this block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/agentctl/launcher ./cmd/agentctl -run 'LauncherPlannedStop|ParentLiveness|AbruptParentLoss|LauncherStop|ParentPipe|LivenessPipe|RuntimeShutdown|UnexpectedExit|Shutdown' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go apps/backend/cmd/agentctl/main.go apps/backend/internal/agent/runtime/agentctl/launcher/launcher_shutdown_test.go apps/backend/cmd/agentctl/parent_liveness_test.go
git diff --check
```

Run the same focused Go command on macOS before delivery and record its result.
Linux evidence alone does not close the reported macOS process boundary.
If macOS execution is unavailable, retain that validation limitation explicitly.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_shutdown_test.go` (new)
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/cmd/agentctl/parent_liveness_test.go` (new)
- Existing launcher survival tests, only where fixture adaptation is necessary

## Dependencies

Task 02 owns the preceding launcher changes. Read its final parser and observer behavior before editing.

## Risks

Signal/EOF races and concurrent unexpected exits can hide a descriptor leak in a message-only test.
Use real helpers and preserve existing forced process-tree cleanup.
A test helper must never signal the host backend or another test's process group.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), [requirements](../../specs/platform/requirements/runtime-diagnostic-signal.md), and the design's shutdown-attribution section.
- `launcher_test.go` and `survival_kill_path_test.go`.
- Current `Launcher.Stop`, platform signal helpers, `monitorParentLiveness`, and `waitForShutdown`.
- `.agents/skills/tdd/references/backend-tests.md` for subprocess and cleanup ownership.

## Results

- `Launcher.Stop` now sends the graceful signal while the liveness writer stays open and closes owned descriptors on every return path.
- Unexpected parent-liveness closure now logs `parent liveness pipe closed, initiating shutdown` without claiming process death.
- Added planned-stop, EOF-attribution, and abrupt-loss helper-process regressions. Both attribution tests drive the production `monitorParentLiveness`, and the abrupt-loss helper also runs `waitForShutdown` and reaps an owned descendant process.
- Verification passed: focused race tests, lint, and `git diff --check`.
- macOS execution was unavailable; the Linux process results retain that explicit delivery limitation.
