---
id: "02-probe-failure-log-diagnostics"
title: "Log sanitized probe failure diagnostics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001
acceptance_criteria:
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.1
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.2
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.3
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.4
system_design:
  - ../../specs/agents/system-design/probe-failure-diagnostics.md
---

# Task 02: Log sanitized probe failure diagnostics

## Summary

A failed probe leaves child-side failure evidence in the backend log: a
bounded stderr tail, sanitized for the log boundary, plus the failure
classification the client response cannot carry. Client payloads stay
sanitized.

## In scope

- Capture the child's stderr in the trailing-window `stderrBuffer`
  (4 KiB retention) during probes.
- On failure, log the tail at both failure sites (`ACP probe failed`,
  `ACP profile probe failed`) through `sanitizeProbeDiagnostic`
  (`scrubTmpPaths` followed by `securityutil.Sanitize`), so
  likely credentials and private URL/path details are masked while ordinary
  diagnostic text stays intact. Discard a truncated first line before logging,
  retaining the raw bounded tail for classification.
- Sanitize direct protocol error fields and response errors, and secondary
  model-discovery errors, at the same provider-diagnostic boundary.
- Include the sanitized underlying error, stderr and managed classification in
  the message so they survive the relay's JSON envelope projection, while
  unrelated structured fields remain excluded.
- Extract the existing sanitizer and its tests into shared
  `common/securityutil`, retaining the runtime routing facade.
- In profile context, additionally log the pre-sanitization managed-runtime
  classification as `managed_failure_code` alongside the client-visible code.
- Regression tests for the scrubbed tail, the credential masking (both
  contexts), and the pre-sanitize code.

## Out of scope

- Client-visible probe error copy and provider-gateway status hints.
- Persistent-session stderr streaming to the task UI.
- The launcher log-format pin (Task 01).

## Applicable criteria

- AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001.1 through 001.4
- System design: [Probe Failure Diagnostics](../../specs/agents/system-design/probe-failure-diagnostics.md)

## Acceptance conditions

1. A profile probe failure log entry carries the npm/child cause with tmp
   paths scrubbed and the managed failure code, while the client response
   keeps the sanitized message and empty code. Without stderr, the sanitized
   protocol error still provides a cause.
2. Credential-shaped values in child stderr (token prefixes, Bearer
   challenges, credential assignments) are masked in both failure logs, and
   the diagnostic text around them survives. Opaque token-shaped values,
   URL account/query content, local paths and marker-less truncated credential
   suffixes must not escape through stderr or structured error fields.
3. The captured tail never exceeds the trailing window.

## Verification commands

```bash
cd apps/backend
go test -race -trimpath ./internal/common/securityutil/ ./internal/agent/runtime/routingerr/ -count=1
go test -race -trimpath ./internal/agentctl/server/utility/ -run 'ProbeFailureLog|ProbeModelDiscoveryFailureLog|CarriesScrubbedStderr|StderrBuffer' -count=1
```

## Likely files

- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/profile_probe_context_test.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor_test.go`
- `apps/backend/internal/agentctl/server/utility/probe_failure_privacy_test.go`
- `apps/backend/internal/agentctl/server/utility/probe_failure_log.go`
- `apps/backend/internal/common/securityutil/sanitize.go` and its existing tests
- `apps/backend/internal/agent/runtime/routingerr/sanitize.go` (compatibility facade)

## Results

Implemented across commits `7f16a1cd8` (scrubbed tail and managed failure
code) and `09a0eca40` (credential masking at both log sites, from PR review).
The bounded stderr capture and its original tests already existed. Review
remediation adds provider-diagnostic redaction, safe handling of a truncated
first line, and protocol/secondary-error coverage. The original author's
reported environment failures are historical claims, not current verification.
Hosted RED assertions and final hosted results are retained in PR evidence.
