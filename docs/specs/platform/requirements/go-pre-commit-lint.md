---
status: active
system: platform
created: 2026-09-22
owners:
  - kandev
---

# Go pre-commit lint requirements

## Overview

Contributors need Go commit checks whose analysis targets follow the staged
change. Platform owns development validation performance and shared operational
guarantees; the CI system retains ownership of hosted workflow trust policy.

## Terminology

- **Selected files:** Existing Go source or test files supplied to the commit
  check. Ordinary commits select staged files; explicit manual runs can select
  other files.
- **Selected package:** The complete Go package containing a selected file.
  Loading dependencies for type information does not select them as lint targets.

## Requirements

### REQ-PLATFORM-GO-PRE-COMMIT-LINT-001: Bound local Go lint analysis

**Intent:** Avoid analyzing unrelated backend packages on every commit while
retaining package context and changed-code enforcement.

#### Acceptance criteria

- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.1:** Selected source and test files shall
  select their complete containing packages. Unrelated packages and unchanged
  descendants shall not become lint targets solely because another package changed.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.2:** Each package shall appear once in each
  supplied batch. Batches shall run serially to avoid competing linter processes.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.3:** A selection without eligible Go files
  shall finish without invoking the linter or falling back to a full scan.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.4:** The check shall preserve the existing
  pull-request comparison-base and changed-code reporting policy. Base-resolution
  errors and linter failures shall prevent the commit.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.5:** Inherited repository-location, index,
  and object-storage overrides shall not redirect the check to another repository.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.6:** Ordinary commits shall retain
  pre-commit's staged-file selection and restoration of unstaged edits.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.7:** CI and the explicit full-backend lint
  command shall retain backend-wide analysis, including impacts on other packages.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.8:** Selected-package lint shall default to
  a 3 GiB per-process Go memory target, two analysis CPUs, and two build workers.
  Explicit memory, CPU, and build-parallelism overrides shall remain available;
  unrelated Go flags shall be preserved. The memory target shall be documented
  as soft rather than a limit on the entire process tree.
- **AC-PLATFORM-GO-PRE-COMMIT-LINT-001.9:** Harness instructions for local backend
  checks and PR fixups shall use selected-package lint and serialize heavyweight
  validation. Full-backend local analysis shall require an explicit request and
  OS-level resource limits; routine backend-wide validation shall belong to CI.

## Out of scope

- Changed-line-only compilation, reverse-dependency selection, or lint caching.
- Changing other commit hooks, lint rules, comparison-base precedence, or CI scope.
- Adding checks for deleted-only files or module/configuration-only changes.
- Application behavior and user interfaces.

## Implementation plans

- [Go pre-commit lint](../../../plans/go-pre-commit-lint/plan.md)
