---
id: "02-prove-browser-continuity"
title: "Prove browser continuity and document 503"
status: done
wave: 2
depends_on:
  - "01-preserve-credential-errors"
plan: "plan.md"
requirements:
  - REQ-AUTH-AUTH-001
acceptance_criteria:
  - AC-AUTH-AUTH-001.11
  - AC-AUTH-AUTH-001.16
  - AC-AUTH-AUTH-001.17
system_design:
  - ../../specs/auth/system-design/credential-lookup-failures.md
---

# Task 02: Prove browser continuity and document 503

## Summary

Prove that the specified 503 preserves a working browser session on desktop and phone.
Document the public API failure contract after the backend change exists.
Use existing page composition and controls without production UI changes.

## In scope

- Add cases in `apps/web/lib/api/client.test.ts` for 503 handling and later success across `fetchJson`, `fetchBlob`, and `fetchConditionalJson`.
- Assert `ApiError.status` and `body.code`, unchanged unauthorized callback count, and successful subsequent reads.
- Preserve a positive control for the existing challenged-401 callback.
- Add desktop and mobile auth-enabled browser scenarios using the existing backend fixture and shared helper.
- Use the existing account surface and a UI-triggered read, such as the sessions or token list.
- Arm the request/response observer before the UI action. Intercept that read with the backend's exact 503 contract.
- Assert the route remains the account route, the session cookie is unchanged, and the signed-in account remains visible.
- Remove the intercept and prove recovery through the same UI action without login.
- Add a challenged-401 control that still reaches `/login`.
- Update `docs/public/authentication.md` with a small reference subsection for 401 versus `503 persistence_unavailable`, recovery, and operator diagnostics.
- State that unreadable credentials produce 503 even on public/deferred paths, while credentialless probes retain their existing behavior.
- Synchronize work-order results and the parent plan after the exact checks pass.

## Out of scope

New frontend production logic, layout, controls, translated copy, automatic retries, fixture fault endpoints, and Postgres pool tuning.
If tests expose an unrelated client defect, record it and reconcile this package before expanding production scope.

## Acceptance

1. All fetch helpers preserve error information on 503 without invoking unauthorized handling. Genuine challenged 401 responses still invoke it.
2. Desktop and phone browser scenarios prove route, identity, and cookie continuity, followed by successful loading with the same session.
3. Public reference text matches the implemented status/header/body contract. All package and cross-reference checks pass with actual results recorded.

## Browser fixture and evidence boundary

Follow `api-token-repeat.spec.ts` and `mobile-api-token-repeat.spec.ts` for owned auth-enabled backend startup and baseline restoration.
Use a unique disposable database under the fixture root and close browser contexts in `finally`.
Share the continuity flow in `credential-store-errors-helpers.ts`.
Name the phone file `mobile-credential-store-errors.spec.ts` so `mobile-chrome` discovers it.
The existing account page provides the phone composition. This work adds no rendered control or geometry.
An ASCII preview is unnecessary because the proposed outcome is continuity of the existing surface.

Browser interception proves response handling, not database failure causality.
Task 01's real Go integration test is the defect-specific RED gate.
Add browser coverage after backend GREEN and record that separation in Results.
Do not claim that a mocked 503 browser test reproduces the Postgres incident.

## Verification

Run each block from the repository root. Install dependencies once before package commands in a fresh worktree.
Run managed E2E commands sequentially with their existing worker limits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/api/client.test.ts)
(cd apps/web && pnpm exec eslint lib/api/client.test.ts e2e/tests/auth/credential-store-errors.spec.ts e2e/tests/auth/mobile-credential-store-errors.spec.ts e2e/tests/auth/credential-store-errors-helpers.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/credential-store-errors.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-credential-store-errors.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/auth-credential-store-errors
```

Confirm discovery of the intended desktop and phone scenarios in each managed run.
The runners build the current web/backend artifacts and own teardown.
Also run the PR-documentation `validateCoverage` preflight described in the parent plan against the final changed paths and linked artifacts.
Record every command and result, including any build or fixture limitation.

## Files likely touched

- `apps/web/lib/api/client.test.ts`
- `apps/web/e2e/tests/auth/credential-store-errors.spec.ts` (new)
- `apps/web/e2e/tests/auth/mobile-credential-store-errors.spec.ts` (new)
- `apps/web/e2e/tests/auth/credential-store-errors-helpers.ts` (new)
- `docs/public/authentication.md`
- `docs/plans/auth-credential-store-errors/plan.md`
- This work order and Task 01 result records, if final integration requires an accurate result update.

## Dependencies

Task 01 must be complete. Read web `AGENTS.md`, `/e2e`, `/mobile-parity`, and `/docs-maintainer` guidance before implementation.

## Risks

The default `chromium` project excludes auth files. Use the explicit projects above.
Auth fixtures can leak enforced auth to later tests unless startup state is restored.
An intercepted response alone does not prove that a browser retained its identity. Assert the visible account and subsequent successful UI load.

## Parallelism

`sequential`

## Inputs

- [Auth requirements](../../specs/auth/requirements/auth.md), criteria `.11`, `.16`, and `.17`.
- [Credential lookup design](../../specs/auth/system-design/credential-lookup-failures.md), HTTP ordering and browser continuity.
- [Parent plan](plan.md), browser scenario matrix and compatibility boundaries.
- Existing `lib/api/client.test.ts`, auth fixture helpers, and desktop/mobile API-token repeat tests.
- Existing [provider-auth package](../integration-provider-auth-errors/plan.md), retained as completed historical evidence.

## Results

Complete. Backend causality is proved by Task 01's real Go integration tests; the browser scenarios prove response handling and session continuity, not database failure causality.

Changed surfaces:

- `apps/web/lib/api/client.test.ts`: 503 classification and recovery for `fetchJson`, `fetchBlob`, and `fetchConditionalJson`, plus the challenged-401 positive control.
- `apps/web/e2e/tests/auth/credential-store-errors.spec.ts` and `mobile-credential-store-errors.spec.ts`: desktop and phone continuity scenarios using the existing account token surface.
- `apps/web/e2e/tests/auth/credential-store-errors-helpers.ts`: shared intercept, route, identity, cookie, recovery, and challenged-401 flow.
- `docs/public/authentication.md`: public 401 versus `503 persistence_unavailable` contract, recovery, public/deferred behavior, and operator diagnostics.

Verification:

- `pnpm exec vitest run lib/api/client.test.ts`: 17 tests passed.
- `pnpm exec eslint` on the changed client and browser files: 0 errors and 0 warnings.
- `pnpm e2e:run --project auth tests/auth/credential-store-errors.spec.ts`: 1 scenario passed.
- `pnpm e2e:run --project mobile-chrome tests/auth/mobile-credential-store-errors.spec.ts`: 1 scenario passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- Spec, package, diff, and PR-documentation coverage checks all passed; coverage status is `covered` with both work orders accepted.

The browser scenarios use intercepted 503 responses and do not claim to reproduce the reporter's Postgres incident.
