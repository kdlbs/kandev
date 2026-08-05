---
id: "04-attachment-hub"
title: "Task-host attachment hub"
status: pending
wave: 2
depends_on: ["03-task-host-supervisor"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 04: Task-Host Attachment Hub

## Acceptance

- Multiple downstream attachments share one initialized task-host peer; request IDs, cancellations,
  responses, and stale generations cannot cross attachments.
- One canonical URI is open upstream. Duplicate opens do not rewind text, changes are versioned in
  arrival order, the final detach/close releases documents, and no attachment transition stops the
  language server.
- New/reconnecting attachments receive generation, workspace metadata, capabilities, and cached
  diagnostics without sending initialize/shutdown/exit.

## TDD sequence

1. Add failing hub tests with two in-memory attachments for ID collision/remapping, cancellation,
   stale response rejection, notification fanout, diagnostic replay, and detach cleanup.
2. Add failing document-broker tests for duplicate open, stale second open, interleaved changes,
   monotonically increasing versions, save ordering, final close, and disconnect release.
3. Implement the attachment handshake and allowed feature-routing boundary. Reject lifecycle and
   unsupported client messages without terminating the task-host generation.
4. Join every attachment reader/writer on hub/manager close and run race/leak repetitions.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api -run 'Test(Attachment|Hub|Document|LSPAttach)'
cd apps/backend && go test -race ./internal/agentctl/server/lsp ./internal/agentctl/server/api
cd apps/backend && go test ./internal/agentctl/server/lsp -run 'Test(Attachment|Hub|Document)' -count=20
```

## Files likely touched

- `apps/backend/internal/agentctl/server/lsp/attachment.go`
- `apps/backend/internal/agentctl/server/lsp/hub.go`
- `apps/backend/internal/agentctl/server/lsp/documents.go`
- `apps/backend/internal/agentctl/server/lsp/attachment_test.go`
- `apps/backend/internal/agentctl/server/lsp/hub_test.go`
- `apps/backend/internal/agentctl/server/lsp/documents_test.go`
- `apps/backend/internal/agentctl/server/api/lsp.go`
- `apps/backend/internal/agentctl/server/api/lsp_test.go`
- `apps/backend/internal/lsp/protocol/message.go`
- `apps/backend/internal/lsp/protocol/message_test.go`

## Dependencies

Task 03 owns the process, upstream peer, generation, workspace metadata, and snapshots.

## Parallelism

Sequential because it mutates the task-host manager and protocol package from Task 03.

## Inputs

- Spec: task-host multiplexer and shared document synchronization.
- Current frontend `JsonRpcConnection`, `lsp-document-sync`, file-URI, and provider request shapes.
- Existing protocol 16 MiB frame limit and process-manager teardown rules.

## Output contract

Report routing/document invariants, RED/GREEN results, race/leak evidence, and any deliberately
unsupported server/client method. Update task/plan status and actual files.

## Results

Pending.
