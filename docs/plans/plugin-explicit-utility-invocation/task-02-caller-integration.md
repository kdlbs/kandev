---
id: "02-caller-integration"
title: "Plugin caller integration and public contract"
status: done
wave: 2
depends_on: ['01-host-contract']
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EXPLICIT-UTILITY-001
acceptance_criteria:
  - AC-PLUGINS-EXPLICIT-UTILITY-001.1
  - AC-PLUGINS-EXPLICIT-UTILITY-001.2
  - AC-PLUGINS-EXPLICIT-UTILITY-001.3
  - AC-PLUGINS-EXPLICIT-UTILITY-001.4
  - AC-PLUGINS-EXPLICIT-UTILITY-001.8
  - AC-PLUGINS-EXPLICIT-UTILITY-001.9
  - AC-PLUGINS-EXPLICIT-UTILITY-001.10
  - AC-PLUGINS-EXPLICIT-UTILITY-001.11
system_design:
  - ../../specs/plugins/system-design/plugin-explicit-utility-invocation.md
---

# Task 02: Plugin caller integration and public contract

## Summary

Add a real plugin caller that reads its own preference and sends an explicit override.
Document the public default/override API and prove the settings-to-execution flow through the packaged fixture.

## In scope

- Add two test-only fixture actions: one omits options; the other reads its saved profile and passes it explicitly.
- Add package/Go tests and the E2E flows in the plan, including cleared preference and invalid saved profile.
- Retain the existing generic picker and hydration behavior. Do not add new production UI controls.
- Update public authoring and manifest documentation, Host interface examples, and the gRPC contract.
- Explain old-client default semantics, old-host rejection, error handling, and plugin-owned preference loading.

## Out of scope

- General agent execution, cwd, session APIs, profile eligibility changes, and unrelated UI redesign.
- Work owned by other tasks. Do not revert their changes or the prior hydration and Windows CI fixes.

## Acceptance

- End-to-end evidence identifies the executed profile, rather than merely the outgoing option.
- Save/reload, clear-to-default, changed default, and invalid override satisfy the mapped criteria.
- Public examples contain no implicit host configuration routing or stale legacy fallback instructions.

## Verification

Use TDD for changed logic. Commands below run from the host repository root unless stated otherwise.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins)
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:run --project chromium -- e2e/tests/plugins/plugin-utility-invocation.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/plugin-fixture/plugin.go`
- `apps/backend/cmd/plugin-fixture/main_test.go`
- `apps/backend/cmd/plugin-fixture/fixture_package_test.go`
- `apps/backend/cmd/plugin-fixture/fixture-package/manifest.yaml`
- `apps/web/e2e/tests/plugins/plugin-utility-invocation.spec.ts (new)`
- `apps/web/e2e/tests/plugins/plugin-test-helpers.ts`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/plans/plugins/GRPC-CONTRACT.md`

## Dependencies

01-host-contract.

## Risks

- Fixture evidence must cover real host selection; an echo of requested options would miss dispatch regressions.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/plugins/requirements/plugin-explicit-utility-invocation.md) and [design](../../specs/plugins/system-design/plugin-explicit-utility-invocation.md).
- [Plan](plan.md), including compatibility rules, evidence matrix, and delivery instructions.
- Existing SDK wire tests, host utility tests, and plugin-fixture patterns. Read changed source and local AGENTS.md before editing.

## Results

Implemented. Frozen workspace dependencies, fixture Go tests, and
`make -C apps/backend e2e-plugin-package` passed. The Chromium E2E test passed
with real fixture transport and execution evidence for default, explicit,
changed-default, cleared-preference, unset-default, and invalid-override
flows. Public-doc validation, document-catalog validation, all 36
specification-linter tests, full specification lint, and `git diff --check`
passed.

The host commit and PR head are recorded in the parent plan after delivery.
