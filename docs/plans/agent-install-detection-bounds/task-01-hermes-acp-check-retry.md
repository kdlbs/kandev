---
id: "01-hermes-acp-check-retry"
title: "Distinguish an elapsed detection bound from a failed check"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-INSTALL-DETECTION-001
acceptance_criteria:
  - AC-AGENTS-INSTALL-DETECTION-001.1
  - AC-AGENTS-INSTALL-DETECTION-001.2
  - AC-AGENTS-INSTALL-DETECTION-001.3
  - AC-AGENTS-INSTALL-DETECTION-001.4
  - AC-AGENTS-INSTALL-DETECTION-001.5
  - AC-AGENTS-INSTALL-DETECTION-001.6
  - AC-AGENTS-INSTALL-DETECTION-001.7
system_design:
  - ../../specs/agents/system-design/agent-install-detection-bounds.md
legacy_specs: []
---

# Work order 01: Hermes ACP check retry

## Outcome

`HermesACP.IsInstalled` reports a healthy Hermes as installed even when a
concurrent discovery sweep pushes `hermes acp --check` past the shared bound,
while every conclusive failure keeps its single-attempt behavior.

## Changes

- `apps/backend/internal/agent/agents/detect.go`: extract
  `runCommandCheck(ctx, path, timeout, args...)`, which reports `found`,
  `timedOut`, and caller cancellation separately. `WithCommandCheck` delegates
  to it and keeps its contract.
- `apps/backend/internal/agent/agents/hermes_acp.go`: add
  `withHermesACPCheck(first, second)`, which measures once more only after the
  first bound elapsed, and wire `IsInstalled` to
  `withHermesACPCheck(commandCheckTimeout, hermesCheckRetryTimeout)` with an 8s
  second bound.
- `detect_test.go` and `hermes_acp_test.go`: tests for every bounded-check
  outcome and for the staged measurement, with invocation counts so "measured
  twice" and "measured once" are told apart.
- `discovery.go` and `host_cli_sweep_budget_test.go`: start host CLI version
  checks as indexed installation results arrive; join every started worker
  before returning partial results. Preserve cache policy and process bounds.
- `backend-tests.yml`, the backend Makefile, and the existing workflow contract:
  execute Hermes/detection regressions in the required native Windows suite.

`runCommandCheck` records whether context cancellation successfully interrupted
the process and inspects the joined process state before reporting an elapsed
bound. A deadline sampled after a conclusive nonzero exit remains unavailable
without a retry. The boundary regression uses real terminal process state and
an expired context rather than wall-clock scheduling.

## Acceptance criteria

- **AC-AGENTS-INSTALL-DETECTION-001.1:** `runCommandCheck` reports an elapsed
  bound as `timedOut` and a nonzero exit without it.
- **AC-AGENTS-INSTALL-DETECTION-001.2:** Only a caller cancellation returns an
  error; a missing binary, a nonzero exit, and an elapsed bound report
  unavailable without one.
- **AC-AGENTS-INSTALL-DETECTION-001.3:** A timed-out first measurement is
  followed by exactly one second measurement whose outcome is final.
- **AC-AGENTS-INSTALL-DETECTION-001.4:** A nonzero exit runs the check exactly
  once, and a missing binary does not run it.
- **AC-AGENTS-INSTALL-DETECTION-001.5:** The production bounds sum to 13s, inside
  the 15s discovery budget.
- **AC-AGENTS-INSTALL-DETECTION-001.6:** No other agent's detection code path or
  bound is modified.
- **AC-AGENTS-INSTALL-DETECTION-001.7:** No configuration, setting, or exported
  API is added.

## Test helper

The Hermes detection test helper copies the test binary instead of
hard-linking it. On Windows a hard link to the running image stays locked for
the lifetime of the test, which made the existing
`TestHermesACP_DetectionRequiresACPCheck` fail its temporary-directory cleanup.
The copy removes that failure and lets the staged cases run on Windows.

Second-check cancellation tests use scripted runner outcomes, channel barriers
and a fake-time parent deadline. They prove retry entry before cancellation,
exactly two attempts, the preserved caller error and cleanup before return.
The real subprocess outcome tests remain selected by the Windows step.

## Validation

- `go test ./internal/agent/agents/ -run 'TestHermesACP|TestRunCommandCheck|TestWithNpxRunnable' -count=1`
- `go build ./...` and `go vet ./internal/agent/agents/`
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
- `python3 .github/scripts/backend-tests-workflow-contract_test.py`
- Hosted native Windows:
  `go test -trimpath -tags fts5 -race -p=1 -count=1 -v -timeout 5m ./internal/agent/agents -run '^Test(HermesACP|RunCommandCheck)'`
- `go test -race ./internal/agent/discovery -run '^TestDetectAll(ChecksHostCLIWhileAnotherAgentIsInstalling|RetainsAvailabilityWhenHostCLIIsCancelled)$' -count=1`

## Review results

PR #4429 owns the review additions above. Its delivery evidence records the
contained exact-source proof, mutation checks, workflow-contract results and
actual hosted test logs at the final candidate SHA. No broad package build or
test is inferred from the local DTO-faked proof.
