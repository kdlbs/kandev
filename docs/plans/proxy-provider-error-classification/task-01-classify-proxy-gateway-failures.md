---
id: "01-classify-proxy-gateway-failures"
title: "Classify proxy gateway failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.5
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.6
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.8
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 01: Classify Proxy Gateway Failures

## Summary

Map structured and exact ACP-wrapped upstream gateway failures to the shared
transient provider-unavailable code. Map the explicit proxy
all-credentials-refused envelope to the existing hard missing-credentials code.

## Acceptance

- Exact 500, 502, and 504 gateway evidence is high-confidence transient
  provider unavailability and permits the existing dynamic policy evaluator.
- The explicit proxy credential-refusal envelope is a high-confidence hard
  credential failure.
- Generic prose and unrelated status text remain unclassified.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr
```

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/{routingerr,runtime_rules,rules}.go`
- `apps/backend/internal/agent/runtime/routingerr/*_test.go`

## Risks

Broad text matching could authorize replay after model-authored or local
failure text. Rules must require the structured status or exact ACP/proxy
envelope.

## Results

- Structured HTTP 500, 502, and 504 now classify as transient
  `provider_unavailable` errors.
- Exact ACP `API Error` 500/502/504 gateway envelopes classify as the same
  high-confidence transient failure. Generic server-error prose remains
  unclassified.
- The explicit `proxy_error` all-credentials-refused envelope classifies as
  hard `missing_credentials`.
- Verification passed: `cd apps/backend && go test -tags fts5
  ./internal/agent/runtime/routingerr`.
