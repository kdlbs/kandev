---
id: "02-classify-agentctl-failures"
title: "Classify agentctl failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 02: Classify agentctl failures

## Summary

Keep useful failure classes in the existing log relay.
Add fields at the current failure sites and extend the current typed allowlists.

## In scope

- Replace the running-agent configuration refusal with `ErrConfigureWhileRunning` (new), with the same error text.
- Add `agent_configure` and `git_inspection` operation fields to their existing failure records.
- Reuse the filesystem classifier for Git status. Classify configuration refusal by typed identity.
- When present, attach trusted task, session, and instance UUIDs from the API instance configuration.
- Add `agent_probe` to utility failure summaries with an existing trusted probe code or `unknown`.
- Extend the forwarding allowlists with the two new operations and five declared classes.
- Keep producer and forwarding tests at their owning package boundaries.

## Out of scope

- New provider signatures, response bodies, HTTP statuses, retries, or raw error forwarding.
- Changes to the existing probe stderr capture, sanitizers, or client failure-code mapping.
- A shared error registry, new identity store, or parser redesign.
- Changes to the Git changing-evidence warning policy.

## Acceptance

1. The running/starting configuration refusal has `agent_running` classification and the same response as before.
   Git status uses the existing bounded class. Available trusted UUIDs survive forwarding.
2. Probe summaries forward only the four existing declared codes or an explicit unknown cause.
   Direct execution errors preserve typed cancellation or deadline evidence. One-shot probes do not invent session identity.
3. Real JSON encoding and the current relay preserve the added fields at an info file threshold.
   Invalid UUIDs, unsupported codes, duplicate fields, raw errors, paths, command arguments, and credentials retain their existing exclusion.

## Regression approach

Add `TestAgentConfigureFailureDiagnostic`, `TestGitStatusFailureDiagnostic`,
and `TestProbeFailureSummaryDiagnostic` in the owning API and utility test packages.
Assert safe fields and unchanged responses through the real handler paths.
The current source fails these assertions because it emits only an excluded error field.

Extend `child_log_context_test.go` for each added operation and class.
Use the real JSON logger encoding and existing `pipeOutput` helpers.
Cover supported values, unknown fallback at the producer, unsupported values at the relay, and sensitive-field exclusion.
Run existing probe privacy tests to preserve their independent redaction boundary.
No production runtime import enters the standalone agentctl package.

## Verification

Execute from the repository root:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/api -run 'AgentConfigureFailureDiagnostic|GitStatusFailureDiagnostic|Configure|GitStatus' -count=1)
(cd apps/backend && go test -race ./internal/agentctl/server/process -run 'Configure' -count=1)
(cd apps/backend && go test -race ./internal/agentctl/server/utility -run 'ProbeFailureSummaryDiagnostic|ProbeFailureLog|ProbeModelDiscoveryFailureLog|CarriesScrubbedStderr|StderrBuffer' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl/launcher -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/manager.go apps/backend/internal/agentctl/server/api/server.go apps/backend/internal/agentctl/server/api/git.go apps/backend/internal/agentctl/server/api/failure_diagnostics.go apps/backend/internal/agentctl/server/api/failure_diagnostics_test.go apps/backend/internal/agentctl/server/utility/handler.go apps/backend/internal/agentctl/server/utility/failure_diagnostics_test.go apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/failure_diagnostics.go` (new, private field helpers)
- `apps/backend/internal/agentctl/server/api/failure_diagnostics_test.go` (new)
- `apps/backend/internal/agentctl/server/utility/handler.go`
- `apps/backend/internal/agentctl/server/utility/failure_diagnostics_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context_test.go`

## Dependencies

None. Execute after Task 01 to keep the session sequential.

## Risks

- A free-form probe code cannot become trusted diagnostic context.
- A configuration sentinel must keep its original error text and response.
- A pre-launch or one-shot operation can lack session identity.

## Parallelism

`sequential`

## Inputs

- [Failure attribution](../../specs/platform/requirements/runtime-failure-attribution.md), criterion .9.
- [Child context requirements](../../specs/platform/requirements/runtime-diagnostic-signal.md), criteria .4 and .5.
- [Producer design](../../specs/platform/system-design/runtime-failure-attribution.md#agentctl-failure-producers).
- [Forwarding allowlist](../../specs/platform/system-design/runtime-diagnostic-signal.md#safe-context-allowlist).
- Existing probe redaction, handler, and real JSON forwarding tests.

## Results

Completed on 4 November 2026.

- `process.ErrConfigureWhileRunning` replaces the formatted refusal and keeps
  the text `cannot configure while agent is running`.
- The configure and generic Git status failure records now add `operation`,
  `error_class`, and canonical task, session, and instance UUIDs. The response
  bodies, statuses, and existing raw local error fields are unchanged.
- Git status reuses `fsdiagnostics.ErrorClass`. The configuration sentinel maps
  to `agent_running`; cancellation and deadline keep their typed classes and all
  other configuration errors are `unknown`.
- Probe failures add `operation=agent_probe` with one of the four declared probe
  codes or `unknown`. Direct execution errors use typed cancellation or deadline
  evidence first.
- The launcher allowlists accept `agent_configure`, `agent_probe`, and the five
  declared classes. Unsupported values are still dropped.
- `go test -race` passed for the API, process, utility, and launcher packages.
  `bash scripts/lint-go-changed` reported 0 issues.
- Review remediation also attaches the trusted instance identity to probe
  failure records, keeps the changing-evidence Git status class, and logs the
  multi-repository failure record. Handler-level execution-error coverage and a
  reader-ping case preserve those branches.
