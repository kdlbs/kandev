---
id: "02-recovery-proof-and-guidance"
title: "Behavioral proof and recovery guidance"
status: done
wave: 2
depends_on:
  - "01-capacity-policy-and-feedback"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.5
  - AC-PLATFORM-TURN-CONTINUITY-003.6
  - AC-PLATFORM-TURN-CONTINUITY-003.7
  - AC-PLATFORM-TURN-CONTINUITY-003.8
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 02: Behavioral proof and recovery guidance

## Summary

Prove delayed recovery through real mock ACP and rendered desktop/phone Chat,
including cancellation, expiry, and backend restart. Update public guidance
only after implementation makes the new recovery window available.

## In scope

- Shared delayed-capacity fixture, desktop and mobile specs, deterministic
  backend time advancement available only through isolated test construction.
  Read `/e2e` and mock-agent scoped guidance before fixture edits.
- Every scenario in the plan's E2E matrix, with same-process/native identity and
  initialization trace evidence. Test beyond the fifth attempt, not merely
  adjusted banner copy. Preserve completed tool rows and internal-prompt hiding.
- Update existing desktop/phone transient-turn-runtime-continuity expectations
  and fixture failure counts that currently assume five capacity retries; do
  not leave these tests waiting thirty minutes or accidentally succeeding early.
- Screenshot UI-01/UI-02 and phone hitbox, wrapping, overflow, composer and draft
  checks. Existing UI previews remain the structural acceptance reference.
- Public recovery explanation in `docs/public/tasks-and-workflows.md`; inspect
  root README and screenshot catalog for conflicting claims before delivery.
- Record exact task checks and align plan delivery status; do not rewrite
  historical results in earlier completed packages.

## Out of scope

- Live-user task replay, production credential calls, generalized time travel,
  unrelated recovery E2E suites, and unrequested commit/push/PR operations.

## Acceptance

1. Desktop and phone prove recovery beyond five attempts plus cancellation,
   expiry, count exhaustion, reload/viewer deduplication, restart retirement,
   and manual refusal, with unchanged native identity where recovery is admitted.
2. Phone rendering matches UI-01/UI-02, all controls are usable by touch, and
   long text causes no horizontal document overflow or draft loss.
3. Public explanation states the actual limits and fallback behavior; exact
   focused checks and spec/documentation gates pass with results recorded.

## Verification

Run from repository root after Task 01's checks. The managed runner rebuilds
production assets and the backend; run these focused suites sequentially.

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/delayed-capacity-retries.spec.ts tests/session/transient-turn-runtime-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-delayed-capacity-retries.spec.ts tests/session/mobile-transient-turn-runtime-continuity.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/delayed-capacity-retries
```

If mock-agent Go logic changes, also run
`(cd apps/backend && go test ./cmd/mock-agent -count=1)` and changed-Go-file lint.
If shared E2E helpers introduce utility logic, add/run their targeted Vitest
tests. Re-run affected Task 01 tests only for implementation corrections.
Use the repository PR-documentation `validateCoverage` preflight on changed
paths and linked files before completing the package.

## Files likely touched

- `apps/web/e2e/tests/session/delayed-capacity-retries.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-delayed-capacity-retries.spec.ts` (new)
- `apps/web/e2e/helpers/delayed-capacity-retries.ts` (new)
- `apps/web/e2e/helpers/transient-turn-runtime-continuity.ts`
- Existing desktop/phone `transient-turn-runtime-continuity.spec.ts` files.
- `apps/web/e2e/fixtures/backend.ts` and narrowly scoped test-construction support if needed.
- `apps/backend/internal/backendapp/e2e_capacity_retry_clock.go` and its tests.
- Backend composition wiring in `main.go`, `helpers.go`, `types.go`, and `orchestrator.go`.
- `apps/web/e2e/helpers/capacity-retry-clock.ts`.
- Existing mock-agent capacity scenarios are reused without protocol changes.
- `docs/public/tasks-and-workflows.md`
- This plan and work-order results; owning requirement/design delivery links.

## Dependencies

Task 01 must be done. Use its clock seam without exposing a production override.
No new rendered product UI is added here; validate the complete
[UI-01/UI-02 previews](plan.md#ascii-ui-preview) delivered by Task 01.

## Risks

Mock-only backend construction must remain absent from shipped production.
Retained runtime restart scenarios must not turn adoption into retry authority.
Browser fake clocks cannot control Go timers; use positive backend observations.

## Parallelism

`sequential`

## Inputs

- REQ-PLATFORM-TURN-CONTINUITY-003 and AC `.1` through `.8`.
- Owning design, Capacity episode timing and Capacity episode verification.
- Plan E2E matrix and existing transient runtime continuity helpers/traces.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and `docs/public/README.md`.

## Results

Implemented and locally verified. Existing mock ACP scenarios are reused. The
clock is constructed only with both existing E2E and mock-agent selectors,
including mock mode `only`; production route absence and clock synchronization
are covered by race-tested unit tests.

Both managed suites passed with one worker: Chromium 8/8 and mobile-chrome 8/8.
They cover extended recovery on the same process/native conversation, completed
side-effect preservation, Cancel followed by new input, restart retirement,
window expiry without dispatch, reload/viewer continuity, unsafe-work refusal,
and eleven-attempt exhaustion. Phone checks use touch controls, a 44px Cancel
hitbox, viewport containment, and the native Send button. Fresh desktop/phone
waiting and expired screenshots were inspected and compressed; all four files
are present in the capture manifest. The suites reused freshly rebuilt artifacts
under the managed runner's normal freshness checks.

Initial browser failures exposed missing clean-continuation and initiating-user
continuity checks; Task 01 regressions and fixes address both. Test-only corrections
aligned mock response copy, phone sending, and eleven-attempt assertions. Earlier
stale-artifact preflight rejections are not counted as behavioral validation.
Public guidance, documentation validators/tests, spec gates, and PR coverage pass.

After PR remediation and reconciliation with main's shared-policy refactor,
Chromium and mobile-chrome each passed all eight scenarios again. Both suites
used freshly rebuilt backend/delivery/plugin artifacts and production web assets
with unchanged freshness gates. New desktop/phone waiting and expired captures
were collected for the remediation head. Timeout diagnostics now preserve the
original polling error without secondary file or API reads.
