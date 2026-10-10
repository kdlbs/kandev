---
id: "01-web-tanstack-query-eslint"
title: "Add selected official TanStack Query ESLint rules"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WEB-TANSTACK-QUERY-LINT-001
acceptance_criteria:
  - AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.1
  - AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.2
  - AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.3
  - AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.4
  - AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.5
system_design:
  - ../../specs/platform/system-design/web-tanstack-query-eslint.md
---

# Task 01: Add selected Query rules

## Outcome

The web ESLint configuration enforces selected official TanStack Query rules
with focused tests that prove the rules detect unsafe patterns and accept
stable patterns.

## Scope

- Add @tanstack/eslint-plugin-query as a web development dependency.
- Enable the selected rules in the web ESLint flat configuration.
- Add focused tests for rule severities, invalid and valid patterns, and
  existing SystemInfo query consumers.

## Exclusions

- Changes to application query code or runtime behavior.
- Type-aware rules that need parser services.
- A broader TanStack Query migration or recommended preset.
- Public documentation changes.

## Acceptance

- AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.1: A query function value missing
  from its query key produces an error.
- AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.2: Rest destructuring produces a
  warning, and the web lint command rejects warnings.
- AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.3 and
  AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.4: Unstable query-hook dependencies
  and unstable QueryClient creation produce errors.
- AC-PLATFORM-WEB-TANSTACK-QUERY-LINT-001.5: Existing SystemInfo Query
  consumers produce no diagnostics from the selected plugin rules.

## Verification

Run from the repository root:

- cd apps && pnpm --filter @kandev/web exec vitest run scripts/lib/tanstack-query-eslint-config.test.ts
- cd apps && pnpm --filter @kandev/web lint
- cd apps && pnpm exec prettier --check web/eslint.config.mjs web/scripts/lib/tanstack-query-eslint-config.test.ts web/package.json
- python3 scripts/list-docs.py validate
- python3 scripts/lint-spec-files.py --all
- git diff --check

## Results

Completed. The focused test passed all five cases, full web lint passed,
Prettier passed, and the repository documentation and specification validators
passed.
