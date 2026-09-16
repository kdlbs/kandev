---
id: "01-correct-replay-grants"
title: "Correct replay grants"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.13
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.15
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
system_design:
  - "../../specs/plugins/system-design/conversation-recovery.md"
---

# Task 01: Correct replay grants

## Summary

Make every valid contiguous replay acknowledgment succeed.
Preserve the distinct snapshot cutoff and all token authorization checks.

## In scope

- Separate cutoff and cursor inputs in the internal grant helper.
- Resolve replay claims and effective cursor before minting grants.
- Update all helper callers, including ACK renewal and tests.
- Add backend protocol tests and the plugin replay regression.

## Out of scope

Core hydration, continuation expiry, schema changes, and public SDK changes.

## Acceptance

- Resume from 10 with events 11 and 12. ACK 11, ACK 12, and live ACK 13 all succeed.
- Snapshot cutoff remains 12. Foreign, expired, tampered, and forward-gap ACKs fail without cursor movement.
- Fresh and replacement subscriptions grant their actual registered position and release claims on failure.

## TDD sequence

1. Add `TestOrderedReplayGrantAcceptsEveryContiguousAck` using the real signer,
   accepted subscribe response, and `handleSessionAck`. Cover core and plugin consumers.
2. Obtain RED for the rejected intermediate ACK. Add invalid-ACK and fresh/replacement controls.
3. Correct the grant inputs and call sites. Preserve one-event and duplicate ACK behavior.
4. Add the plugin reconnect case in `conversation-host.test.tsx`; assert later live projection.

## Files likely touched

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/client_session_ack_test.go`
- `apps/backend/internal/plugins/conversation_stream_service.go`
- `apps/backend/internal/plugins/conversation_handlers_test.go`
- `apps/backend/internal/plugins/conversation_tokens.go` only if helper plumbing requires it
- `apps/web/lib/plugins/conversation-host.test.tsx`
- Other internal grant callers found by `rg -n MintSessionStreamGrant apps/backend`
- `docs/plans/plugins/PLUGIN-API.md` for internal token position clarification

## Verification

From the repository root, after implementation/validation authorization:

```bash
(cd apps/backend && go test -tags fts5 ./internal/gateway/websocket ./internal/plugins)
(cd apps/web && pnpm exec vitest run lib/plugins/conversation-host.test.tsx)
git diff --check
```

These package-scoped Go commands avoid an unrelated full backend audit.
Task 02 owns the shared real-transport browser proof.

## Dependencies

None. Read the whole recovery design before editing the shared grant API.

## Risks

Minting before poison disposition or using W for C can preserve the defect.
A mock that accepts any token cannot prove this repair.


## Inputs

- [Plan and evidence](plan.md).
- [Recovery design](../../specs/plugins/system-design/conversation-recovery.md).
- [Existing requirement](../../specs/plugins/requirements/prompt-history-extraction-host.md).
- Source baseline: PR #3588, commit `792264a0736563d9597bda15cdda5ad97bd60f84`.
- Read the scoped AGENTS.md and TDD skill before implementation.

## Parallelism

`sequential`. Another agent can execute this work order after the user dispatches it.
Do not spawn agents from this work order.

## Results

RED evidence reproduced the defect: with a client acknowledged through
sequence 10 and replay events at 11 and 12, the original grant rejected the
first contiguous ACK at sequence 11 for both core and plugin consumers.

The gateway now uses the snapshot watermark for the snapshot grant and the
actual replay cursor for the resume grant. Claim cleanup remains attached to
failed mint and registration paths. Invalid forward-gap, malformed, and
tampered ACKs still fail without moving the cursor.

GREEN evidence:

- go test -tags fts5 ./internal/gateway/websocket ./internal/plugins: passed.
- go test -race -tags fts5 ./internal/gateway/websocket ./internal/plugins: passed.
- pnpm exec vitest run lib/plugins/conversation-host.test.tsx lib/plugins/conversation-host-isolation.test.tsx: 2 files, 34 tests passed.
- git diff --check: passed.

The checks are package-scoped. They do not constitute a full backend audit or
the full PR review.
