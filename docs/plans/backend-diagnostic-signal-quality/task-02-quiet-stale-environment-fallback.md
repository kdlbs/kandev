---
id: "02-quiet-stale-environment-fallback"
title: "Quiet stale environment fallback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/expected-runtime-log-severity.md"
---

# Task 02: Quiet stale environment fallback

Classify a stale session environment reference as an expected fallback.

## Scope

- Match `taskrepo.ErrTaskEnvironmentNotFound` before the warning log.
- Use debug severity for that typed condition.
- Preserve the task-based environment fallback and workspace result.
- Keep warnings for all other lookup errors.

## Exclusions

- Do not change session or task-environment persistence.
- Do not remove the fallback lookup.
- Do not classify errors by message text.

## Traceability

- `REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.11`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.12`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.15`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.16`
- Design: `docs/specs/platform/system-design/expected-runtime-log-severity.md`

## Acceptance

- A typed missing environment emits no warning and still returns the task
  fallback result.
- An unexpected environment lookup error still emits one warning.
- The fallback keeps the authoritative task-environment identifier.

## Verification

```bash
cd apps/backend && go test ./internal/task/service -run 'TestGetWorkspaceInfoForSession_(AlignsTaskEnvironmentIDOnFallback|MissingEnvironmentLogSeverity|UnexpectedEnvironmentErrorLogSeverity)' -count=1
```

The missing-environment log test must fail before the production change because
the current branch records every direct lookup error as a warning.

## Files likely touched

- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`
- `apps/backend/internal/task/service/service_turns_logging_test.go`

## Dependencies

None.

## Results

Implemented typed not-found classification before logging the stale session
environment lookup. The task fallback remains authoritative, with debug
evidence for the expected miss and warning-level evidence for unexpected
lookup errors.

Verification:

```text
cd apps/backend && go test ./internal/task/service -run 'TestGetWorkspaceInfoForSession_(AlignsTaskEnvironmentIDOnFallback|MissingEnvironmentLogSeverity|UnexpectedEnvironmentErrorLogSeverity)' -count=1
Go test: 3 passed in 1 packages
```
