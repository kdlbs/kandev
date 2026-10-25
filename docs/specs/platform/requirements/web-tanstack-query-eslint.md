---
status: active
system: platform
created: 2026-09-27
updated: 2026-09-27
owners:
  - kandev
---

# Web TanStack Query lint rules

## Overview

The web application uses TanStack Query for server state. Contributors need
lint feedback when query keys omit values used by a query function, query
results disable tracked-property behavior, query result objects enter React
Hook dependency arrays, or a QueryClient changes between renders. These checks
keep common cache and rendering mistakes visible during development.

## Requirements

### REQ-PLATFORM-WEB-TANSTACK-QUERY-LINT-001: TanStack Query static checks

**Intent:** Detect unsafe TanStack Query patterns in the web application before
they change runtime behavior or invalidate cache identity.

#### Acceptance criteria

- **AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.1:** When a query function reads a
  value that identifies its cached result, web lint shall report an error if
  the query key does not include that value.
- **AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.2:** When a query result is
  destructured with a rest property, web lint shall report a warning. The web
  lint command shall fail for that warning because it has a zero-warning
  budget.
- **AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.3:** When a complete query hook
  result is used in a React Hook dependency array, web lint shall report an
  error.
- **AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.4:** When a QueryClient is created
  during component rendering without stable ownership, web lint shall report
  an error.
- **AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.5:** Enabling these checks shall
  leave existing web TanStack Query consumers free of TanStack Query lint
  diagnostics.

## Exclusions

- Changes to query cache ownership, runtime behavior, or data freshness.
- Type-aware query rules until the web ESLint configuration provides and
  verifies TypeScript parser services.
