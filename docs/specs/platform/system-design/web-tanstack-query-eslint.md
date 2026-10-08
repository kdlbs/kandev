---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WEB-TANSTACK-QUERY-LINT-001
created: 2026-09-27
updated: 2026-09-27
owners:
  - kandev
---

# Web TanStack Query lint rules system design

## Purpose and boundaries

The Platform system owns shared repository quality checks. This design adds
selected official TanStack Query ESLint rules to the web application's flat
configuration. It changes contributor feedback only; query runtime behavior,
cache ownership, and network traffic remain unchanged.

## Requirement mapping

| Requirement                              | Design section                  |
| ---------------------------------------- | ------------------------------- |
| REQ-PLATFORM-WEB-TANSTACK-QUERY-LINT-001 | Rule selection and Verification |

## Rule selection

The web ESLint configuration registers the official
@tanstack/eslint-plugin-query package for TypeScript and TSX files. It enables
four rules from the [official plugin guide](https://github.com/TanStack/query/blob/main/docs/eslint/eslint-plugin-query.md):

| Rule                  | Severity | Contract                                                                                                                                                                                   |
| --------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| exhaustive-deps       | Error    | Query-function values that affect results belong in the query key. See the [rule documentation](https://github.com/TanStack/query/blob/main/docs/eslint/exhaustive-deps.md).               |
| no-rest-destructuring | Warning  | Rest destructuring a query result disables tracked-property optimizations. See the [rule documentation](https://github.com/TanStack/query/blob/main/docs/eslint/no-rest-destructuring.md). |
| no-unstable-deps      | Error    | React Hook dependency arrays use stable values extracted from query results. See the [rule documentation](https://github.com/TanStack/query/blob/main/docs/eslint/no-unstable-deps.md).    |
| stable-query-client   | Error    | A QueryClient has stable ownership across component renders. See the [rule documentation](https://github.com/TanStack/query/blob/main/docs/eslint/stable-query-client.md).                 |

The package version follows the existing TanStack Query dependency line and
resolves through the shared pnpm lockfile. The complete recommended preset is
not enabled; the configuration lists only rules selected for the current code.

The no-void-query-fn rule remains disabled. It requires TypeScript parser
services and a program, which the current ESLint configuration does not
provide. Enable it only after that type-aware configuration is introduced and
verified.

## Verification

The focused test at
apps/web/scripts/lib/tanstack-query-eslint-config.test.ts runs ESLint against
the repository configuration. It checks each selected rule with an invalid
fixture, checks that valid stable patterns pass, and verifies that the existing
SystemInfo query hook and provider remain free of plugin diagnostics.

The full web lint command runs ESLint with a zero-warning budget. Therefore the
warning-level no-rest-destructuring rule also fails that command when it finds
a violation.
