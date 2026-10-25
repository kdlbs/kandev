---
created: 2026-09-19
status: implemented
requirements:
  - REQ-OFFICE-CONFIG-AUTOMATION-001
system_design:
  - ../../specs/office/system-design/config-chat-automation-creation.md
---

# Configuration Chat Automation Creation Plan

## Overview

Add one configuration-only MCP creation tool backed by the existing automation
service. Deliver schema, dispatch, composition wiring, prompt, public docs and
focused tests in one vertical work order. The user authorized implementation in the follow-up turn.

## Inputs

- [Requirements](../../specs/office/requirements/config-chat-automation-creation.md)
- [System design](../../specs/office/system-design/config-chat-automation-creation.md)

## Work orders

- [x] [Task 01: Expose automation creation](task-01-expose-creation.md), wave 1,
  no dependencies, sequential.

## Tests

Implemented coverage:

| Acceptance | Test location and coverage |
| --- | --- |
| AC-OFFICE-CONFIG-AUTOMATION-001.1 | `internal/mcp/server/config_automation_handlers_test.go`: `TestConfigAutomationCatalog` and mode-transition cases |
| AC-OFFICE-CONFIG-AUTOMATION-001.2 | Same server file: `TestConfigAutomationArguments`; `internal/mcp/server/config_automation_integration_test.go`: `TestConfigAutomationMCPCreateAndRead` |
| AC-OFFICE-CONFIG-AUTOMATION-001.3 | `internal/mcp/handlers/config_automation_handlers_test.go`: `TestConfigAutomationCreateErrors` and `TestAutomationCannotCreateAutomation`; integration access-denial cases |
| AC-OFFICE-CONFIG-AUTOMATION-001.4 | Existing `TestSyspromptToolNames_MatchMCPConfigMode`; integration asserts creation produces no manual run |

## End-to-end evidence

The integration test lives beside the MCP server to use its real tool invocation
fixture without exporting test-only production APIs. It traverses MCP tool invocation, scoped dispatcher,
automation service and SQLite persistence, then an existing read path. There
are no rendered UI changes, so no Playwright file or ASCII preview is required.

## Verification

Run from the repository root after TDD implementation:

```sh
(cd apps/backend && go test ./internal/mcp/... ./internal/backendapp ./internal/automation -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

- `PATH=/usr/local/go/bin:$PATH go test ./internal/mcp/... ./internal/backendapp ./internal/automation -count=1` from `apps/backend`: passed. The initial run found the catalog count change (41 to 42); corrected and the full command passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 62 tests.
- `node scripts/validate-public-docs.mjs`: passed, 47 published pages.
- `python3 scripts/list-docs.py validate`: passed, 291 decisions and 1038 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

TDD evidence: initial catalog/argument tests failed because the tool was absent;
the backend registration test failed because the action was absent. Both passed
after implementation. No delegation, commit, push, or PR was performed.


## Risks and exclusions

Creation can make enabled triggers eligible to fire immediately. The existing
service can return a persisted automation with fewer triggers after a storage
failure; inspect returned state and avoid automatic retries. No new CRUD suite,
external tool exposure, storage transaction, flag or UI is included.

## PR review remediation

Made the advertised schema's root reject unknown fields explicitly, matching
existing shared runtime validation. Added a regression for both discovery and
the misspelled `max_concurrent_run` argument. Added public-doc test/validation
commands to the repeatable verification blocks. Product behavior and ownership
remain unchanged. Targeted verification is recorded after the checks complete.
