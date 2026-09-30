---
id: "06-review-remediation"
title: "Fix review findings in snapshot safety and recovery"
status: done
wave: 6
depends_on: ["05-browser-proof"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.22
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.34
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.35
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 06: Fix review findings in snapshot safety and recovery

## Summary

Close eight source-confirmed review findings from the implementation. Keep complete file membership independent of detail readiness, and preserve strict source, capture, cancellation, and retry ownership across backend and frontend paths.

## In scope

1. Give each tracker lifetime an unambiguous opaque identity. Compare revisions only inside that identity; use capture-time and backend execution/workspace-generation validation across sources. Fence retired execution and stream generations on workspace callbacks, initial subscribe, HTTP, and correlated refresh paths.
2. Never promote prior diffs/statistics to ready for new evidence unless the exact content/index/HEAD/comparison identity proves equivalence. Keep pending mixed facets pending.
3. Start the tracker-owned enrichment deadline before any validation. Pass that bounded background-admission context to every validator and digest, while keeping fresh membership commands interactive and validating against the live index.
4. Separate raw evidence limits from emitted diff limits. Keep basic membership fast and ensure one oversized or high-aggregate source set cannot suppress supported file details or independent statistics.
5. Include submodule identity and observed nested HEAD in equality and revalidate it before publication.
6. Evict aborted refresh attempts synchronously, fence cleanup by request identity, and settle canceled refresh state without touching a successor. Cover coordinator ownership and StrictMode-like release/re-retain ordering.
7. Gate refresh-state transitions with the same source/revision/timestamp ordering decision as snapshots. Older ready, loading, or failure frames cannot alter current state.
8. Propagate Git command failures as unavailable detail outcomes, preserve exact-evidence healthy details where possible, and retry unavailable enrichment on an unchanged explicit refresh.
9. Preserve archive diff data when optional status metadata fails; accept SHA-1 and SHA-256 object IDs; keep file-diff readiness independent from a missing implicit ancestry ref.
10. Start or join a fresh, bounded snapshot request on each Changes activation even when a complete snapshot is cached. Continue scoped sibling-source probing after failed/incomplete results, and cap each WebSocket connection at four active Git refreshes with correlated rejection above the cap.
11. Fence every workspace stream callback by its captured startup generation and agentctl client identity, including callbacks arriving after replacement.

## Verification

Use deterministic gates, barriers, or injected Git-command outcomes. Prove each regression fails before its production fix. Run targeted frontend/backend tests after each sub-workstream, then:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/gateway/websocket ./internal/orchestrator -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/git-status-refresh-coordinator.test.ts hooks/domains/session/use-session-git-derived.test.ts lib/state/slices/session-runtime/git-status-state.test.ts lib/ws/handlers/git-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps && pnpm --filter @kandev/web build:vite)
(cd apps/backend && make build)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Additional named regressions cover archive metadata failure, SHA-256 repositories, retry admission between unavailable publication and worker settlement, source fallback to a healthy sibling, fresh activation with a complete cache, stale correlated quality frames, correction-worker shutdown, and per-connection refresh admission. Lifecycle callback tests reject retired startup generations and replaced agentctl clients.

## Parallelism

Sequential in the primary session. The findings touch shared ordering and publication contracts.

## Results

All eight review findings are fixed with deterministic regression coverage. Tracker IDs now qualify each tracker lifetime and frontend revisions order only within that identity; retired executions and workspace callbacks/initial-subscribe reads are fenced. New snapshots no longer inherit prior diffs as ready. Enrichment validation uses tracker-owned background admission under its own deadline. Evidence hashing is separate from the diff-output cap, submodule HEAD participates in evidence equality, and failed detail commands publish unavailable quality while retaining healthy details and allowing unchanged explicit retry. Refresh attempts are synchronously evicted and source-order checks gate both snapshot and quality transitions.

PR fixup additionally covers task-root promotion after an execution starts: a live source may remain at its exact registered repository worktree path, while unregistered and retired paths stay fenced. The active repository inventory is revalidated after the asynchronous refresh response. The multi-repository E2E waits for the correlated refresh response, checks the two-repository result shape, and verifies both groups render in Changes; the right-pane E2E explicitly selects Files after the right column is restored, and the navigation retry E2E reselects Files before operating its status action.

- `(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`: passed; process 164.815s, API 88.994s.
- `(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/gateway/websocket ./internal/orchestrator -count=1)`: passed; lifecycle 88.263s, backendapp 94.242s, WebSocket 4.998s, orchestrator 159.826s.
- `(cd apps/web && pnpm exec vitest run hooks/domains/session/git-status-refresh-coordinator.test.ts hooks/domains/session/use-session-git-derived.test.ts hooks/domains/session/use-session-git-summary.test.ts lib/state/slices/session-runtime/git-status-state.test.ts lib/ws/handlers/git-status.test.ts lib/ws/client.test.ts components/review/review-diff-list-auto-mark.test.tsx components/task/changes-panel-body-context.test.tsx components/task/changes-panel-helpers.test.ts components/task/changes-panel-git-status.test.ts components/task/mobile/mobile-changes-panel.test.tsx components/task/task-changes-panel-layers.test.ts components/task/task-changes-panel.test.ts)`: passed (13 files, 156 tests).
- Follow-up affected frontend suite: passed (15 files, 164 tests); coordinator, relation hook, and refresh hook tests passed (3 files, 13 tests).
- `(cd apps/web && pnpm run typecheck)`, `(cd apps && pnpm --filter @kandev/web lint)`, `(cd apps && pnpm --filter @kandev/web build:vite)`, `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)`: passed.
- `(cd apps/web && pnpm e2e:run --host tests/task/add-workspace-sources.spec.ts -- --grep 'adds a local repository and folder successively' --retries=0)`: passed (1 test); the managed runner rebuilt backend, Vite assets, and the fixture plugin.
- Backend `golangci-lint` passed across the nine affected packages (0 issues); `(cd apps/backend && make build)` passed for host binaries and agentctl cross-build targets.
- Desktop and mobile recovery E2E each passed (1 test) through the managed runner, which rebuilt the production backend, Vite assets, and fixture plugin before each isolated browser run.
- Documentation validation passed: 334 decisions and 1262 specifications; specification lint, 62 public-doc tests, 47-page validation, and `git diff --check` passed.

## PR fixup CI follow-up

The pre-fix PR check wait reached its 45-minute deadline with 36 checks passed, 11 skipped, three failed, and 20 pending. The backend failures exposed two test assumptions that predated progressive status delivery: the recovered-base integration test expected comparison totals in the first fresh response, and the admission test counted background enrichment as part of the interactive basic read.

The recovered-base test now checks that the first response contains complete live membership, then uses the API's explicit `details=wait` mode to verify configured-base totals. The admission test gates enrichment and proves that a fresh basic response returns with pending details before any background Git command is admitted. These preserve the pending-then-ready contract; no production behavior or durable requirement changed.

- `(cd apps/backend && go test -race ./internal/agentctl/server/api -run '^TestRecoveredBaseBranches_FirstGitResponses$' -count=1)`: passed.
- `(cd apps/backend && go test -race ./internal/agentctl/server/process -run '^TestGetGitStatusFreshReturnsBeforeBackgroundEnrichment$' -count=1)`: passed.
- `(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`: passed; process 127.532s, API 58.943s.
- Fresh PR checks are required after this fixup push. The prior wait did not establish a terminal result for the pending E2E and frontend checks.
