---
id: "02-scope-review-diff-readiness"
title: "Scope review diff readiness assertions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---
# Task 02: Scope review diff readiness assertions

## Outcome

Remove the recurring false flake in the moved-file review regression test by
making its loading assertion apply only to the selected file section.

## Requirements and design

- `REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002`
- `AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.3`
- `AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.4`
- `docs/specs/platform/system-design/e2e-duration-aware-sharding.md`,
  Reliability and flake contract section

## Acceptance

- `review-file-status.spec.ts` anchors the moved-file explanation and loading
  assertions to the selected moved-file diff section, using existing stable
  review markup. Unselected lazy sections cannot fail the selected-file check.
- The test uses causal Playwright assertions and no fixed sleep, timeout
  inflation, or retry-dependent success path.
- The focused test passes repeatedly with retries disabled and the existing
  E2E lint and sleep-ratchet checks remain clean.

## Likely files

- `apps/web/e2e/tests/review/review-file-status.spec.ts`

## Verification

```bash
cd apps/web
pnpm e2e:run --host --no-build tests/review/review-file-status.spec.ts -- --repeat-each=30 --workers=1 --retries=0
pnpm exec eslint --max-warnings 0 e2e/tests/review/review-file-status.spec.ts
pnpm run e2e:sleep-ratchet
```

The focused run must complete without retries or the observed global
`Loading diff...` count failure. If the selected section has a new stable
boundary, keep the selector scoped to that boundary and record the reason in
the test.

## Dependencies and parallelism

No implementation dependency. Execute sequentially in the primary session.

## Exclusions

Do not modify the recently merged PR-status hover or automation-notification
tests. Do not change the review product component unless the existing section
boundary cannot be addressed from the test.

## Results

The pre-fix stress run exposed a separate existing workspace-update fixture
race on one repetition. The targeted selector fix then passed once and passed
all 30 repetitions with retries disabled. ESLint and the E2E sleep ratchet also
pass.
