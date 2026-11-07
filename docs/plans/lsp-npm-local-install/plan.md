---
created: 2026-10-27
status: implemented
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
legacy_specs: []
---

# Implementation Plan: Managed LSP npm local installation

## Overview

Make the shared npm installer select local mode for its managed prefix so a
successful TypeScript or Python server install remains discoverable when the
task or parent environment sets npm global mode or global location. One
sequential work order covers the process boundary and the smallest strategy change.

The supplied qualification ran the real strategy and npm offline against a
local fixture: inherited `NPM_CONFIG_GLOBAL=true` installed into `prefix/bin`
and caused binary lookup to fail; the `global=false` control passed. This is a
layout mismatch, not evidence of a network or registry failure.

## Scope

### In scope

- Preserve the managed local executable layout under inherited npm global mode.
- Preserve task-selected npm, runner, environment, prefix, registry, and error
  and cancellation behavior.

### Out of scope

- Resetting npm configuration, replacing registries or toolchains, changing
  runtime update policy, cleaning old caches, or modifying user global installs.
- UI changes, npm package version policy, and other installer strategies.

## Technical approach

`internal/lsp/installer/registry.go` selects `tools.NpmStrategy` for TypeScript
and Python. In `internal/tools/installer/npm_strategy.go`, explicitly select npm
local mode on the existing `install` command with `--global=false` and
`--location=project` before the package arguments. Keep `--prefix`,
`CommandSpec` and runner path, environment, working directory, result lookup
and error handling intact. Do not alter the
caller or strip npm configuration. Existing argument-recording tests need to
reflect the command change, but are not the regression proof.

## Tests

- AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.9: add
  `TestNpmStrategyRealNpmLocalInstall` in
  `apps/backend/internal/tools/installer/npm_strategy_test.go`. Execute real
  npm with a temporary, dependency-free fixture package and executable, offline
  and with private cache/config/prefix. Cover inherited global=true,
  location=global and a local control, verify returned local binary and absence of a global
  binary, and demonstrate the inherited-global case fails before the change.
  Exercise the runner environment path with actual subprocess execution and
  assert the task-supplied npm and unrelated configuration, including registry,
  reach the process; retain the managed prefix. Never use a real registry or a
  user's global installation. An explicit integration-test opt-in may gate the
  real npm test for environments lacking Node, but the verification command
  below must enable it and fail if npm is unavailable. Windows is explicitly
  skipped after the npm availability check: the test does not invoke `npm.cmd`
  through a command interpreter. Existing PATHEXT unit coverage still applies.
- AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.9: extend runner tests for nonzero and
  canceled command error propagation where not already covered, without
  replacing the real filesystem test with an argument assertion. Existing
  `TestNpmStrategyUsesInjectedCommandRunner`, task PATH lookup, and platform
  extension tests cover the adjacent command and launcher contracts.

## E2E tests

The observable outcome is restored server installation. The offline real-npm
strategy-to-filesystem test exercises the defective boundary directly; existing
LSP browser and mobile suites cover unchanged presentation and lifecycle. A
browser test with a fake server would not establish npm layout, and a real
registry download is excluded. No new browser E2E or mobile test is warranted
for this backend-only repair; this targeted evidence does not claim a new
full-editor end-to-end run.

## Public documentation

No public documentation change is planned. The existing
[`developer-tools` guide](../../public/developer-tools.md#language-servers)
already describes npm packages in Kandev's managed language-server storage and
preserving the task environment. This repair restores that behavior without a
new setting, user action, or UI; screenshots and mobile behavior are unchanged.
Reassess if implementation changes the public contract.

## Work orders

- [x] [Task 01: Keep managed npm installs local](task-01-managed-npm-local.md)

## Verification results

The work order records the RED/GREEN regression and targeted checks. Public docs and UI remain unchanged.

## Risks

- npm option precedence must override inherited global mode while preserving
  other task settings; the offline real-process regression is the decisive test.
- npm may not exist in every Go-only environment. The required local check uses
  the pinned Node 24.21 runtime; CI without npm may skip only the explicitly
  gated real-process test, while the required enabled check must fail if absent.
- Existing misplaced global-layout files are not migrated or removed.
- Inherited `NPM_CONFIG_SAVE=false` can cause a later npm install in the shared
  prefix to prune a previous server. This behavior predates the fix and remains;
  changing managed manifest/save policy is outside this work package.
