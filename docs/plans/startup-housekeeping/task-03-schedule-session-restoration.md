---
id: "03-schedule-session-restoration"
title: "Prioritize independent promptless session restoration"
status: done
wave: 3
depends_on:
  - "02-gate-runtime-activation"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-004
acceptance_criteria:
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.2
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.3
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.4
system_design:
  - ../../specs/platform/system-design/startup-housekeeping.md
---

# Schedule independent session restoration

## Summary and scope

Use TDD to remove the serial global restore loop. Bound restore operations to
two, prioritize the focused pending session, and deduplicate per session. Use
existing promptless native restore and guarded authenticated attachment, retaining
the same conversation. Restoration must not release queued instructions.

## Owned files and dependencies

- `orchestrator/silent_restart_recovery.go`, recovery input/commit and
  already-resumed/legacy-native paths that must share admission.
- `orchestrator/session_launch.go`, `idle_session_focus.go`, and recovery tests.
- `apps/web/hooks/domains/session/use-session-resumption.ts` and its request
  guards/tests; existing `session.launch` protocol and activation source.
- Real journal/checkpoint fixtures in existing recovery tests.

Task 02 must be complete. No new endpoint, settings, provider support, or general
queue semantics. This work changes scheduling and effect admission, not rendered
layout; shared hook tests plus task 04 mobile E2E establish parity.

## Implementation acceptance

1. With one independent restore held, another becomes ready; a focused pending
   candidate wins the next available slot. Repeated open/focus joins one attempt,
   preserves the two-operation bound, and respects session capacity.
2. Startup/open/focus/reconnect produce zero new prompt submissions, user turns,
   tool replays, or queued dispatches, including legacy and already-resumed paths.
   Uncertainty remains truthful while the native session identity is preserved.
3. Stop/archive/shutdown and stale completion races fence pending work and join
   owned workers without harming a successor. Unsupported or ambiguous ownership
   preserves the existing blocked/pending outcome.

## Verification

Install workspace dependencies once from `apps/` if this worktree has no install.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'Silent|Restart|Restor|Focus|AlreadyResumed|LegacyNative|PendingRemote' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption*.test.*)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Run changed-file Go lint and targeted ESLint on changed frontend source. TDD
must observe the concurrency and no-dispatch regressions fail before changing
production code. Do not substitute sleeps for deterministic barriers.

## Risks

Concurrent attempts must not share a lifecycle lock across sessions or bypass a
per-session lock. Frontend priority signals must be limited to the visible
selected session and must not silently opt hidden sessions into restoration.
Reading retained output is allowed; emitting instructions is not.

## Results

Implemented the service-owned two-slot native restore dispatcher with deduplication,
focused-session priority, cancellation, and joined shutdown. Focus joins the same
promptless attempt through the existing session activation source. Native legacy
and already-resumed paths preserve queued-prompt pause.

Validation: race-enabled orchestrator, executor, and lifecycle restore/focus/
legacy/pending-owner regressions passed. The concurrency and legacy no-dispatch
regressions failed before the implementation. All 120 focused frontend tests
passed across 11 files, including focused-session resumption; web typecheck passed
with an 8 GiB Node heap. Changed-file Go lint and targeted ESLint passed. Desktop
and phone two-conversation restart checks passed in task 04's isolated E2E runs.

PR review refinement: the pass drains one bounded metadata page before fetching
another. The regression failed with both restore workers blocked and passed after
the change, including the positive control that the next page is read after drain.
The existing independent-candidate, focus, deduplication and shutdown tests passed
under the race detector.
