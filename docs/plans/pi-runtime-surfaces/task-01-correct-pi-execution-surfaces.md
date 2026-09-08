---
id: "01-correct-pi-execution-surfaces"
title: "Correct Pi execution surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/cli/requirements/cli-mode-parity.md"
---

# Task 01: Correct Pi Execution Surfaces

## Intent

Keep Pi's ACP JSON-RPC adapter on structured execution surfaces while launching
the separately distributed interactive `pi` CLI for passthrough. Align host
installation and discovery with the executable users actually run.

## Inputs

- `docs/specs/cli/requirements/cli-mode-parity.md`, sections **ACP and passthrough commands
  remain distinct** and the three Pi scenarios.
- `docs/specs/agents/requirements/runtime-updates.md`, managed-runtime command-routing
  boundary.
- `docs/decisions/2026-08-12-validated-managed-runtime-version-selection.md`,
  amended ACP-only ownership decision.
- Existing agent pattern: `apps/backend/internal/agent/agents/claude_acp.go`
  keeps its managed ACP package separate from its native passthrough CLI.

## Dependencies

None.

## Files Likely Touched

- `apps/backend/internal/agent/agents/pi_acp.go`
- `apps/backend/internal/agent/agents/new_acp_agents_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `docs/public/agents-and-profiles.md`
- `README.md`
- `docs/plans/pi-runtime-surfaces/plan.md`
- `docs/plans/pi-runtime-surfaces/task-01-correct-pi-execution-surfaces.md`

## TDD Sequence

1. Change the Pi expectations in `new_acp_agents_test.go` to require
   `PassthroughCmd == ["pi"]`, detection through `pi --version`, and the exact install
   script
   `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`.
2. Extend `TestPassthroughPiWritesProjectFile` to require the lifecycle-resolved
   passthrough argv to be `["pi"]` while preserving the MCP file assertions.
3. Run the targeted Go command and confirm it fails because production still
   returns `npx -y pi-acp` and the old install/detection contract.
4. Apply the minimal `pi_acp.go` changes. Do not change structured ACP or
   inference argv.
5. Update the public Pi installation and command-surface documentation.
6. Run the exact verification block and record every result below and in
   `plan.md`.

## Acceptance Criteria

- Structured `BuildCommand`, `Runtime().Cmd`, and one-shot inference remain
  `npx -y pi-acp`.
- Passthrough lifecycle command resolution returns `pi`, including the path
  that materializes Pi's project MCP configuration.
- Pi discovery requires `pi`, and installation runs exactly
  `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`; discovery
  rejects a same-named executable that fails the non-interactive version check.
- Public docs explain the separate ACP adapter and interactive CLI without
  implying that managed ACP version selection controls passthrough.

## Verification

Run from the repository root:

```bash
cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle -run 'Test(NewACPAgents_(AllCommandSurfaces|InstallScript|DetectionRequiresGlobalBinary)|PiACPDetectionRejectsPiBinaryWithoutVersionSupport|PassthroughPiWritesProjectFile)$' -count=1 && cd ../../ && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check
```

## Parallelism

`sequential`. The agent definition, shared contract fixture, lifecycle
regression, and documentation describe one coupled behavior.

## Risks

- Hosts that have only a global `pi-acp` adapter will become correctly
  unavailable until the interactive Pi package is installed; this is required
  because adapter-only installation cannot run passthrough.
- The package is installed with `--ignore-scripts` exactly as reported. The
  implementation must not relax or remove that safety flag.
- No package version-management support is added for `pi-acp` in this repair.

## Output Contract

Report the root-cause fix, files changed, red/green command results, public-doc
validation, blockers, remaining risks, and synchronized task/plan status in the
primary conversation.

## Results

- Red: the focused Go contract run failed on the old Pi passthrough argv and
  install script before the production change.
- Green: the focused Go command passed with 43 tests across the agents and
  lifecycle packages, including the collision regression for `pi --version`.
- Public docs validation passed: 61 validation tests, 41 published pages, and
  `git diff --check`.
- The lifecycle regression preserves Pi's existing `--model default` profile
  flag while proving the executable is `pi` and the `.pi/mcp.json` file is
  materialized.
- PR fixup: restored the `pi` collision rationale and validated the package's
  non-interactive version flag with the repository's `WithCommandCheck` helper.
