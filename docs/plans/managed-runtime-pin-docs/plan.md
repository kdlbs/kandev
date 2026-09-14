---
created: 2026-09-11
status: complete
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
legacy_specs: []
---

# Implementation Plan: Remove duplicated runtime defaults

## Overview

Replace current-version claims in three documents with links to the runtime
catalogue. Preserve exact versions that identify historical experiments.
One sequential work order completes this documentation repair.

## Evidence and root cause

[PR 3606](https://github.com/kdlbs/kandev/pull/3606) changes only
`apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`.
Its [review comment](https://github.com/kdlbs/kandev/pull/3606#discussion_r3989535885)
identifies stale defaults in three documents.

The updater writes the JSON catalogue. The workflow stages only that file.
Separate prose and a table duplicate the current defaults without an update
mechanism. Each version change can therefore leave those claims stale.

The read-only reproduction compares the PR catalogue with these claims:

- `ACP_BRIDGE_VERSIONS.md` lists six default versions and a concrete Claude
  launch example as current.
- ADR 0034 calls Codex `1.10.0` the current default.
- The timeout design calls Claude `0.75.1` the default at this commit.
  That same version also identifies the binary used in historical experiments.

## Requirement conformance

The agents system owns the managed runtime catalogue and update lifecycle.
[REQ-AGENTS-RUNTIME-UPDATES-001](../../specs/agents/requirements/runtime-updates.md)
already defines exact defaults (`AC-AGENTS-RUNTIME-UPDATES-001.5`) and the grouped
maintenance PR (`AC-AGENTS-RUNTIME-UPDATES-001.9`). The implementation follows
those criteria. This repair corrects descriptions of that implementation.

No new product requirement or ADR is necessary. The existing
[system design](../../specs/agents/system-design/runtime-updates-01.md#scheduled-pin-update-workflow)
continues to describe the updater. The
[token decision](../../decisions/2026-09-04-use-repository-token-for-runtime-pin-prs.md)
continues to govern workflow authorization.

The related packages are `managed-runtime-version-awareness` and
`managed-runtime-pin-workflow-auth`. This package does not reopen their runtime
or workflow work orders or replace their recorded results.

## Scope

### In scope

- Link current defaults directly to the JSON catalogue in all three documents.
- Preserve the agent names, package names, ACP arguments, and effective-version semantics.
- Identify the timeout experiments as historical evidence for their exact tested binary.

### Out of scope

- Version bumps, runtime code, updater code, or workflow changes.
- Generated documentation tables or a general documentation scanner.
- New timeout experiments or changes to their recorded results.
- UI changes, browser tests, and public website documentation.
- Pushing, merging, rerunning PR 3606, or replying to its review thread during implementation.

## Technical approach

In `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`, remove the
default-version column. Link to the adjacent JSON catalogue for current
defaults. Use `@<effective-version>` in the Claude command example and explain
that the placeholder represents the resolved exact version.

In `docs/decisions/0034-agentclientprotocol-codex-acp.md`, replace the numeric
current-default claim with a relative catalogue link. Preserve the decision,
package identity, and operator-selection semantics.

In `docs/specs/agents/system-design/mcp-timeout-budgets.md`, keep `0.75.1`,
`0.3.257`, the binary path, and the experiment results. Remove the claim that
the experiment version is the current default. Link current defaults to the
catalogue and state that the evidence concerns the tested version.

The workflow requires no change. After this repair reaches `main`, its next
run starts from documentation that does not duplicate current version values.
A fix applied only to the automation branch can disappear because the workflow
recreates that branch from `origin/main`.

## Tests

The regression check is the **current-default documentation audit** in Task 01.
Before the correction, it finds the three current-version claims. After the
correction, those claims are absent and all three catalogue links resolve.
This documentation-only repair uses direct inspection instead of permanent
tests that assert prose wording.

Existing updater fixtures cover changed and unchanged catalogues for
`AC-AGENTS-RUNTIME-UPDATES-001.9`. The existing workflow contract suite covers
the grouped PR and validation sequence. These checks supplement the document
audit without changing their test files.

No E2E test is necessary because runtime and rendered UI behavior do not change.

## Work orders

- [x] [Task 01: Remove duplicated runtime defaults](task-01-remove-duplicated-defaults.md)

## Verification results

Package and implementation validation on 2026-09-11:

- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/update-agent-runtime-pins.test.mjs`: 7 tests passed.
- `python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py`: 9 tests passed.
- The current-default documentation audit passed after the three corrections.
- All three catalogue links resolve to
  `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`.
- `make -C apps/backend build`: passed. The build reported non-fatal missing
  `codesign`/`rcodesign` warnings for Darwin artifacts.
- `git diff --check`: passed.
- `git status --short`: contains only the three target documents and this plan package.
- Requirement IDs, acceptance IDs, and the system-design path exist.

## Risks

- A broad version replacement can falsely imply that newer runtimes passed old experiments.
- Relative links must resolve from each document, including the deeply nested timeout design.
- This repair removes the known recurring cause. It cannot guarantee that future reviews contain no other findings.
