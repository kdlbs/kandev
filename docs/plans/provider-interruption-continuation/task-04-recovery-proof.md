---
id: "04-recovery-proof"
title: "Prove isolated desktop and phone recovery"
status: completed
wave: 4
depends_on:
  - "03-recovery-feedback"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 04: Prove isolated desktop and phone recovery

## Summary

Use controlled provider frames and deterministic restore outcomes to prove the
complete user flow. Assert dispatched prompt identity and persisted state as
well as recovery UI, so a success label cannot conceal original-prompt replay.

## In scope

- Leave a finite 60-minute Windows CI job budget for cold setup, tests, helper
  regressions, and cache publication; retain the 25-minute package deadline.
- Required-CI remediation of the existing recovered-base-branch API test:
  details waiters accept their own completed enrichment without treating its
  publication revision as a superseding basic observation. Preserve actual
  replacement, cancellation, and unavailable-detail fences.
- Mock-only typed support using existing mock dialect provenance. Add named
  scenarios for output-only, completed read, pending read, write, unknown work,
  resumed progress before settlement, transient/hard restore failure, and
  ambiguous continuation acceptance. Prove missing native identity through
  real repository/lifecycle tests rather than a mock browser metadata override.
- Persist mock episode counters/accepted-prompt markers across owned process
  restarts and clean them in CloseSession/E2E reset, like the overload fixture.
  Original user prompt and continuation must be distinguishable without
  exposing real prompts or provider credentials.
- Shared bounded E2E helper for seeding and state evidence; feature enablement
  via `backend.restart(overrides)` and baseline restore, not inherited env.
- Desktop/phone scenarios from the plan's matrix; reload, two viewers, actual
  cancellation after admission, queued human prompt priority, backend restart,
  and live-adoption negative case.
- Keep `/transport-lost` data-only manual recovery and existing overload replay
  tests intact. Compare rendered UI-01/UI-02 structure, phone 44px controls,
  expanded technical details, and no document overflow.

## Out of scope

Live-user-session reproduction, real network outages, Docker daemon scenarios,
new provider signatures, mock support accepted by production adapters, and
generic full-suite verification.

## Acceptance

- Fixtures prove restoration of the same conversation, exactly one admitted
  continuation, no original-prompt resend, partial history preservation, and
  no successful workflow event from the interrupted turn. Unsafe cases are manual.
- Real cancellation, supersession, transient restore failure, reload/multi-viewer,
  and restart/adoption outcomes match the backend episode contract; persisted
  evidence agrees with visible state and notices disappear after successful cleanup.
- Both viewport projects discover and pass their focused tests; phone controls
  measure at least 44px and recovery/details fit without horizontal overflow.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -race ./cmd/mock-agent ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/backendapp -run 'Test(MockInterruptionContinuation|CursorContinuationEvidence|ContinuationNativeOnlyRestore)' -count=1)
(cd apps/web && pnpm e2e:run --docker --project chromium tests/session/provider-interruption-continuation.spec.ts tests/session/transient-retry.spec.ts tests/session/transient-retry-transport-lost.spec.ts)
(cd apps/web && pnpm e2e:run --docker --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts tests/session/mobile-transient-retry.spec.ts)
```

Run project commands sequentially. Managed runner rebuilds production assets and
host mock/backend; do not reuse stale `--no-build` artifacts or pass all-worker
overrides. Record discovered counts and results. Use causal observations and
API polling; time-based assertions are not evidence of a dispatch outcome.

## Files likely touched

- `.github/workflows/backend-tests.yml` and its workflow contract test
- `apps/backend/internal/agentctl/server/process/workspace_git_status*.go`
  and `workspace_tracker.go` for the required-CI details-wait handoff repair
- `apps/backend/cmd/mock-agent/{handler,scenarios,emitter}.go` and focused tests
- Mock-only ACP dialect hook and tests
- `apps/web/e2e/helpers/provider-interruption-continuation.ts` (new)
- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts` (new)
- `apps/web/e2e/pages/session-page.ts` only for reusable scoped selectors
- Mock reset/close fixture cleanup where counters are owned

## Dependencies

Tasks 01-03 complete. The native proof from Task 01 remains a distinct support
prerequisite; deterministic mocked restore is not its replacement.

## Risks

A mock that takes a separate recovery route will miss production wiring. Route
it through the same typed snapshot and admission logic with mock-only provenance.
Cleanup omissions can contaminate another worker-scoped session.

## Parallelism

`sequential`

## Inputs

Plan E2E matrix and UI-01/UI-02; design Validation strategy; e2e and mobile-parity
skills; mock-agent AGENTS.md and current transient-retry helpers/specs.

## Results

Added mock-only attested RPC errors through the production safety ledger and
persisted same-ID output/read/write/pending/unknown scenarios. Mock and ACP wire
race tests pass. The desktop suite now has 14 continuation checks, alongside
six existing replay/copy regressions. Sequential Docker runs cover the matrix;
the final seven-case run rebuilt the latest source and passed every affected
acceptance/startup/restart path. The existing first-turn overload regression
also passed its isolated rerun after a timeout in the longer matrix run.

