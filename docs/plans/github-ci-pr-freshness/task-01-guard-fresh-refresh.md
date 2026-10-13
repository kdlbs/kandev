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
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.6
  - AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.7
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

- Preserve status freshness during partial feedback persistence and invalidate
  feedback only for an external provider feedback event.

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
(cd apps/backend && go test -trimpath ./internal/github -run '^(TestPRFeedbackPreservesStatusFreshness|TestAutomationFeedbackInvalidationPreservesSiblingCache|TestGetPRFeedbackForWorkspace_CachedResponseDoesNotRewriteRow)$' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestPRFeedbackEvent' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/github ./internal/orchestrator -count=1)
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
const { execFileSync } = require('node:child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/github-ci-pr-freshness.md',
  'docs/specs/integrations/system-design/github-ci-pr-freshness.md',
  'docs/plans/github-ci-pr-freshness/plan.md',
  'docs/plans/github-ci-pr-freshness/task-01-guard-fresh-refresh.md',
];
const gitLines = args => execFileSync('git', args, { encoding: 'utf8' }).trim().split('\n').filter(Boolean);
const [base] = gitLines(['merge-base', 'HEAD', 'origin/main']);
const changedFiles = [...new Set([
  ...gitLines(['diff', '--name-only', base, 'HEAD']),
  ...gitLines(['diff', '--name-only', 'HEAD']),
  ...gitLines(['ls-files', '--others', '--exclude-standard']),
])];
const result = validator.validateCoverage({
  changedFiles,
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

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/service_pr_feedback_sync.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_automation_auth.go`
- `apps/backend/internal/github/service_pr_feedback_freshness_test.go`
- `apps/backend/internal/github/service_pr_feedback_sync_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_feedback_cache_test.go`

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


## PR remediation

Greptile identified incomplete feedback observations renewing merge-status
freshness; CodeRabbit identified stale cached feedback on a provider event.
Both findings are actionable and addressed by this work order.

Review remediation validation:

- RED: `TestPRFeedbackPreservesStatusFreshness` failed all five cases because
  partial feedback renewed the stored and published timestamp.
- RED: `TestPRFeedbackEventReadsNewFeedbackWithoutStatusRefresh` failed with
  no queued prompt when the provider event reused an older cached response.
- Focused GitHub producer/cache tests and orchestrator freshness/event tests
  passed. Coverage now includes 23 new regression cases plus the strengthened
  existing cache-hit persistence check.
- `(cd apps/backend && go test -trimpath -race ./internal/github ./internal/orchestrator -count=1)`:
  passed. GitHub package: 98.231s; orchestrator package: 210.856s.
- `(cd apps/backend && golangci-lint run ./... --new-from-rev=7c340deb50847c9015e3be73998e78f921e4aec4 --timeout=5m)`:
  passed, zero issues.
- Documentation catalog, 36 specification-linter tests, full specification
  lint, documentation coverage over the full changed-file set, and diff
  checks passed.

The targeted cache invalidation preserves sibling PR cache entries. Credential
failure prevents dispatch from cached feedback. Partial feedback retains the
existing status-sync timestamp or clears it when the head/base changes; normal
status-sync writes continue advancing it. Both review findings are addressed
locally. Publication and exact-head CI/review evidence are tracked in PR #4462.
