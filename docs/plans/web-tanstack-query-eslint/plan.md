---
created: 2026-09-27
updated: 2026-09-27
status: done
requirements:
  - REQ-PLATFORM-WEB-TANSTACK-QUERY-LINT-001
system_design:
  - ../../specs/platform/system-design/web-tanstack-query-eslint.md
---

# Implementation Plan: Web TanStack Query lint rules

## Outcome

Add selected official TanStack Query static checks to web ESLint. The checks
make cache-key omissions, unstable query-result dependencies, unstable client
ownership, and rest destructuring visible without changing query runtime
behavior.

The [requirement](../../specs/platform/requirements/web-tanstack-query-eslint.md)
defines the contract. The [system design](../../specs/platform/system-design/web-tanstack-query-eslint.md)
records the selected rules, limitations, and verification boundary.

## Work order

- [x] [Task 01: Add selected Query rules](task-01-selected-query-rules.md)

## Results

Completed. The focused ESLint configuration test, full web lint, formatting,
documentation validation, and specification lint passed.

## Public documentation

This change only affects contributor lint feedback. It does not change user
workflows, runtime behavior, public APIs, configuration, or installation, so
public documentation does not need an update.
