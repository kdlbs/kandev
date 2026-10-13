---
id: "01-guard-fresh-refresh"
title: "Guard fresh CI PR refreshes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.1
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.3
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.4
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.5
system_design:
  - ../../specs/integrations/system-design/github-ci-pr-freshness.md
---

# Task 01: Guard fresh CI PR refreshes

## Summary

Prevent fresh PR automation evaluations from forcing another provider sync.
Use TDD with handler-level call-count regressions, then add the minimum
freshness guard and retain stale-state safety behavior.

## In scope

- Add the four regression groups named in the plan using existing mock services.
- Require `!freshlySynced` at the existing refresh conditional.
- Audit refresh-specific fixtures and make their input stale when necessary.

## Out of scope

Review policy, provider derivation changes, UI, new throttles, schema, and APIs.

## Acceptance

- Fresh auto-fix, auto-merge, and combined cases record zero forced sync calls
  while eligible feedback and merge actions still execute.
- Stale/missing timestamps refresh once and evaluate the matching result;
  stale returned status and refresh errors preserve merge and lifecycle safety.
- No-refresh and PR-scoped disabled cases remain quiet; partial-sync coverage
  explicitly reaches the refresh branch and uses the matching fresh result.

## Verification

Run from the repository root. First run the new tests against unchanged code
and record the expected fresh-input call-count failures. After the correction:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestCIAutomation(FreshStatus|StaleStatus|RefreshAdmission)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The backend module requires Go 1.26; automatic toolchain selection or an
installed compatible toolchain must be available. Record actual results and
any tooling limitation. Ensure Node is on PATH; it was discovered in the local
toolchain installation during implementation. Run this local repository
coverage preflight from the root (it
includes the production file this work order owns):

```bash
node <<'JS'
const fs = require('node:fs');
const validator = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/github-ci-pr-freshness.md',
  'docs/specs/integrations/system-design/github-ci-pr-freshness.md',
  'docs/plans/github-ci-pr-freshness/plan.md',
  'docs/plans/github-ci-pr-freshness/task-01-guard-fresh-refresh.md',
];
const result = validator.validateCoverage({
  changedFiles: [...paths, 'apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go'],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result));
process.exitCode = result.ok ? 0 : 1;
JS
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_freshness_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  (refresh-specific fixture corrections only)
- Paired freshness specifications and this plan for lifecycle/results updates.

## Dependencies

None. The user explicitly authorized implementation on 2026-10-13.

## Risks

An existing test with fresh input may silently cease to exercise refresh failure
or partial-sync handling. Assert sync call counts and keep the returned matching
PR separate from stale input. Feedback write-through and its residual semantic
updates remain intact.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-ci-pr-freshness.md).
- [System design](../../specs/integrations/system-design/github-ci-pr-freshness.md).
- Existing freshness and refresh/merge tests in the orchestrator package.
- `.agents/skills/tdd/references/backend-tests.md`.

## Results

Implementation validation on 2026-10-13:

- RED: the focused regression command failed five fresh-input cases with one
  forced sync instead of zero, before the production guard was added.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run
  '^TestCIAutomation(FreshStatus|StaleStatus|RefreshAdmission)' -count=1)`:
  passed (four groups, 15 cases; package time 0.542s).
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -count=1)`:
  passed (package time 206.892s), including refresh failure, freshness boundary,
  stale-merge rejection, options updates, and the corrected partial-sync fixture.
- `python3 scripts/list-docs.py validate`: passed (381 decisions, 1587 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; all changed Go files are gofmt-clean.
- The work-order Node preflight using
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`: passed,
  status `covered`, no errors. Node was installed but absent from PATH.

Implementation matches the paired requirements and design. The requirement is
active, the design current, and this work order complete. No live-provider
rate measurement was performed. Publication is tracked through the pull request.

