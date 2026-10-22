---
id: "02-harden-bootstrap"
title: "Harden OpenCode bootstrap admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OPENCODE-AUTH-001
acceptance_criteria:
  - AC-EXECUTORS-OPENCODE-AUTH-001.1
  - AC-EXECUTORS-OPENCODE-AUTH-001.2
  - AC-EXECUTORS-OPENCODE-AUTH-001.3
  - AC-EXECUTORS-OPENCODE-AUTH-001.4
  - AC-EXECUTORS-OPENCODE-AUTH-001.5
  - AC-EXECUTORS-OPENCODE-AUTH-001.6
  - AC-EXECUTORS-OPENCODE-AUTH-001.7
  - AC-EXECUTORS-OPENCODE-AUTH-001.8
  - AC-EXECUTORS-OPENCODE-AUTH-001.9
  - AC-EXECUTORS-OPENCODE-AUTH-001.10
  - AC-EXECUTORS-OPENCODE-AUTH-001.11
system_design:
  - ../../specs/executors/system-design/opencode-v2-credentials.md
---

# Task 02: Harden OpenCode bootstrap admission

## Summary

Implement one compatible controller/worker bootstrap protocol using resolved
runtime metadata and destination-owned credentials. This is a single vertical
slice because runtime selection, capability discovery, and pending-state
admission must agree before a launch can safely skip source credentials.

## In scope

- Typed family/source/version propagated through normal launch and Kubernetes
  refresh, configuration DTOs, and process state. Preserve upstream wrappers.
- Named worker capability and authenticated redacted destination-status API.
  Check compatibility before any host export; reject unsupported sources.
- Explicit source environment snapshot, final destination environment, bounded
  commands, cancellation, redaction, and process admission serialization.
- Empty/present/pending bootstrap state and private store-scoped pending marker.
  Retry partial import; skip export only for completed or pre-existing stores.
- Session-local login/account-switching instructions in `docs/public/executors.md`.
- Four existing lint findings and meaningful regression tests for changed logic.

## Out of scope

Live provider testing belongs to Task 03. No broker, SQLite inspection, automatic
credential replacement, or deployment-workaround retirement.

## Acceptance

1. Runtime/capability/environment gates pass before source export; unsupported
   native/custom sources and old workers fail explicitly. V1, standalone,
   unselected methods, and structured wrappers preserve their contracts.
2. A present destination resumes with no controller export. Empty stores import;
   pending imports require successful retry. Unreadable stores, marker failures,
   concurrent configure/start, and malformed responses cannot admit ACP.
3. Refreshed values/selections survive retry; source-path isolation and redaction
   tests pass; recovery documentation and affected-package checks pass.

## Verification

From repository root:

```sh
(cd apps/backend && env LC_ALL=C LANG=C go test -trimpath ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/remoteauth ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/api)
(cd apps/backend && env LC_ALL=C LANG=C go test -race -trimpath ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/remoteauth ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'OpenCode|Opencode|Credential|Identity|LaunchWrapper' -count=1)
(cd apps/backend && go build -trimpath ./cmd/kandev ./cmd/agentctl)
bash scripts/lint-go-changed $(git diff --name-only upstream/main -- '*.go') $(git ls-files --others --exclude-standard -- '*.go')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

New regressions (planned names): `TestOpenCodeCredentialRuntimeMetadata`,
`TestOpenCodeCredentialCapabilityBeforeExport`,
`TestOpenCodeCredentialSourceEnvironment`,
`TestOpenCodeCredentialRestartWithoutSource`,
`TestOpenCodeCredentialPendingRetry`, and
`TestOpenCodeCredentialLostAcknowledgement`. Put each in its owning existing
credential test file. Include negative cases, partial import, marker I/O failure,
store-path change, stop/cancel, and configuration/start races.

## Files likely touched

- `apps/backend/internal/agent/managedruntime/opencode_credentials.go` and tests
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/opencode_credentials*.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_credentials_test.go`
- `apps/backend/internal/agent/runtime/agentctl/opencode_credentials*.go` and configure DTOs
- `apps/backend/internal/agentctl/server/api/identity.go`, `server.go`, and configure handlers
- `apps/backend/internal/agentctl/server/api/opencode_credentials*.go`
- `apps/backend/internal/agentctl/server/config/` and `server/process/` configuration and credential guards
- `docs/public/executors.md`

## Dependencies

Completed initial Task 01. Use the revised requirement/design and
[bootstrap decision](../../decisions/2026-10-30-opencode-bootstrap-ownership.md).

## Risks

Upstream now carries structured wrappers separately from ACP argv. Read the
current launch path rather than replaying the old prefix reconstruction. A
pending marker must survive worker interruption in the same credential-store
lifetime and cannot expose secrets. Existing unrelated package failures must
be distinguished from new regressions.

## Parallelism

`sequential`

## Inputs

The paired requirement/design, current launch-wrapper capability checks, CLI
2.1.2 export/import source, existing credential tests, and scoped AGENTS.md files.

