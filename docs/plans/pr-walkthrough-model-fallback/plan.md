---
created: 2026-10-28
status: done
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
legacy_specs: []
---

# Implementation Plan: PR walkthrough model fallback

## Overview

Discover free `opencode` models at generation time, retry recognized quota,
rate-limit, or availability failures with the next candidate, and use the
requested Go model as a final option. One work order covers discovery,
supervision, and workflow wiring together because those boundaries must work
end to end.

## Evidence and root cause

PR #4795's Actions run received a free-usage-limit error for
`opencode/deepseek-v4.1-flash-free`; OpenCode remained alive until the
600-second supervisor deadline. The workflow passes one fixed model and the
runner retries only an incomplete zero exit. Local OpenCode 1.18.5 probes of
DeepSeek, GLM, Big Pickle, and MiMo free models each returned `OK`; they used
local credentials and do not establish the quota scope of the Actions key.
The pinned workflow CLI is the compatibility target for discovery and errors.

## Scope

### In scope

- Discover current free, tool-capable text models from the `opencode` provider
  using refreshed CLI metadata.
- Advance through candidates on recognized quota, rate-limit, or availability
  errors and use Muse Spark 1.3 Contributor after free candidates are exhausted.
- Preserve the existing bounded completion, trust, and publication contracts.
- Record model identities and outcomes without credentials in diagnostics.

### Out of scope

- Changes to rendering, publishing, PR eligibility, or description linking.
- Automatic paid choices other than the requested Go model.
- Assuming that different free models have independent quota limits.

## Technical approach

Extend `.github/scripts/pr-walkthrough-runner.py` and the trusted workflow.
Discover the CLI's refreshed verbose catalog, restrict free rotation to the
`opencode` provider, and parse each model's explicit price and capabilities;
validate IDs and use deterministic ordering. Retain other provider metadata only
for the explicit Go fallback. Supervise each model with clean per-attempt state
and detect attributable, recoverable model failures while a process remains live.
Remove the agent's hardcoded model and avoid passing an unsupported variant.
Route `OPENCODE_API_KEY` to Go using the pinned CLI's confirmed provider
mechanism. Respect the shared 600-second deadline, existing completion
verification and cancellation semantics. The model data-use change is
documented in the linked decision.

## Tests

The runner regression in `.github/scripts/pr-walkthrough-runner_test.py`
must first fail against the current single-model implementation: a fake
OpenCode process emits a quota or rate-limit error and stays alive, then a
second candidate renders a verified output. Cover catalog parsing, free prices
and capabilities, provider restriction, dynamic additions and removals, empty
and failed discovery, paid-last ordering, credentials, provider-specific errors,
false positives, deadlines, cleanup, cancellation, and verification failure.
Update `.github/scripts/pr-walkthrough-workflow-contract_test.py` for workflow
wiring, trusted provenance, and diagnostics. The tests exercise the real
renderer and verifier with fake CLI processes; no paid API is required.

## End-to-end evidence

Run the focused runner and workflow contract suites. A later eligible GitHub
Actions run is needed to confirm the Actions secret, provider quota behavior,
Go subscription, and public delivery; local credentials cannot prove those.

## Work orders

- [x] [Task 01: Add dynamic model fallback](task-01-model-fallback.md)

## Verification results

- `python3 .github/scripts/pr-walkthrough-runner_test.py`: 45 tests pass.
  The catalog, provider restriction, discovery, rotation, rate-limit fallback,
  attribution, deadline, cleanup, cancellation, and verification cases are
  covered.
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py`: 31 tests
  pass.
- `python3 scripts/list-docs.py validate`: 410 decisions and 1767
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: clean.
- The remaining PR walkthrough suite passes: history, PR body, context,
  renderer, and verifier tests.
- Local probes of the pinned OpenCode CLI (v1.19.3, archive SHA verified)
  returned 111 model records, 9 eligible `opencode` free candidates, and the
  requested Go model metadata. A dummy `OPENCODE_API_KEY` made the Go provider
  appear and produced an invalid-credential error, which confirms credential
  routing without a paid call.
- A later eligible GitHub Actions run must confirm the Actions secret,
  provider quota rotation, Go subscription access, and public delivery. Local
  credentials cannot prove those.

## Risks

- Some free models and the Contributor model can use PR context for training or
  retain it; the existing zero-retention policy is deliberately changed.
- CLI catalog and error formats can change; malformed or stale discovery must
  fail safely. A shared free quota or rate limit can defeat rotation.
- The paid model may be unavailable in the runner region or its secret may not
  authorize Go. The deadline can expire before all candidates are tried.
