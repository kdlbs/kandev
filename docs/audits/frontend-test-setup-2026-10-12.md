# Frontend test setup audit, 2026-10-12

The current draft's frontend Actions step took 38m11s. The Vitest log reports
2,872 files, with summed setup/import phases exceeding assertion execution.
These overlapping worker phases are not wall-clock durations.

This continuation removes unused setup for 581 explicitly reviewed suites:
47 move from `browser-locales` to `node`, and 534 move to `browser`.
The Node baseline is 421: the earlier October 11 cleanup added
`e2e/helpers/backend-fixture-lifecycle.test.ts` to the original 420 assignments
in commit `aa3e581676`. These 47 migrations bring that count to 468 in
`354212310f`; the additional helper is not part of this migration ledger.
The 581 Vitest assignments preserve test source, assertions, skips, setup
implementations, isolation and worker budgets. Production code stays unchanged.
The explicit registry remains the only routing authority. New/unclassified suites continue to use full setup.

## Review boundary

The dependency review traced relative, `@/`, and workspace-package aliases,
literal dynamic imports, re-exports, Vitest mocks and importActual targets.
It screened transitive DOM use and locale activation, fixed-language translation,
resource-store inspection, locale options/catalog imports and computed imports.
The existing heuristic inventory was only a review queue.

The browser assignments retain Happy DOM, React's act guard, English catalogs,
WebSocket/localStorage/network safeguards, Monaco aliasing and per-file isolation.
They omit only the full non-English catalog preload. IndexedDB tests, React icon
tables, cmdk and sortable helpers retain a browser environment. Node assignments
cover pure calculations, filesystem/source guards and E2E harness helpers.

Shared references reviewed as unrelated to locale preload:

- `lib/ssr/user-settings.ts`: programming-language LSP preference arrays.
- Monaco initialization, completion and LSP-provider modules: programming-language APIs.
- `hooks/domains/github/task-pr-sync-resource.ts`: a task/PR request cache.
- `lib/api/domains/frontend-error-log-api.ts`: browser language diagnostic metadata.
- `lib/ui/state-labels.ts`: comments describing language changes; calls use the existing English translator.
- Docker/Kubernetes fixture policy and retry-report helpers: resource ownership/RBAC/attachment paths.
- Browser-demo system runtime: storage-maintenance resource selectors.
- ESLint i18n options: migration comments and Node rule configuration.
- Vitest Monaco boundary: the inherited alias resolves to the existing URI/model stub.

Multilingual catalog/announcement tests, locale owners and unresolved dependencies
retain full setup. This is a finite reviewed batch, not an automatic classifier.
The companion CSV records every file, old/new project, baseline result counts,
unchanged source hash and dependency footprint.

## Validation

At parent `aa3e5816766baf1ed1e2a991bd14817453da2b38`, the complete
production-environment baseline passed 2,864 files, 25,554 tests, with four skips.
Vitest reported 1,455.77 seconds (24m16s). Raw JSON/logs are ignored artifacts.

The 36-file stratified sample covers pure Node/source helpers, protocol fakes,
stores, hooks and rendered UI using real English translations. At two workers,
three alternating pairs retained all 234 exact test identities/outcomes:

| Pair | Order | Baseline seconds | Candidate seconds |
| --- | --- | ---: | ---: |
| 1 | baseline, candidate | 26.475 | 9.844 |
| 2 | candidate, baseline | 25.494 | 9.600 |
| 3 | baseline, candidate | 25.625 | 9.602 |

Median wall time: 25.625 to 9.602 seconds, 62.5% less. This sample gain is not
an estimate of full-suite or hosted savings. The two-worker sample fits the
existing 20% local budget; both full runs use the unchanged default budget.

Boundary fault injection temporarily placed
`lib/settings/profile-drag-accessibility.test.ts` in the English browser project.
Its Portuguese instruction assertion failed with English fallback text. The
registry was restored byte for byte before applying the reviewed assignments.
The test remains in full setup and passed in the final full suite.

The complete candidate passed the same 2,864 files, 25,554 tests and four skips.
Normalized file/test identity multiplicities and outcomes match exactly, with no
missing or added identities. Vitest elapsed time fell from 1,455.77 to 1,350.39
seconds (24m16s to 22m30s): 105.38 seconds saved, 7.2% less. Summed setup fell
from 1,646.04 to 1,257.22 seconds (23.6% less); other phase times are overlapping
worker sums, not wall time. Only the registry differed across all 9,051 tracked
frontend/workspace inputs during this complete pair.

Raw report SHA-256:

- Baseline: `1960147959ca5a41226fcb64b3f263fa552d3ba269bfa05cd7d536b45655df56`.
- Candidate: `f6339cf62b31639f385e16e7972301ee4293062c0274764ba49cd2d6d8b1a20e`.

A single full pair is separate from the repeated sample median. Hosted Actions
comparison is pending; the platform work order's three hosted full samples and
40% execution target remain unmet.

## Auto-start assertion repair (earlier audit F15)

The existing Playwright auto-start case now asserts zero initial sessions,
observes and correlates the automatic `session.ensure` response before any
intervention, checks exactly one resulting session, waits passively for its
terminal turn state, and verifies the exact persisted user prompt and agent
reply. The original page must display that reply and an idle input through
passive locators. The test makes no recovery click or page reload.

This repairs a weak oracle without adding a test case or changing shared
recovery helpers. Earlier F14's broader helper audit remains open.

A Go source overlay replaced `startTask`'s description fallback with a different
valid mock script. The legacy E2E still passed (one test, 8.1 seconds). The
repaired test failed at its exact persisted-prompt assertion, showing the
replacement script instead of the original sentinel. The source file was never
edited; the original backend executable was restored and SHA-256 checked before
positive-control verification. No production change or test-only export ships.

Normal baseline: one test passed, 9.2 seconds. Repaired keeper and three desktop
preference siblings passed together with no retries (four tests, 41.0 seconds). The mobile
preference sibling passed with no retries (one test, 12.3 seconds). Typecheck,
changed-spec ESLint, nine frontend workflow checks, seven measurement-script
checks, documentation inventory validation, specification lint and diff checks
passed.

## Review dispositions

The clock-install suggestion was checked against installed Playwright
1.62.0-alpha-1783623505000: `server/clock.ts` uses `Date.now()` when time is absent.
The existing LSP test already uses current wall time; no timestamp change is needed.
Merge-readiness review rechecked the current installed Playwright 1.61.1 and
confirmed the same default.

The Office status lifecycle keeps valid writes, invalid-status rejection,
terminal-step persistence and comment persistence in one fixture-backed journey.
Splitting it again would restore repeated setup. Both suggestions were replied
to and resolved during the user-authorized merge-readiness pass, with the
current-source evidence recorded in their respective GitHub review threads.
