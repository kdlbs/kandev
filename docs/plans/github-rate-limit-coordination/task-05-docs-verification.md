---
id: 05-docs-verification
title: Documentation and verification
status: completed
wave: 5
depends_on: [04-operation-rate-errors]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.1
  - AC-INTEGRATIONS-GITHUB-RATE-001.2
  - AC-INTEGRATIONS-GITHUB-RATE-001.3
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-001.5
  - AC-INTEGRATIONS-GITHUB-RATE-002.1
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-002.3
  - AC-INTEGRATIONS-GITHUB-RATE-003.1
  - AC-INTEGRATIONS-GITHUB-RATE-003.2
  - AC-INTEGRATIONS-GITHUB-RATE-003.3
  - AC-INTEGRATIONS-GITHUB-RATE-003.4
  - AC-INTEGRATIONS-GITHUB-RATE-003.5
  - AC-INTEGRATIONS-GITHUB-RATE-004.1
  - AC-INTEGRATIONS-GITHUB-RATE-004.2
  - AC-INTEGRATIONS-GITHUB-RATE-004.3
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 05: Documentation and Verification

## Acceptance

- Workflow Sync recovery and operation-local rate errors are documented.
- Structured transition observability avoids repeated skipped-tick warnings.
- Backend tests/lint, spec/docs validation, and diff hygiene pass.

## Verification

- `make -C apps/backend test`
- `make -C apps/backend lint`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Results

Implemented the public Workflow Sync recovery guidance, operation-local rate
error reference, structured classification and recovery telemetry, and
transition-only logging. Focused GitHub and Workflow Sync tests pass.

Passing repository gates:

- `make -C apps/backend lint`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (61 tests)
- `node scripts/validate-public-docs.mjs` (41 pages)
- `python3 .github/scripts/lint-harness-files.py AGENTS.md`
- `git diff --check`

The 2026-09-25 merge against `main` at `b88aea31ad49b2cda40e8ad84452356888e36a42`
reconciled Workflow Sync with the landed auth circuit, preserved the task-bound
rate snapshot tool, and refreshed the public recovery guidance. The current
focused GitHub, Workflow Sync, MCP, and backend-app package suites pass;
Workflow Sync race tests, backend lint, spec lint/catalog validation, and both
public-doc validators pass. The PR delta passes `git diff upstream/main --check`.

`make -C apps/backend test` reports three unchanged-package failures in this
environment: `internal/common/config` sees the injected
`KANDEV_HEALTH_TIMEOUT_MS=360000` (the focused test passes with that variable
unset), while `internal/task/handlers` and `internal/task/service` reject local
repository parent access. Representative task failures reproduce from an exact
archive of the same `main` base. The broad gate remains failing on these base
failures.
