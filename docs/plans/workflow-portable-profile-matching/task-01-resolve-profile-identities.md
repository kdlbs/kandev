---
id: "01-resolve-profile-identities"
title: "Resolve portable workflow profile identities"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PORTABLE-PROFILE-001
acceptance_criteria:
  - AC-TASKS-PORTABLE-PROFILE-001.1
  - AC-TASKS-PORTABLE-PROFILE-001.2
  - AC-TASKS-PORTABLE-PROFILE-001.3
  - AC-TASKS-PORTABLE-PROFILE-001.4
  - AC-TASKS-PORTABLE-PROFILE-001.5
  - AC-TASKS-PORTABLE-PROFILE-001.6
  - AC-TASKS-PORTABLE-PROFILE-001.7
system_design:
  - ../../specs/tasks/system-design/workflow-portable-profile-matching.md
---

# Task 01: Resolve portable workflow profile identities

## Summary

Implement the option-aware portable descriptor and safe sync preflight across
all profile reference sites. Preserve existing browser binding and legacy
import contracts while making sync failures visible and recoverable.

## In scope

- Implement version 3 validation and export, structured matching, safe sync
  preflight, browser descriptor equality, and all review-action remapping.
- Cover legacy versions, no-match and ambiguity, catalog errors, existing
  disabled bindings, malformed inputs, and unchanged-source recovery.
- Update public workflow import/export and sync docs alongside behavior.

## Out of scope

- Runtime profile configuration and changes to external workflow repositories.
- New sync UI controls or a change to legacy raw-import tie-break policy.

## Acceptance

1. Version 3 round-trips options in all profile references; v1/v2 remain
   readable and older readers reject v3. Distinct option maps select the
   intended eligible profile.
2. Any missing, ambiguous, or unreadable sync reference leaves its workflow
   unchanged, reports a contextual warning, and permits unrelated workflows
   to proceed. Disabled matching bindings survive when unambiguous.
3. Browser, raw, and MCP imports retain their respective selection contracts;
   end-to-end tests show the selected profile and visible warning recovery.

## Verification

From the repository root, write the regression tests first and run the relevant
commands to confirm the expected pre-fix failure, then run all commands below
after implementation. Use the existing names in the commands or update this
work order if the implementation adds focused test files.

```bash
(cd apps/backend && go test -trimpath ./internal/workflow/models ./internal/workflow/service ./internal/workflow/handlers ./internal/mcp/handlers ./internal/workflowsync -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'TestAgentProfile|TestBuildAgentProfileMatcher|TestWorkflowImportProfileCatalog' -count=1)
(cd apps/web && pnpm exec vitest run app/settings/workspace/use-workflow-import.test.ts app/settings/workspace/workflow-import-profile-selection.test.tsx components/settings/workflow-sync-status-banner.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-sync-gitlab.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-import-export.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed <changed-go-files>` from the root with the
actual changed Go files. Install workspace dependencies with `(cd apps && pnpm
install --frozen-lockfile)` first if missing.

## Files likely touched

- `apps/backend/internal/workflow/models/export.go` and related tests
- `apps/backend/internal/backendapp/services.go` and `agent_profile_matcher_test.go`
- `apps/backend/internal/workflow/service/service.go`, `sync_apply.go`,
  `import_profile_selection.go`, and related tests
- `apps/backend/internal/workflowsync/service.go` and related tests
- Workflow import HTTP and MCP handlers and tests as necessary
- `apps/web/lib/types/http.ts`, import tests, and named E2E files
- `docs/public/workflow-import-export.md` and `docs/public/workflow-sync.md`
- This plan, work order, and paired specifications for delivery status

## Dependencies

None.

## Risks

Preflight must not lead to a second matcher pass or deletion of an unresolved
desired workflow. Review-action references may repeat, so matching by triple
alone is insufficient. Version 3 must enforce version 2 completion and target
rules. Avoid surfacing provider option values in logs or warnings.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-portable-profile-matching.md)
- [System design](../../specs/tasks/system-design/workflow-portable-profile-matching.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/docs-maintainer`

## Results

Implemented version 3 profile descriptors, option-aware lookup, and sync
preflight with warnings and no writes for unresolved workflows. The initial
format, exporter, and sync regressions failed before the implementation and
passed afterward. Browser import retains explicit binding and revision checks.

Verification passed on the updated base:

- Backend commands above, including all MCP handler tests.
- Focused frontend Vitest: 23 tests; web typecheck passed with
  `NODE_OPTIONS=--max-old-space-size=8192` after the default heap was exhausted.
- Chromium sync and import/export E2E: 14 tests combined, including option
  matching, ambiguity, unchanged bindings, recovery, and reload.
- Changed-Go lint, public-doc validators, specification catalog and lint, and
  `git diff --check` passed. Public-doc validator tests: 63 passed.

The E2E regression followed the backend red tests because the browser fixture
could not express a version 3 profile until the portable parser was extended.

Review fixup: review-action imports now choose the option-aware conversion
before resolving profiles, avoiding a redundant legacy catalog lookup. The
new regression failed before the fix and passed afterward. The full workflow
service test package and changed-Go lint passed after the correction.
