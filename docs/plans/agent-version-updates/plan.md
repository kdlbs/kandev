---
decision: docs/decisions/2026-07-25-scheduled-core-agent-version-pins.md
created: 2026-07-25
status: completed
---

# Implementation Plan: Weekly Core Agent Version Updates

## Overview

This is an infra/workflow change, so the product-spec gate does not require a
feature spec. The implementation follows
[ADR-2026-07-25-scheduled-core-agent-version-pins](../../decisions/2026-07-25-scheduled-core-agent-version-pins.md).
It first makes all five core npm agent launch surfaces exact-versioned, then
adds a tested allowlisted updater, and finally schedules the updater to refresh
one review-only pull request.

## Backend

### Core runtime pins

- Add exact package/version/spec constants to
  `apps/backend/internal/agent/agents/copilot_acp.go` and
  `apps/backend/internal/agent/agents/gemini.go`, matching the existing Claude
  and Codex pattern.
- Use the exact package spec across `BuildCommand`, `Runtime`,
  `InferenceConfig`, passthrough configuration, and `InstallScript`.
- Advance Claude ACP, Codex ACP, and OpenCode to the stable npm versions
  selected when implementation starts.
- Extend agent command-surface tests so all five core agents prove their exact
  package specs on every launch/install path.

### Current-version documentation

- Expand `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md` into the
  maintained core-agent pin inventory and document Cursor's supported-channel
  exception.
- Update the current launch commands in `README.md`.
- Amend `docs/decisions/0034-agentclientprotocol-codex-acp.md` once so it links
  to the maintained pin inventory instead of asserting a forever-current
  numeric version.

## Automation

### Allowlisted updater

- Add `scripts/update_agent_versions.py`.
- Query npm metadata only; reject malformed or prerelease `latest` versions.
- Read each current version from its named Go constant.
- Preflight every allowlisted file and expected occurrence count before writing
  any replacement.
- Write a machine-readable and Markdown update report for the workflow.
- Make no changes when all pins are current.

### Scheduled pull request

- Add `.github/workflows/update-agent-versions.yml` with a weekly schedule and
  manual dispatch.
- Use a fixed concurrency group and an automation-owned branch.
- Mint a dedicated, least-privilege GitHub App token, run the updater and
  targeted tests, then create or refresh one conventional-commit pull request
  only when the worktree changed.
- Do not execute candidate npm packages in the token-bearing job and do not
  auto-merge the pull request.
- Pin every referenced GitHub Action to an immutable SHA.

## Tests

- **What:** Copilot and Gemini use exact package specs across all command and
  install surfaces.
  **File:** agent tests under `apps/backend/internal/agent/agents/`.
  **How:** table-driven command assertions against the selected versions.
- **What:** the updater changes only allowlisted occurrences and reports the
  old/new versions.
  **File:** `scripts/update_agent_versions_test.py`.
  **How:** standard-library unit tests with temporary fixture repositories and
  mocked registry results.
- **What:** malformed versions, prereleases, and source-shape drift fail closed.
  **File:** `scripts/update_agent_versions_test.py`.
  **How:** negative unit tests asserting no partial writes.
- **What:** the workflow retains its schedule, least-privilege App token,
  no-execution boundary, verification commands, fixed branch behavior, and
  SHA-pinned actions.
  **File:** `.github/scripts/update-agent-versions-workflow_test.py`.
  **How:** workflow contract assertions following existing CI workflow tests.

## Implementation Waves

Wave 1:

- [x] [Task 01: Pin core agent runtimes](task-01-pin-core-agent-runtimes.md)

Wave 2:

- [x] [Task 02: Build the allowlisted updater](task-02-build-version-updater.md)

Wave 3:

- [x] [Task 03: Schedule the update pull request](task-03-schedule-update-pr.md)

## Verification

- `cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle`
- `python3 scripts/update_agent_versions_test.py`
- `python3 .github/scripts/update-agent-versions-workflow_test.py`
- `python3 .github/scripts/lint-action-pinning_test.py`
- `python3 .github/scripts/lint-action-pinning.py`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- Commit through active hooks, then run delegated `/verify` in full mode before
  any push.

## Operational prerequisite

Configure a GitHub App with repository `contents: write` and
`pull_requests: write`, expose its ID as `AGENT_UPDATE_APP_ID`, and store its
private key as `AGENT_UPDATE_APP_PRIVATE_KEY`. The workflow can be merged
before those values exist, but scheduled PR creation will fail closed until
they are configured.
