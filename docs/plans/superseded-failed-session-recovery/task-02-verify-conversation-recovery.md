---
id: "02-verify-conversation-recovery"
title: "Verify desktop and phone recovery"
status: pending
wave: 2
depends_on:
  - "01-guard-passive-recovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.13
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.14
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.15
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 02: Verify desktop and phone recovery

## Summary

Prove that passive inspection preserves the failed conversation without starting its agent.
Verify explicit recovery and remembered selection on desktop and phone.

## In scope

- Hook regression coverage and isolated desktop/phone E2E scenarios.
- Minimal existing-control wiring only if tests expose a missing recovery path.
- Public recovery documentation after implementation.

## Out of scope

- New banners, confirmation dialogs, default-selection changes, npm startup repair, or generic concurrency restrictions.

## Acceptance

1. Opening, selecting, reloading, or revisiting the failed conversation preserves history and failure state with zero automatic launch or prompt mutation.
2. Explicit recovery targets that conversation and preserves primary ownership. Workspace inspection remains available while passive agent recovery is suppressed.
3. Phone session selection and recovery provide the same result through existing controls; no working sibling preserves normal recovery.

## UI-01: Inspect a superseded failure

Use the [combined preview](plan.md#ui-01-inspect-a-superseded-failure), criteria 001.13 and 001.15.

```text
Desktop: [ A1 Failed, selected ] [ A2 Running, primary ]
         [ History and failure ] [ Existing recovery actions ]
Phone:   [ Session picker: A1 Failed ]
         [ History and failure ]
         [ Existing recovery actions, stacked ]
```

Retain the current session picker, localized recovery card, navigation, safe areas, and chat scroll owner.
No new product labels or layout are required. If existing controls need changes, keep phone touch targets at least 44px.

## Test procedure

Add a focused hook case to `use-session-resumption.test.ts` or a sibling test file if its size requires splitting.
Denied status must not call `markSessionStarting` or `session.launch` automatically.
A stale allowed status followed by suppressed launch must not trigger restore/fresh fallback or leave a false running projection.
Explicit Resume must send user_action semantics and retain its current result handling.

Use API setup and existing isolated seed helpers from session and queued-ownership E2E tests.
If deterministic failure seeding needs a fixture extension, keep it E2E-only in the existing fixture path.
Do not add a production mutation endpoint or write to a user's database.
Use server state and launch/message counters to prove absence of work after page readiness, not an arbitrary sleep.
Test ordinary recovery as a control so a disabled global preference cannot produce a false positive.
Do not assert that every session.launch frame is absent: a legitimate sibling focus frame has a different target and intent.

Update `docs/public/sessions-and-review.md` with the narrow exception and explicit recovery behavior.
The page remains user guidance; do not publish the unproven npm hypothesis as a diagnosis.

## Verification

Run from the repository root. Install once if workspace dependencies are missing.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts)
make build-backend
make build-web
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/superseded-failed-session-recovery.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-superseded-failed-session-recovery.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a new hook test file is created, add its exact path to the Vitest command before running it.
Run E2E commands sequentially. Record actual results and fixture limitations.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.test.ts`
- `apps/web/hooks/domains/session/use-session-resumption.navigation.test.ts`
- `apps/web/hooks/domains/session/use-session-resumption.ts` (conditional)
- `apps/web/hooks/domains/session/use-session-resumption-operations.ts` (conditional)
- `apps/web/e2e/tests/session/superseded-failed-session-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-superseded-failed-session-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/superseded-failed-session-recovery-helpers.ts` (new, if shared setup is needed)
- `docs/public/sessions-and-review.md`

## Dependencies

Task 01.

## Risks

A mock without a resumable token or a disabled auto-start preference can hide the defect.
Browser-only silence does not prove the backend avoided a launch; assert both boundaries.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/tasks/system-design/queued-session-ownership.md#superseded-failed-conversation-recovery)
- [Requirements](../../specs/tasks/requirements/queued-session-ownership.md), criteria 001.13 through 001.15.
- Existing session recovery and queued ownership tests; `/e2e` and `/mobile-parity` instructions.

## Results

Pending. No browser or implementation tests ran during package authoring.
