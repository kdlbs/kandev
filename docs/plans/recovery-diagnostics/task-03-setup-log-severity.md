---
id: "03-setup-log-severity"
title: "Quiet empty setup scripts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.12
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
---

# Task 03: Quiet empty setup scripts

## Summary

Record intentional comment-only setup omissions at debug level. Preserve
setup resolution and actual script-failure warnings.

## In scope

- Change the omission log in `resolvePreparerSetupScript` from Warn to Debug.
- Preserve identity fields and omit script contents.
- Cover default and explicit comment-only scripts without changing execution.

## Out of scope

Other work orders, terminal behavior, UI, schema, retry policy, and live-instance changes.

## Acceptance

1. Comment-only scripts return an empty result without a warning.
2. Debug logs retain useful identity fields and contain no script content.
3. Executable scripts and actual setup failures retain their existing behavior.

## Regression evidence

Extend `preparer_script_test.go` with
`TestResolvePreparerSetupScriptDiagnosticSeverity`. Observe the logger while
resolving default and explicit comment-only scripts. The warning assertion must
fail before the change. Include blank and shebang-only inputs and an executable
script control. Restore the package logger with cleanup and do not run this test
in parallel. Retain existing script execution failure tests.

## Verification

Run the regression first and record its expected failure. After implementation,
run these commands from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(ResolvePreparerSetupScript|IsScriptEffectivelyEmpty|.*SetupScript.*)' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestResolvePreparerSetupScriptDiagnosticSeverity' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/preparer_script.go`
- `apps/backend/internal/agent/runtime/lifecycle/preparer_script_test.go`

## Dependencies

None. Execute in the plan order by default.

## Risks

Package logger replacement needs serial tests and cleanup.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/diagnostic-logging.md)
- [System design](../../specs/platform/system-design/diagnostic-logging-01.md)
- [Plan evidence and exclusions](plan.md)
- Backend AGENTS.md and TDD backend test guidance.

## Results

Changed the intentional comment-only setup omission log from WARN to DEBUG.
Resolution remains empty for default, explicit, blank, and shebang-only scripts;
executable scripts remain executable. The observer regression confirms identity
fields remain present and script contents remain absent.

The warning assertion failed before the production change because the existing
log level was WARN. Verification passed:

```text
go test ./internal/agent/runtime/lifecycle -run 'Test(ResolvePreparerSetupScript|IsScriptEffectivelyEmpty|.*SetupScript.*)' -count=1
go test -race ./internal/agent/runtime/lifecycle -run 'TestResolvePreparerSetupScriptDiagnosticSeverity' -count=1
git diff --check
```
