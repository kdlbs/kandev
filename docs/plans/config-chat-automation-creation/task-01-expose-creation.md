---
id: "01-expose-creation"
title: "Expose automation creation in configuration chat"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-CONFIG-AUTOMATION-001
acceptance_criteria:
  - AC-OFFICE-CONFIG-AUTOMATION-001.1
  - AC-OFFICE-CONFIG-AUTOMATION-001.2
  - AC-OFFICE-CONFIG-AUTOMATION-001.3
  - AC-OFFICE-CONFIG-AUTOMATION-001.4
system_design:
  - ../../specs/office/system-design/config-chat-automation-creation.md
---

# Task 01: Expose automation creation

## Summary

Create the configuration-chat tool and wire it to the existing automation
service. Supply executable coverage from tool discovery through persistence.

## In scope

Tool schema and registry, action constant, handler dependency and wiring,
authorization preservation, prompt instructions, public docs, and focused tests.

## Out of scope

New UI, other automation tools, scheduler behavior, storage redesign, flags,
external MCP discovery, and generic review/verification passes.

## Acceptance

1. The tool is advertised only in configuration mode, with lossless structured
   forwarding and existing creation defaults/response semantics.
2. Scoped dispatch creates and reads a real automation; invalid or unauthorized
   requests and raw automation-run calls fail without creating records.
3. Prompt and public docs describe resource discovery and trigger enablement;
   no manual run is issued by the creation handler.

## Verification

Use `/tdd` and the backend testing reference. Add failing cases listed in the
[plan](plan.md#tests) before production code. Run from the repository root:

```sh
(cd apps/backend && go test ./internal/mcp/... ./internal/backendapp ./internal/automation -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/server/config_automation_handlers.go` and matching
  new test file; `server.go`; existing schema/catalog assertions as needed.
- `apps/backend/internal/mcp/handlers/config_automation_handlers.go` and matching
  new test file; `handlers.go` for the narrow service dependency and registration.
- `apps/backend/pkg/websocket/actions.go`;
  `apps/backend/internal/backendapp/helpers.go` for production wiring; new
  `apps/backend/internal/mcp/server/config_automation_integration_test.go` for
  real tool invocation, scoped dispatch and persistence coverage.
- `apps/backend/config/prompts/config-context.md` and
  `docs/public/automation-and-mcp.md` and its `coverage.json` entry.
- This plan and work order, plus paired spec statuses after implementation.

## Dependencies

None. Reuse `automation.Service.CreateAutomation`, its DTOs, existing scoped
MCP dispatch, and existing settings discovery; do not add a new store interface.

## Risks

Shared workflow registration also feeds other surfaces. Configuration principal
surface cannot be inferred from the existing principal enum alone. Existing
creation has no retry idempotency and is not atomic across trigger inserts.

## Parallelism

`sequential`

## Inputs

Read the paired requirement and system design in full, `apps/backend/AGENTS.md`,
server `config_handlers.go`, handler `config_workflow_handlers.go`,
`automation_authorization.go`, automation `models.go`, `service.go` and
`handlers.go`, and composition `registerMCPAndDebugRoutes`.

## Results

- `PATH=/usr/local/go/bin:$PATH go test ./internal/mcp/... ./internal/backendapp ./internal/automation -count=1` from `apps/backend`: passed. The initial run found the catalog count change (41 to 42); corrected and the full command passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 62 tests.
- `node scripts/validate-public-docs.mjs`: passed, 47 published pages.
- `python3 scripts/list-docs.py validate`: passed, 291 decisions and 1038 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

TDD evidence: initial catalog/argument tests failed because the tool was absent;
the backend registration test failed because the action was absent. Both passed
after implementation. No delegation, commit, push, or PR was performed.


## PR review remediation

Made the advertised schema's root reject unknown fields explicitly, matching
existing shared runtime validation. Added a regression for both discovery and
the misspelled `max_concurrent_run` argument. Added public-doc test/validation
commands to the repeatable verification blocks. Product behavior and ownership
remain unchanged. Targeted verification is recorded after the checks complete.