Rendered evidence checks same native ID, one original prompt, one continuation,
filtered restored history, successful turn settlement, transient and hard
restore failure, ambiguous acceptance, queued human work, waiting and accepted
cancellation, reload, two viewers, and restart with and without surviving work.
Missing native identity is proven at the real repository/lifecycle boundary,
not through an artificial browser metadata override. Earlier host Chromium
launches failed before application entry; all rendered verification uses the
managed Docker runner with one worker. Phone results are recorded in the plan.

Integration failures produced compiling regressions before production fixes:
startup error ownership, stopped-executor grace, accepted context lifetime,
shutdown notice persistence, and finishing startup ownership at acceptance.
Scoped race tests and final desktop integration checks pass after these fixes.

PR review remediation covers fixture prefix dispatch, cancellation, episode cleanup, setup-error preservation, and failed native-restore worktree release. Focused race tests pass. The real checkout-release regression passes in Linux Docker; macOS sandbox checkout restrictions prevented reaching that boundary on the host.

CI static-check remediation extracts the existing execution-absence predicate
and gives the mock output scenario its own constant, preserving behavior.
Focused orchestrator/mock race regressions pass. The full backend CI command
passes on macOS with zero issues:

```bash
# From apps/backend; use the authoritative fetched base.
golangci-lint run ./... --new-from-rev=8403b464b42719d4f0357c9376a33a11a88f7416 --timeout=10m
```

The Linux attempt ended on Docker storage I/O errors; the first host attempt
exhausted temporary space. Resetting this task's generated Go cache recovered
about 50GB, and the host rerun passed. Shared Docker data was not altered.
The UI is unchanged by this cleanup, so the screenshots from the UI fixup
commit remain representative.

The Windows job reached its 40-minute workflow limit on two successive heads.
The first run had completed every test and was saving its cache; the second
had passed the core race suite but was interrupted during helper regressions.
The job budget is now 60 minutes, while the 25-minute Go package deadline stays
unchanged. Its existing ten-test workflow contract fails against the former
budget and passes against the updated workflow.

A broad host process-package attempt exposed the unchanged parent-process
inspection test returning `-1` in this sandbox, and then exceeded the default
ten-minute package-total deadline during its third repetition. An isolated
parent-process test reproduces the same host restriction. These attempts are
failed evidence, not successful full-package validation. CI's process-package
race tests passed on the preceding head; focused Git-status validation uses
explicit package-total deadlines and excludes unrelated process inspection.

The recovered-base-branch API failure exposed a details-wait handoff race:
a ready enrichment publication can advance the revision before the caller
receives its accepted basic observation. The channel-controlled regression
fails against the former implementation, then passes for both that completion
and rejection of a superseding observation. Ready publications retain their
basic source revision; selecting the current result and pending job is atomic
under the enrichment lock.

An earlier host stress run also returned a completed but unavailable detail
result. That is a separate quality-failure path, and the exact command cause
was not reproduced. Temporary tracing runs passed 40 and then 100 repetitions
of the recovered-base-branch API test, the full API package three times with
the race detector, and the workspace tracker suite. These results do not
prove the earlier unavailable result fixed. Temporary tracing was removed
before final production-source verification. Unavailable results remain
errors and are not accepted by the new handoff rule.

Final checks after tracing removal pass: the recovered-base-branch API race
regression (with atomic coverage), the complete selected workspace tracker
race suite, all ten workflow contract tests, documentation catalog/spec lint,
and six-work-order PR coverage. The backend changed-line lint reports zero
issues against fetched main `a1669c0a5584d390d210b9d5570df81c41d3936e`.

```bash
# From apps/backend. The process invocation disables only the host Git ignore
# file through an invocation-scoped Git wrapper.
go test -race -v -covermode=atomic ./internal/agentctl/server/api -run '^TestRecoveredBaseBranches_FirstGitResponses$' -count=1 -timeout=5m
go test -race -v ./internal/agentctl/server/process -run 'Test(WorkspaceTracker|GitStatus)' -count=1 -timeout=20m
```

These checks validate the handoff repair, not a physical network transition.
Current-head CI and review results must still be refreshed after publication.

2026-10-07 required-CI discovery repair: the disabled-continuation desktop case
contained two declarations of the same `session` variable in one scope.
The duplicate navigation/declaration was also present on merged main. Removed
only the redundant second navigation; all metadata, native-no-replay and
manual-recovery UI assertions remain. The exact CI catalog command failed
before this edit and passed afterward:
`pnpm --dir apps/web exec tsx e2e/scripts/plan-shards.ts --web-root
"$PWD/apps/web" --output-dir <owned-cache-directory>`.
The failure occurred during shard discovery after all product builds passed.
The focused managed browser scenario passed its first attempt with retries
disabled after a fresh host build:
`pnpm --dir apps/web e2e:run --host --project chromium --
e2e/tests/session/provider-interruption-continuation.spec.ts
--grep "disabled continuation" --retries=0 --trace=retain-on-failure`.
Catalog/spec lint, focused browser ESLint and the 59-work-order coverage
preflight passed. Pushed-head CI remains an external pending gate.
