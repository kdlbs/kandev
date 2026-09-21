---
id: "01-rebind-claims"
title: "Rebind transferred Send Now claims"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.8
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.10
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 01: Rebind transferred Send Now claims

## Summary

When a session handoff moves a pending Send Now claim, persist the successor's
full queue session identity and operation generation. This keeps restart
reconciliation usable while retaining exact claim fencing.

## Acceptance

- Accepted and unaccepted claims transferred through direct and durable owned
  paths use the destination session incarnation and generation.
- Claim IDs, accepted state, source rows, dispatch metadata, FIFO order, and
  rollback behavior remain unchanged.
- Requests and settlements using the retired source identity remain fenced.
- Reconciliation after source-session removal and process restart can settle
  both claim states under the successor identity.

## Verification

```bash
(cd apps/backend && go test -v -run '^(TestSQLiteTransferSessionIdentitiesRebindsPendingSendNowClaim|TestTransferredIdentityAwareSendNowClaimReconcilesAfterProcessRestart)$' ./internal/orchestrator/messagequeue ./internal/orchestrator)
(cd apps/backend && go test -race ./internal/orchestrator/messagequeue)
(cd apps/backend && go test ./internal/orchestrator/messagequeue ./internal/orchestrator)
```

## Results

The direct and durable transfer paths now pass the live destination identity
into the atomic transfer transaction, which validates it before rebinding the
claim and derives the claim generation from the destination session generation.
Accepted and unaccepted transfer, restart reconciliation, source fencing, and
race coverage pass. No UI or public documentation change is required.

