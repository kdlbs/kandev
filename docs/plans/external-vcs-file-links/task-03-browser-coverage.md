---
id: "03-browser-coverage"
title: "Browser coverage"
status: done
wave: 3
depends_on: ["01-link-foundation", "02-toolbar-wiring"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 03: Browser coverage

## Acceptance

- Desktop Playwright proves a GitHub linked-PR file action opens the head-branch file in a new tab, base fallback works, and an unpublished added file has no action.
- Mobile Playwright proves the Files viewer exposes the same target through a 44px control, opens a new tab, and does not create document horizontal overflow.
- Tests configure provider metadata through existing E2E APIs and do not rely on a developer instance or external provider credentials.

## Verification

```bash
(cd apps/web && pnpm e2e -- tests/review/external-vcs-file-link.spec.ts --project=chromium)
(cd apps/web && pnpm e2e -- tests/task/mobile-external-vcs-file-link.spec.ts --project=mobile-chrome)
```

## Files likely touched

- `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts`
- `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if existing repository setup helpers cannot express the provider metadata needed by these isolated tests.

## Dependencies

- `01-link-foundation` and `02-toolbar-wiring` complete.

## Inputs

- Spec: desktop published/base/unavailable scenarios and mobile parity scenario.
- Plan: `E2E Tests` and `Mobile design contract`.
- Patterns: `mobile-file-viewer.spec.ts`, `mobile-review-file-status.spec.ts`, `SessionPage`, and existing mock GitHub association helpers.

## Output contract

Report summary, files changed, exact Playwright results, screenshots/geometry observations when useful, blockers, flakes, and any divergence. Update only this task file's `status` to `in_progress` at start and `done` after acceptance and verification pass; do not edit `plan.md`.

## Completion report

- **Summary:** Added isolated desktop and mobile browser coverage for a provider-backed file link, including published-head, base fallback, and unavailable added-file behavior.
- **Changed scope:** `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts`, `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts`, and E2E repository setup helpers needed to seed trusted provider/task metadata.
- **Focused verification:** `(cd apps/web && pnpm e2e -- tests/review/external-vcs-file-link.spec.ts --project=chromium)` passed (2 tests); `(cd apps/web && pnpm e2e -- tests/task/mobile-external-vcs-file-link.spec.ts --project=mobile-chrome)` passed (1 test). The mobile scenario asserted a 44px control, same new-tab target, and no document horizontal overflow.
- **Blockers/risks:** No provider credentials or developer-instance dependency was introduced. The generic repository HTTP write path was later deliberately excluded by the security remediation; fixture setup uses the trusted provider/task path.
