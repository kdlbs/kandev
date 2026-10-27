---
id: "01-managed-npm-local"
title: "Keep managed npm installs local"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001
acceptance_criteria:
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.9
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 01: Keep managed npm installs local

## Summary

Select local mode for npm installs into the managed prefix, regardless of an
inherited global setting, and verify the actual process and installed layout.
Keep the existing task environment and installer lifecycle contracts.

## In scope

- Add an independent offline real-npm regression with a private local fixture;
  verify inherited global=true and location=global fail before their respective
  fixes and succeed afterward, alongside a local control and task runner
  environment preservation.
- Make the minimal `NpmStrategy.Install` change; update affected recording
  tests and cover nonzero/cancel propagation where needed.

## Out of scope

- Registry or toolchain redesign, broad npm configuration resets, old cache
  cleanup, global installs, UI changes, and unrelated installer strategies.

## Acceptance

- Real npm installs the fixture into the managed local layout and returns its
  executable with inherited global=true, inherited location=global, and local mode.
- Task-selected npm, runner environment, managed prefix and registry survive;
  failures and cancellation continue to propagate through the runner.
- The existing TypeScript and Python registry paths continue to use the shared
  strategy without a new UI or public configuration contract.

## Verification

Use the pinned Go 1.26 and Node 24.21 installations on `PATH` before running
these commands. The opt-in test must fail, not silently skip, if npm is missing.
The commands are rooted at the repository root. Run the first test against the
new regression before the code fix and record its expected failure, then run
the complete block after the fix. The Go memory limit applies to tests; lint
retains its normal default memory budget.

```bash
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB KANDEV_TEST_NPM_INTEGRATION=1 go test -trimpath -tags fts5 -p 1 ./internal/tools/installer -run '^TestNpmStrategyRealNpmLocalInstall$' -count=1)
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB KANDEV_TEST_NPM_INTEGRATION=1 go test -trimpath -tags fts5 -p 1 ./internal/tools/installer ./internal/lsp/installer -count=1)
GOTOOLCHAIN=local GOMAXPROCS=2 bash scripts/lint-go-changed apps/backend/internal/tools/installer/npm_strategy.go apps/backend/internal/tools/installer/npm_strategy_test.go apps/backend/internal/tools/installer/command_runner_strategy_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/platform docs/plans/lsp-npm-local-install apps/backend/internal/tools/installer
git status --short -- docs/plans/lsp-npm-local-install
```

Use a unique `KANDEV_RUN_QUIET_DIR` per invocation if using `scripts/run-quiet`.
Implementation and the resource grant were released after design review.

## Files likely touched

- `apps/backend/internal/tools/installer/npm_strategy.go`
- `apps/backend/internal/tools/installer/npm_strategy_test.go`
- `apps/backend/internal/tools/installer/command_runner_strategy_test.go`

## Dependencies

None.

## Risks

- Test isolation must include npm cache, user and global config, prefix and
  offline registry, with no shared state or network access. Use a local fixture
  with no dependencies or lifecycle scripts.
- A test that only checks the new flags cannot prove npm's output layout or
  executable discovery. The real-npm test explicitly skips Windows after npm
  availability checking; existing PATHEXT shim lookup remains covered separately.
- With inherited `NPM_CONFIG_SAVE=false`, a later install in the shared prefix
  can prune an earlier server. This pre-existing save-policy behavior is not
  changed by this task.

## Parallelism

Sequential.

## Inputs

- [LSP requirements](../../specs/platform/requirements/lsp-file-intelligence.md)
- [LSP system design, managed npm installation](../../specs/platform/system-design/lsp-file-intelligence-02.md#managed-npm-installation)
- Existing installer strategy, command runner and tests; backend test guidance.

## Results

- RED: enabled `TestNpmStrategyRealNpmLocalInstall` failed only for inherited
  global=true with the binary missing from the local layout; global=false passed.
- The location=global regression failed independently against the first fix
  while both local and global-flag controls passed. After the corrective batch,
  all three real-npm scenarios and both installer packages passed; changed-file
  Go lint reported zero issues. Windows real-npm execution was not tested.
- `list-docs.py validate`, `lint-spec-files.test.py` (36 tests),
  `lint-spec-files.py --all`, and the documented diff/status checks passed.
- Node 24.21 and Go 1.26 were selected on PATH; test commands used the stated
  local toolchain, CPU, memory, `fts5`, `-p 1`, and `-trimpath` settings.
