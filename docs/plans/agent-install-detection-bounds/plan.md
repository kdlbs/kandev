---
created: 2026-10-12
status: complete
requirements:
  - REQ-AGENTS-INSTALL-DETECTION-001
system_design:
  - ../../specs/agents/system-design/agent-install-detection-bounds.md
legacy_specs: []
---

# Implementation Plan: Agent install detection bounds

## Overview

Stop a healthy agent from being reported as not installed when a concurrent
discovery sweep stretches its capability check past the shared bound, without
slowing every other agent and without re-running conclusive failures.

## Confirmed defect

`HermesACP.IsInstalled` used `WithCommandCheck`, which bounds the check at the
shared `commandCheckTimeout` of 5s and returns the same unavailable result for
an elapsed bound as for a nonzero exit. Discovery runs every enabled agent's
`IsInstalled` concurrently, so on the reporting host the check took 5.5s to
12.1s at sweep width while exiting 0, and Hermes was reported as not installed
on every sweep. Other agents detected by the same backend were reported
available, so the failure was specific to the Hermes check rather than to PATH
or the sweep.

A synthetic `hermes` binary that stalls 6s before exiting 0 reproduces it: the
check is reported unavailable after 5s, the same result as a binary that exits 1.

## Scope

### In scope

- Distinguish an elapsed bound from a nonzero exit in the shared bounded check.
- One staged measurement for Hermes, whose check is measurably slower than the
  shared bound.
- Focused unit tests for both, including a Windows-safe test helper.
- Start host CLI version checks alongside pending installation checks and join
  them before publishing partial results.
- Deterministic cancellation and parent deadline during the second check, plus
  bounded native Windows execution and a workflow-selection contract.

### Out of scope

- The discovery budget, installation-probe concurrency, and cache policy.
- Any other agent's detection path or bound.
- New configuration, settings, or an exported API.
- Frontend and public documentation changes.

## Technical approach

Factor the body of `WithCommandCheck` into `runCommandCheck`, which also
reports whether the bound elapsed. `WithCommandCheck` keeps its exact behavior
on top of it, so no other agent changes. Add `withHermesACPCheck`, which
measures once more at a larger bound only when the first bound elapsed.
Production bounds are 5s then 8s, summing to 13s inside the 15s sweep budget.
Version checks start when each indexed installation result arrives, retaining
the existing version timeout and process semaphore. Started version workers
finish before the response or cache publication.

## Work packages

| Work order | Scope | Status |
| --- | --- | --- |
| [task-01](task-01-hermes-acp-check-retry.md) | Bounded-check discrimination and the Hermes staged measurement | done |

## Validation

- `go test ./internal/agent/agents/ -run 'TestHermesACP|TestRunCommandCheck|TestWithNpxRunnable' -count=1`
- `go build ./...` and `go vet ./internal/agent/agents/`
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
- `python3 .github/scripts/backend-tests-workflow-contract_test.py`
- Native Windows hosted execution:
  `go test -trimpath -tags fts5 -race -p=1 -count=1 -v -timeout 5m ./internal/agent/agents -run '^Test(HermesACP|RunCommandCheck)'`
- Discovery concurrency and cancellation:
  `go test -race ./internal/agent/discovery -run '^TestDetectAll(ChecksHostCLIWhileAnotherAgentIsInstalling|RetainsAvailabilityWhenHostCLIIsCancelled)$' -count=1`

## Review validation

PR #4429 adds deterministic second-check cancellation and deadline coverage,
preserves real subprocess tests, and makes Hermes regression execution part of
the required native Windows job. Local validation uses a contained, stdlib-only
exact-source proof with an explicit discovery DTO fake and the workflow
contract. Full package, build, vet and suite results are hosted CI evidence;
the local proof does not stand in for those results.
