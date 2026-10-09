---
id: "02-graphql"
title: "Classify GraphQL rate payloads"
status: done
wave: 2
depends_on: ["01-rebase"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.1
  - AC-INTEGRATIONS-GITHUB-RATE-001.2
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-001.6
  - AC-INTEGRATIONS-GITHUB-RATE-004.1
  - AC-INTEGRATIONS-GITHUB-RATE-004.2
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 02: Classify GraphQL rate payloads

## Summary

Classify rate-error payloads independently of the HTTP status. Retain the governing quota and retry evidence through PAT and CLI paths.

## In scope

- Share payload detection and failure classification.
- Recognize message-only secondary errors and typed rate errors.
- Process available quota payload evidence before constructing the error, including CLI nonzero-exit responses.
- Preserve successful zero-remaining responses and partial non-rate GraphQL behavior.
- Retain the maximum applicable provider retry boundary and the existing sanitized error contract.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

Add TestGraphQLRatePayloadClassificationAcrossClients. Cover primary reset, later Retry-After, invalid reset, message-only secondary, partial data, and successful zero remainder. Assert the next admission as well as the returned error.

## Acceptance

- HTTP 200 primary errors return primary exhaustion and the governing retry boundary.
- Message-only rate errors return secondary details and update admission.
- Successful payloads and non-rate GraphQL errors retain their existing semantics.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/github -run 'Test.*(GraphQL|Rate|Classif|RetryAfter)' -count=1)
(cd apps/backend && go test -race ./internal/github -run 'Test.*(GraphQL|Rate|Admission)' -count=1)
```

## Files likely touched

- apps/backend/internal/github/rate_error.go
- apps/backend/internal/github/graphql.go
- apps/backend/internal/github/gh_client.go
- apps/backend/internal/github/pat_client.go
- apps/backend/internal/github/rate_error_test.go
- apps/backend/internal/github/pat_client_testserver_test.go
- apps/backend/internal/github/gh_client_commands_test.go

## Dependencies

01-rebase. Execute sequentially.

## Risks

A successful response can spend the final quota point. Status 200 plus remaining zero is not sufficient evidence of an operation failure.

## Parallelism

`sequential`

## Inputs

RATE-001 and RATE-004. System design: Response classification. Existing TestPATClientExecuteGraphQLClassifiesRateLimitedPayload.

## Results

Completed 2026-09-27. PAT and CLI GraphQL paths share payload-aware
classification. HTTP 200 rate errors use quota data and preserve the later
valid retry boundary. Invalid reset values retain zero-remaining evidence and
use the conservative fallback. Message-only secondary errors update admission.
Rate-error details omit partial GraphQL data; successful zero-remainder and
partial non-rate responses retain their behavior.

Validation passed:

- `go test ./internal/github -run '^TestGraphQLRatePayloadClassificationAcrossClients$' -count=1`
- `go test ./internal/github -run 'Test.*(GraphQL|Rate|Classif|RetryAfter)' -count=1`
- `go test -race ./internal/github -run 'Test.*(GraphQL|Rate|Admission)' -count=1`
