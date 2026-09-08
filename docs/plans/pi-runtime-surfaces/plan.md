---
spec: docs/specs/cli/requirements/cli-mode-parity.md
created: 2026-08-16
status: complete
---

# Implementation Plan: Separate Pi Runtime Surfaces

## Overview

Pi's built-in definition assigns `npx -y pi-acp` to both structured ACP
execution and interactive passthrough. Registry metadata confirms that
`pi-acp` publishes the ACP adapter binary, while
`@earendil-works/pi-coding-agent` publishes the interactive `pi` binary. The
repair keeps the ACP adapter on structured and inference paths, launches `pi`
for passthrough, and makes installation and discovery agree on that executable.

The durable managed-runtime boundary is recorded in
[`docs/decisions/2026-08-12-validated-managed-runtime-version-selection.md`](../../decisions/2026-08-12-validated-managed-runtime-version-selection.md),
and the observable behavior is specified in
[`docs/specs/cli/requirements/cli-mode-parity.md`](../../specs/cli/requirements/cli-mode-parity.md) and
[`docs/specs/agents/requirements/runtime-updates.md`](../../specs/agents/requirements/runtime-updates.md).

---

## Backend

### Pi agent definition

- Update `apps/backend/internal/agent/agents/pi_acp.go` so
  `BuildCommand`, `Runtime().Cmd`, and `InferenceConfig().Command` remain
  `npx -y pi-acp`.
- Change `PassthroughConfig.PassthroughCmd` to the globally installed `pi`
  executable. Do not route the passthrough path through managed ACP package or
  version resolution.
- Make `IsInstalled` probe `pi`, the executable provisioned for users and
  required by passthrough, with a non-interactive `--version` check instead of
  accepting an adapter-only `pi-acp` installation or an unrelated `pi`
  executable that cannot satisfy the full integration.
- Change `InstallScript` to the supplied safe install recipe:
  `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`.

### Public documentation

- Update `docs/public/agents-and-profiles.md` to identify `pi` as Pi's detected
  and passthrough executable, document the exact install command, and explain
  that structured sessions continue through `pi-acp`.
- Clarify the Pi entry in `README.md` so the supported-agent table does not
  imply that the ACP adapter is also the interactive CLI.

## Tests

- **What:** Structured Pi chat and inference remain on `npx -y pi-acp`, while
  `PassthroughCmd` is exactly `pi`; the install recipe and detection binary
  match the interactive package.
  **File:** `apps/backend/internal/agent/agents/new_acp_agents_test.go`.
  **How:** Change the Pi row in the shared command-surface contract before the
  production edit. Confirm the command, installation, and detection tests fail
  for the reported mismatch, then pass after the repair.
- **What:** The lifecycle's real Pi passthrough resolution, including MCP
  project-file materialization, returns `pi` rather than the ACP adapter.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`.
  **How:** Extend `TestPassthroughPiWritesProjectFile` to assert the resolved
  command argv as well as the existing `.pi/mcp.json` contract. This covers the
  registry-to-lifecycle passthrough path where a future managed-runtime change
  could otherwise reintroduce the conflation.

No browser E2E is needed because the settings UI and passthrough terminal UI do
not change; the behavior is the backend command selected behind the existing
profile mode.

## Verification Results

- Red: the focused Go contract run failed on Pi's old passthrough command and
  install script, as expected.
- Green: the focused Go command, including the `pi --version` collision
  regression, passed with 43 tests across the agents and lifecycle packages.
- PR fixup: the detection probe now uses `WithCommandCheck("pi", "--version")`
  and restores the rationale comment for the remaining binary-name tradeoff.
- Public docs: `node --test scripts/validate-public-docs.test.mjs` passed all
  61 tests, `node scripts/validate-public-docs.mjs` validated 41 pages, and
  `git diff --check` passed.

## Implementation Tasks

- [x] [task-01-correct-pi-execution-surfaces](task-01-correct-pi-execution-surfaces.md)

Execution is sequential in the primary conversation. There are no parallel
candidates for this single vertical repair.
