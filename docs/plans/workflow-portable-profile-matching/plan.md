---
created: 2026-10-28
status: implemented
requirements:
  - REQ-TASKS-PORTABLE-PROFILE-001
system_design:
  - ../../specs/tasks/system-design/workflow-portable-profile-matching.md
legacy_specs: []
---

# Implementation plan: Portable workflow profile matching

## Overview

Make workflow profile references distinguish provider options, and prevent
unattended synchronization from silently guessing. Deliver one vertical work
order because format validation, matching, and safe reconciliation must agree.

## Scope

### In scope

- Versioned export and import of profile configuration options for workflow,
  direct step, and review-action references.
- Option-aware matching and fail-safe sync warnings with existing binding and
  legacy compatibility rules.
- Browser import bindings, sync status evidence, and public documentation.

### Out of scope

- Changing the external workflow repository or configuring live profiles.
- New interactive sync controls, sessions, profile configuration, or runtime routing.

## Technical approach

Extend `workflow/models/export.go` and its review conversion to version 3;
update the matcher and resolver in `backendapp/services.go`, browser binding
equality in `workflow/service/import_profile_selection.go`, and sync preflight
in `workflow/service/sync_apply.go`. Carry the descriptor through the web
import types. Reuse existing status warning rendering. Keep v1/v2 comparison
without options and keep legacy raw-import tie breaking; sync alone rejects
unresolved candidates without writing the affected workflow. Use the paired
design for exact matching and failure semantics.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.1, .4, .7 | `export_test.go`, `export_review_test.go`: v1/v2/v3 validation, v3 YAML/JSON round trip, version rejection, review actions, option-only export |
| 001.2, .3, .6 | `agent_profile_matcher_test.go`, `sync_apply_test.go`, `sync_review_profile_test.go`: distinct options, duplicates, read errors, current IDs, warning, no writes and recovery |
| 001.5 | `import_profile_selection_test.go`, `service_test.go`, MCP and HTTP import tests: option-aware auto-match, explicit binding, legacy and raw behavior |

All identifiers have prefix `AC-TASKS-PORTABLE-PROFILE-`.

## E2E tests

Extend `apps/web/e2e/tests/workflow/workflow-sync-gitlab.spec.ts` in Chromium:
seed profiles with matching triples and different options, sync a version 3
workflow, inspect the bound ID, then introduce ambiguity and verify the status
warning and unchanged workflow before recovery. This covers 001.1, .2, .4,
and .6 through the existing UI. Add an import browser case to
`workflow-import-export.spec.ts` for version 3 binding round trip (001.5).
The existing warning surface changes content, not its rendered composition.

## Work orders

- [x] [Task 01: Resolve portable profile identities](task-01-resolve-profile-identities.md)

## Verification results

Design validation: `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
Implementation verification: the targeted backend packages, profile matcher,
frontend tests and typecheck, public-doc validators, and changed-Go lint passed.
The combined Chromium workflow sync and import/export suites passed 14 tests
on the updated base. The first typecheck exceeded the default Node heap; the
same check passed with an 8 GiB heap. See the work order for commands.
The review fixup also passed the workflow service suite and changed-Go lint.

## Risks

- Legacy descriptors can become ambiguous where older sync silently chose a
  profile. Warning and preserving the existing workflow are intentional.
- An old Kandev version cannot consume version 3; version rejection prevents
  it from silently ignoring identity options.
- Provider-defined option values can contain sensitive data; exported files
  require user review and warnings must omit those values.
- Profile changes concurrent with preflight are outside a new cross-store
  transaction; do not claim write atomicity for subsequent storage failures.
