---
id: "01-cursor-api-client"
title: "Add the Cursor API client and contract fixtures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-001.2
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-003.1
  - AC-EXECUTORS-CURSOR-CLOUD-003.3
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 01: Add the Cursor API client and contract fixtures

## Summary

Client tests prove request encoding and parse all supported event/status forms without duplicate simplified/rich events.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Add a typed Go client in proposed internal/cursorcloud. Implement creation, run submission, status, cancellation, catalogs, and SSE parsing.
- Add local HTTP fixtures for success, malformed payloads, truncated tools, shared event IDs, retention expiry, authentication errors, busy responses, and rate limiting.
- Keep production origin fixed, disable credential-bearing redirects, bound response sizes, and sanitize errors. Separate safe reads from non-idempotent mutations.
- Record the exact upstream schema assumptions in fixtures. Use injectable clients and clocks; do not require a paid account for tests.

- Verify create-time agentId conflict semantics and follow-up MCP/header replacement against current official documentation and explicit fixture schemas.
- Add TestFollowupReplacesMCPHeaders and TestCreateStableAgentID. Add unsupported-contract cases that block launch/rollout and preserve unknown submission state; no broad bearer or non-idempotent create fallback.
- Include list-runs support for explicit submission resolution. Live verification remains a rollout gate; fixture success is not provider proof.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Client tests prove request encoding and parse all supported event/status forms without duplicate simplified/rich events.
- A timed-out follow-up is returned as outcome-unknown without an automatic retry; read retries honor cancellation and rate-limit timing.
- Credentials never appear in errors or logs, and redirects cannot send them to another origin.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/backend && go test ./internal/cursorcloud -count=1)
```

### Evidence mapping

- 001.2, 002.5: `internal/cursorcloud/client_test.go: TestClientAdmissionErrors, TestFollowupTimeoutNoRetry`.
- 003.1, 003.3: `internal/cursorcloud/stream_test.go: TestStreamEvents, TestStreamRetentionExpired`.

## Files likely touched

- `apps/backend/internal/cursorcloud/ (new client, DTOs, SSE parser, testdata, tests)`.

## Dependencies

None.

## Risks

The public-beta API can change. Fixtures must distinguish documented facts from assumptions; unknown states cannot be interpreted as success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Implemented the typed v1 client for stable-ID agent creation, follow-up runs, agent/run lookup, run listing, cancellation, model/repository catalogs, and SSE streaming. Mutating requests are never retried; ambiguous submissions remain `ErrOutcomeUnknown`. Follow-ups require an explicit MCP-server array so callers cannot accidentally keep a prior run's grant. Redirects are not followed, responses and SSE frames are bounded, and provider error bodies are not exposed.

The contract fixtures cover the documented `bc-<uuid>` identity and `agent_id_conflict` behavior, replacement `mcpServers` on follow-up, request payload fields, run/result shapes, status and tool events, duplicate SSE IDs with distinct event types, retention expiry, malformed payloads, authentication/busy errors, rate-limit retry timing, cancellation, timeout ambiguity, and redirect handling. The current upstream schema was checked against the [Cursor Cloud Agents API](https://cursor.com/docs/cloud-agent/api/endpoints); this is fixture verification, not a live account check.

Verification passed:

```bash
(cd apps/backend && go test ./internal/cursorcloud -count=1)
```

No paid or live Cursor request was made. Live create conflict and second-turn MCP replacement remain rollout checks.
