---
spec: docs/specs/platform/requirements/i18n.md
created: 2026-08-02
status: done
---

# Implementation Plan: Simplified Chinese Locale

## Overview

Extend the existing eager i18next and embedded Go catalog design with canonical
locale `zh-cn`. Implement the human catalogs and locale registration first,
then make locale negotiation and catalog parity strict, and finally prove the
persisted browser flow, public documentation, and user-requested repository-wide
checks without changing unrelated i18n migration scope.

---

## Backend

### Locale registration and negotiation

- `apps/backend/internal/i18n/i18n.go`: register `zh-cn`; normalize supported
  locale input case-insensitively to the canonical lowercase id so cookies and
  `Accept-Language: zh-CN` converge on the same locale.
- `apps/backend/internal/i18n/locales/zh-cn.json`: translate the three
  server-rendered browser error messages with an exact key match to `en.json`.
- `apps/backend/internal/i18n/i18n_test.go`: cover canonical normalization,
  case-insensitive BCP-47 input, Chinese message lookup, exact catalog parity,
  cookie precedence, q-value ordering, region matching, and English fallback.
- `apps/backend/internal/webapp/shell_test.go`: prove a `zh-cn` runtime locale
  produces `<html lang="zh-cn">` in the first server response.

## Frontend

### Catalogs and locale runtime

- Add `apps/web/src/locales/zh-cn/*.json` for all 18 current namespaces,
  mirroring all 3,300 current English keys while preserving interpolation
  variables, plural key suffixes, `<Trans>` tags, code tokens, shortcuts, and
  brand names.
- `apps/web/lib/i18n/index.ts`: register `zh-cn`, label it with the fixed endonym
  `简体中文`, and retain the rule that only `pseudo` is hidden in production.
- `apps/web/lib/i18n/index.test.ts` and `boot.test.ts`: cover support detection,
  selection lists, activation, translated catalog resolution, cookie persistence,
  `<html lang>`, boot-payload/cookie restoration, and invalid fallback.
- `apps/web/lib/i18n/formats.test.ts`: prove `zh-cn` reaches `Intl.NumberFormat`
  and `Intl.DateTimeFormat` through stable options while `pseudo` maps to `en`.
  No production formatter change is expected unless the RED test exposes one.

### Catalog integrity gate

- Refactor the smallest reusable catalog-parity operation out of
  `apps/web/scripts/check-i18n-keys.mjs` and cover it with a temporary-fixture
  test under `apps/web/scripts/`.
- Discover every committed real locale directory except source `en` and generated
  `pseudo`. Require exact namespace parity and exact per-namespace key parity,
  and report locale, namespace, missing keys, and extra keys.
- Preserve the existing source-key/orphan checks and the generated pseudo sync
  instruction; never generate or rewrite the English catalog.

## Documentation

- `docs/i18n.md`: document the complete real-language workflow, synchronized
  frontend/backend locale lists, fixed endonym labels, strict catalog check,
  pseudo generation boundary, and partial-migration limitation.
- `docs/public/feature-status.md`: describe English/Simplified Chinese selection
  and state that only migrated surfaces are translated.
- Keep this spec aligned with the shipped real-locale contract.

## Tests

- **Frontend locale contract:** `apps/web/lib/i18n/index.test.ts` and
  `boot.test.ts`; activate and restore a real Chinese locale and reject invalid
  input.
- **Formatting contract:** `apps/web/lib/i18n/formats.test.ts`; spy on or compare
  stable `Intl` behavior without asserting an OS-specific full date string.
- **Catalog parity:** a focused script test with temporary locale directories;
  prove missing namespace, extra namespace, missing key, and extra key failures.
- **Backend negotiation:** table-driven tests in
  `apps/backend/internal/i18n/i18n_test.go` plus the shell lang test in
  `apps/backend/internal/webapp/shell_test.go`.

## E2E Tests

- **Scenario:** GIVEN Appearance settings, WHEN 简体中文 is selected, THEN
  migrated copy and `<html lang="zh-cn">` appear, reload preserves both through
  the locale cookie, and the test restores English.
- **Files:** `apps/web/e2e/tests/i18n/language-switch.spec.ts` and
  `apps/web/e2e/tests/i18n/mobile-language-switch.spec.ts`.
- **What to verify:** stable translated text, canonical document locale, cookie
  persistence, reload behavior, and isolation from the existing pseudo test on
  both desktop and the `mobile-chrome` Pixel 5 project.
- Capture a clean Chinese Appearance screenshot for the PR and manually inspect
  translated surfaces for raw keys, broken interpolation/tags, and overflow.

## Verification Results

Implementation and focused verification are complete. The locale catalogs have
exact parity across 18 namespaces and 3,300 frontend keys; the backend catalogs
have exact parity across 34 messages. The focused frontend contract suite passed
32/32, both i18n gates passed, web typecheck passed, the affected backend system
suite and changed-code lint passed, and desktop and mobile language-switch E2E
passed 3/3 and 1/1 respectively. The Chinese Appearance capture was manually
reviewed and remains an ignored asset.

The full repository Make wrappers were not rerun for this focused rebase fixup;
their statuses and approved focused alternatives are recorded in Task 06. The
branch remains limited to the locale catalogs, validator/tests, mobile E2E
coverage, the backend test synchronization fix, and synchronized documentation.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in this primary conversation. No subagent
work is authorized by these waves.

Wave 1:

- [x] [Task 01: Frontend zh-cn integration](task-01-frontend-locale-catalogs.md)

Wave 2:

- [x] [Task 02: Backend locale negotiation](task-02-backend-locale-negotiation.md)
- [x] [Task 03: Catalog parity gate](task-03-catalog-parity-gate.md)

Wave 3:

- [x] [Task 04: Chinese locale E2E](task-04-chinese-locale-e2e.md)

Wave 4:

- [x] [Task 05: Localization documentation](task-05-localization-documentation.md)

Wave 5:

- [x] [Task 06: Repository verification](task-06-repository-verification.md)

## Risks

- Human review is required for 3,300 Chinese messages; key parity alone cannot
  prove natural wording, preserved semantic tokens, or layout fit.
- Node 24 and Corepack select pnpm 9.15.9 from `apps/package.json`. Go 1.26.0 is
  installed under the current user's program directory and registered in the
  user PATH; Task 02 and repository verification successfully invoked it
  directly.
- Full E2E and manual `make dev` validation are environment-sensitive and may
  expose baseline failures; focused locale evidence must remain separately
  recorded.

## Open Questions

None. The user supplied the locale id, translation scope, persistence contract,
verification commands, branch name, commit title, and PR requirements.