## Results

Implementation and automated verification complete. Earlier failed runs below
are historical; the final serial suites passed. Live evidence belongs to Task 03.

- Typed runtime metadata now reaches normal and Kubernetes configure paths;
  capability checks precede controller export. Managed npm recipes are shared.
- Destination readiness skips source access for existing accounts. Interrupted
  imports retain a secret-free, synced marker beside the CLI-reported database;
  completed-import retries preserve destination tokens and selections.
- Controller export uses its startup environment snapshot. Destination override,
  invalid marker, partial retry, lost acknowledgement, and malformed readiness
  regressions use isolated child processes with synthetic credentials.
- Changed-file lint passed with zero issues; backend/agentctl build passed.
  The seven-package targeted race run passed 564 tests. Documentation checks
  passed (36 spec-validator tests, 63 public-doc tests, 54 public pages).
- The initial full suite reported 5,445 passes, 19 skips and three failures:
  process/lifecycle package timeouts and a VS Code cleanup timeout in the API
  package. The serial GOMAXPROCS=4 rerun also timed out in all three packages;
  these are not waived. Its API stack was in bbolt fdatasync during
  TestAgentStreamReplayMultiplePages. A temporary-file probe measured 4.08 seconds
  for 20 fsync calls. The extended process run also timed out at 30 minutes,
  in TestIncrementalShellSlowStorageBound with bbolt fdatasync on the stack.
  Isolating that test with memory-backed temporary data still exceeded three
  minutes, this time in shell-output SHA-256 digest computation; storage alone
  therefore does not explain the unresolved verification failure. The extended
  serial run completed: API passed in 63.840 seconds; lifecycle finished in
  984.248 seconds with one failure, the two-second watchdog deadline in
  TestSSHKeepaliveDeadlineTearsDownTrackedSessionWithinBoundedTime. That exact
  test passed its isolated rerun without code changes. This does not turn the
  failed package run into a pass. No full-suite pass is claimed.
- A real isolated CLI 2.1.2 `debug paths db` probe honored OPENCODE_DB. On this
  host npm left the package launcher stub installed; explicitly running that
  package's postinstall produced the executable used for this path-only probe.
  This is not lifecycle, import, or OAuth evidence.
- Latest malformed-entry readiness regression passed eight cases after the
  broader runs above; final checks must include this subsequent change.
- A subsequent broad race run timed out and exposed two credential fixtures
  whose ten-second startup budget now covered three CLI children. The isolated
  rerun reproduced the cancellation fixture's startup timeout. Both fixtures
  now allow one minute to reach their barrier; their five-second cancellation
  and one-second child-disconnection assertions remain unchanged. The isolated
  race rerun passed both tests. The broad run is not recorded as a pass.
- The subsequent API-only race selection reported 126 passes and one failure
  in TestShellTerminalStreamTerminatesWhenCredentialRotates terminal cleanup.
  This remains unresolved rather than being classified as unrelated.

- Additional compatibility preflight over pinned SSH into the disposable test
  container passed with real CLI versions 2.1.2 and 2.2.1: stdin import of a
  synthetic account, export, then duplicate-ID import with different value and
  active selection preserved the original export exactly. Each version used a
  fresh isolated HOME/XDG directory, removed afterward. No real credentials were
  accessed. This verifies the CLI merge contract only, not the controller
  lifecycle, interrupted imports, or OAuth refresh required by Task 03.

- Latest focused race run passed 59 tests across managedruntime, agentctl client,
  lifecycle, and worker API packages using the credential-specific test names.
  This does not replace the broader required checks.
- An isolated archive of unmodified upstream/main passed
  TestIncrementalShellSlowStorageBound with the same three-minute timeout. The
  matching current-branch comparison also passed. The earlier full-package
  timeout remains unresolved; neither comparison establishes its cause.
- TestShellTerminalStreamTerminatesWhenCredentialRotates passed an isolated
  race rerun without code changes. The process-package full run without
  competing test jobs passed in 187.783 seconds (45-minute limit). The serial
  full run of the remaining six required packages also passed: agents 6.812s,
  managedruntime 0.025s, remoteauth 0.009s, agentctl client 5.857s, lifecycle
  377.804s, and worker API 56.711s. All seven required full package suites have
  therefore passed on the current changes. Final broad race verification also
  passed all seven packages, run serially with GOMAXPROCS=4 and a 30-minute
  per-package limit. Final build, changed-file lint, documentation validators
  and their tests, and diff whitespace checks all passed.

- Live validation subsequently exposed the ordinary HTTP client's shorter
  timeout overriding the credential-operation deadline. Status and import now
  use the existing long-running client with a two-minute context. A regression
  failed for both operations before the fix. Afterward, client/lifecycle focused
  race tests, changed-file lint, spec validation, and diff checks passed; the
  rebuilt controller passed real bootstrap and no-source restart on all three
  executor types at CLI 2.1.2 and 2.2.1.

No provider-backed OAuth validation, deployment, commit, push, or issue comment performed.
